SHELL := /bin/bash
GO_BUILD_OPTION := -trimpath -tags netgo

.PHONY: check format vet build tidy

help:
	@echo "Please use \`make <target>\` where <target> is one of"
	@echo "  check      to format, vet"
	@echo "  build      to build report-script"

check: vet

format:
	go fmt ./...

vet:
	go vet ./...

build: tidy check
	go build ./...

tidy:
	go mod tidy
	go mod verify