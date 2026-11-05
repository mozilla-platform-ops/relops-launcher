VERSION ?= dev
GO ?= go

.PHONY: build test check release

build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '-X main.version=$(VERSION)' -o bin/relops ./cmd/relops

test:
	$(GO) test ./...

check:
	test -z "$$($(GO) fmt ./...)"
	$(GO) vet ./...
	$(GO) test -race ./...

release:
	mkdir -p dist
	@set -eu; for platform in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath \
			-ldflags '-s -w -X main.version=$(VERSION)' \
			-o dist/relops-$$os-$$arch ./cmd/relops; \
	done
	cd dist && shasum -a 256 relops-darwin-arm64 relops-darwin-amd64 relops-linux-arm64 relops-linux-amd64 > SHA256SUMS
