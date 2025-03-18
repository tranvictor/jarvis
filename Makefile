BUILD_FILES = $(shell go list -f '{{range .GoFiles}}{{$$.Dir}}/{{.}}\
{{end}}' ./...)

JARVIS_VERSION ?= $(shell git describe --tags 2>/dev/null || git rev-parse --short HEAD)
DATE_FMT = +%Y-%m-%d

jarvis: $(BUILD_FILES)
	@go build -trimpath -o "$@"

test:
	go test ./...
.PHONY: test

# We use a trick to capture the second and subsequent words from the command line:
#   make release v0.0.33 "some text"
#
# - $(word 2, $(MAKECMDGOALS)) is the second “goal”: v0.0.33
# - $(wordlist 3, ..., $(MAKECMDGOALS)) takes everything after the second goal,

VERSION := $(word 2, $(MAKECMDGOALS))
RELEASE_TEXT := $(wordlist 3, $(words $(MAKECMDGOALS)), $(MAKECMDGOALS))

.PHONY: release
release:
	@if [ -z "$(VERSION)" ]; then \
	  echo "ERROR: Missing version. Usage: make release v0.0.33 \"Release text\""; \
	  exit 1; \
	fi
	@if [ -z "$(RELEASE_TEXT)" ]; then \
	  echo "ERROR: Missing release text. Usage: make release v0.0.33 \"Release text\""; \
	  exit 1; \
	fi
	@echo "Tagging version: $(VERSION) with message: $(RELEASE_TEXT)"
	git tag -a "$(VERSION)" -m "$(RELEASE_TEXT)"
	# If you want to push the tag right away, un-comment the next line:
	# git push origin "$(VERSION)"
	goreleaser1.17 release --clean

# This is the "phony target" trick: any unknown goal (like v0.0.33) goes here
%:
	@:
