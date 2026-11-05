set shell := ["sh", "-eu", "-c"]

version := trim(read("VERSION"))
go := env("GO", "go")

# List available recipes.
default:
    @just --list

# Build the native launcher with the release version.
build:
    mkdir -p bin
    CGO_ENABLED=0 {{ quote(go) }} build -trimpath -ldflags {{ quote("-X main.version=" + version) }} -o bin/relops ./cmd/relops

# Run the Go integration tests.
test:
    {{ quote(go) }} test ./...

# Check formatting, vet, and run tests with the race detector.
check:
    just --fmt --check
    test -z "$({{ quote(go) }} fmt ./...)"
    {{ quote(go) }} vet ./...
    {{ quote(go) }} test -race ./...

# Build four release binaries and their checksums.
release:
    #!/usr/bin/env sh
    set -eu
    mkdir -p dist
    for platform in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do
        target_os=${platform%/*}
        target_arch=${platform#*/}
        CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" {{ quote(go) }} build -trimpath \
            -ldflags {{ quote("-s -w -X main.version=" + version) }} \
            -o "dist/relops-$target_os-$target_arch" ./cmd/relops
    done
    cd dist
    shasum -a 256 relops-darwin-arm64 relops-darwin-amd64 relops-linux-arm64 relops-linux-amd64 > SHA256SUMS

# Assemble the complete Quick download site under dist/site.
site: release
    {{ quote(go) }} run ./cmd/relops-site --version {{ quote(version) }}

# Build the site and test download selection and copying (requires Node).
site-check: site
    node --check dist/site/js/main.js
    node --test quick_site/tests/downloads.test.cjs
