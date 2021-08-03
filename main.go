package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/spf13/cobra"
	"golang.org/x/net/html"
)

// communityPrefix used to combine the url with repo name
const communityPrefix = "https://github.com/beyondstorage/"

// global variables for statistic
// use global variable may be not a good idea, but the simplest way :)
var issueOpen, issueClose, prOpen, prClose int

// pre-compile regexp when build
var regClosePR = regexp.MustCompile("merged pull request|closed pull request")
var regOpenPR = regexp.MustCompile("opened pull request")
var regOpenIssue = regexp.MustCompile("opened issue")
var regCloseIssue = regexp.MustCompile("closed issue")

// two flags for weekly report
var inputPath, outputPath string

var rootCmd = &cobra.Command{
	Use:   "report-script",
	Short: "report-script generate the predefined format report from BeyondStorage weekly report",
	Example: `  generate report to stdout:     report-script --input "https://url/to/report"
  generate report to file:       report-script --input "https://url/to/report" --output path/to/file
  generate report by local file: report-script --input path/to/input`,
	Version: "0.1.0",
	Run: func(cmd *cobra.Command, args []string) {
		generateReport(inputPath, outputPath)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&inputPath, "input", "", "input for BeyondStorage weekly report, url or local path")
	rootCmd.PersistentFlags().StringVar(&outputPath, "output", "", "output for formatted report, if blank, use stdout instead")
	// mark input flag required
	rootCmd.MarkPersistentFlagRequired("input")
}

func generateReport(input, output string) {
	var writer io.Writer
	if output == "" {
		writer = os.Stdout
	} else {
		f, err := os.Create(output)
		if err != nil {
			log.Fatalf("create output file <%s> failed: [%v]", output, err)
		}
		writer = f
	}

	var reader io.Reader
	// if input start with http or https, handle as url
	// otherwise, handle as local file (because sometimes the network may not work as intended)
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		// Request the HTML page.
		res, err := http.Get(input)
		if err != nil {
			log.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			log.Fatalf("status code error: %d %s", res.StatusCode, res.Status)
		}
		reader = res.Body
	} else {
		res, err := os.Open(input)
		if err != nil {
			log.Fatal(err)
		}
		defer res.Close()
		reader = res
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		log.Fatal(err)
	}

	// init user-link dict
	// key-value like:
	//   @username1: url/to/username1
	//   @username2: url/to/username2
	userDict := make(map[string]string)

	// init header-content dict
	// key-value like:
	//   go-storage: [@user opened issue xxx, @user merged PR request]
	//   go-service-fs: [@user opened PR request, @user closed issue]
	headerContentDict := make(map[string][]string)

	// headers defined as slice to keep headers sequential
	headers := make([]string, 0)

	// location the report element in html document
	doc.Find("td.comment-body").Children().Each(func(i int, s *goquery.Selection) {
		// handle h2 as topic
		if s.Is("h2") {
			aNode := s.ChildrenFiltered("a")
			// attr, ok := aNode.Attr("href")
			// if !ok {
			// 	log.Println("get topic href not found, text:", aNode.Text())
			// 	return
			// }
			headers = append(headers, aNode.Text())
			headerContentDict[aNode.Text()] = make([]string, 0)
			// fmt.Fprintf(writer, "\n## [%s](%s)\n", aNode.Text(), attr)
			return
		}

		// handle ul as pr/issue list
		if s.Is("ul") {
			s.ChildrenFiltered("li").Each(func(_ int, liNode *goquery.Selection) {
				// location the user node
				userNode := liNode.ChildrenFiltered("a.user-mention")

				// if this issue added by bot, just skip over
				if isBot(userNode.Text()) {
					return
				}

				// loop to location the user text
				userText := liNode.Nodes[0].FirstChild
				for ; userText.Type != html.TextNode; userText = userText.NextSibling {
					continue
				}

				// check user link exists, if not, add to userDict
				if _, ok := userDict[userNode.Text()]; !ok {
					userLink, ok := userNode.Attr("href")
					if !ok {
						log.Println("get href from user failed, text:", userNode.Text())
						return
					}
					userDict[userNode.Text()] = userLink
				}

				// add counter by text data
				count(userText.Data)

				// location the issue node
				issueNode := liNode.Children().ChildrenFiltered("a")

				// loop to location the issue text
				issueText := issueNode.Nodes[0].FirstChild
				for ; issueText.Type != html.TextNode; issueText = issueText.NextSibling {
					continue
				}

				// get href attr in a tag
				issueLink, ok := issueNode.Attr("href")
				if !ok {
					log.Println("get href from issue failed, text:", issueNode.Text())
					return
				}

				// headers' last element is the current header, got current header's list
				list := headerContentDict[headers[len(headers)-1]]
				list = append(list, fmt.Sprintf("[%s]%s[%s](%s)", userNode.Text(), userText.Data, issueText.Data, issueLink))
				headerContentDict[headers[len(headers)-1]] = list
			})
		}
	})

	// now start writing to output
	// print weekly stats
	fmt.Fprintf(writer, `
## Weekly Stats

| | Opened this week | Closed this week |
| ---- | ---- | ---- |
| Issues | %d | %d |
| PR's | %d | %d |
`, issueOpen, issueClose, prOpen, prClose)

	fmt.Fprintf(writer, "\n") // add blank line

	// print header and content
	for _, header := range headers {
		// skip headers without contents
		if len(headerContentDict[header]) == 0 {
			continue
		}
		// example: "## [go-storage](https://github.com/beyondstorage/go-storage)"
		fmt.Fprintf(writer, "## [%s](%s%s)\n", header, communityPrefix, header)
		fmt.Fprintf(writer, "\n") // add blank line
		for _, content := range headerContentDict[header] {
			// example: "- [@username] opened an issue [issue name](issue url)\n"
			fmt.Fprintf(writer, "- %s\n", content)
		}
		fmt.Fprintf(writer, "\n") // add blank line
	}

	fmt.Fprintf(writer, "\n") // add blank line

	// print user-link map
	for user, link := range userDict {
		fmt.Fprintf(writer, "[%s]: %s\n", user, link)
	}
}

func count(content string) {
	switch {
	case regOpenPR.MatchString(content):
		prOpen++
	case regClosePR.MatchString(content):
		prClose++
	case regOpenIssue.MatchString(content):
		issueOpen++
	case regCloseIssue.MatchString(content):
		issueClose++
	}
}

// isBot check whether a user is robot
// for now, we only introduced two robots: dependabot, BeyondRobot
func isBot(name string) bool {
	switch name {
	case "@dependabot", "@BeyondRobot":
		return true
	default:
		return false
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}
