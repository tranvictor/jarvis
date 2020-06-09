BUILD_FILES = $(shell go list -f '{{range .GoFiles}}{{$$.Dir}}/{{.}}\
{{end}}' ./...)

JARVIS_VERSION ?= $(shell git describe --tags 2>/dev/null || git rev-parse --short HEAD)
DATE_FMT = +%Y-%m-%d

bin/jarvis: $(BUILD_FILES)
	@go build -trimpath -o "$@"

test:
	go test ./...
.PHONY: test
