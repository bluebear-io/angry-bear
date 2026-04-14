.PHONY: all build test lint clean install demos release release-snapshot release-check

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

all: lint test build

build:
	go build -ldflags "$(LDFLAGS)" -o bin/angry-bear ./cmd/angry-bear

test:
	go test -race ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/ dist/ coverage.out

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/angry-bear

demos:
	@command -v vhs >/dev/null 2>&1 || { echo "VHS is required for demos. Install: https://github.com/charmbracelet/vhs"; exit 1; }
	@mkdir -p docs/assets
	@for tape in demo/*.tape; do echo "Recording $$tape..."; vhs "$$tape"; done

release:
	goreleaser release --clean

release-snapshot:
	goreleaser build --snapshot --clean

release-check:
	goreleaser check --config .goreleaser.yaml
