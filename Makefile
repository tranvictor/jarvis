BUILD_FILES = $(shell go list -f '{{range .GoFiles}}{{$$.Dir}}/{{.}}\
{{end}}' ./...)

JARVIS_VERSION ?= $(shell git describe --tags 2>/dev/null || git rev-parse --short HEAD)
DATE_FMT = +%Y-%m-%d

# Image that bundles the darwin (osxcross), windows (mingw-w64) and linux cross
# C toolchains referenced by the per-target CC settings in .goreleaser.yml.
# The tag is the Go version inside the image (see go.mod), not a goreleaser version.
GORELEASER_CROSS_VERSION ?= v1.23.2

jarvis: $(BUILD_FILES)
	@go build -trimpath -ldflags "-X github.com/tranvictor/jarvis/cmd.VERSION=$(JARVIS_VERSION)" -o "$@"

test:
	go test ./...
.PHONY: test

# Interactive release: draft notes from git/PRs, let you edit them, commit
# leftovers, prompt for the version, then tag + push + goreleaser-cross.
# Requires a TTY and GITHUB_TOKEN (repo + homebrew-tranvictor tap).
.PHONY: release
release:
	@GORELEASER_CROSS_VERSION=$(GORELEASER_CROSS_VERSION) bash scripts/release.sh
