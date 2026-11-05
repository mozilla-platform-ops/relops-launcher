set shell := ["sh", "-eu", "-c"]

version := trim(read("VERSION"))
go := env("GO", "go")
sign_identity := env("RELOPS_SIGN_IDENTITY", "Developer ID Application: Andrew Erickson (W5BHZ9R8A3)")
team_id := env("RELOPS_TEAM_ID", "W5BHZ9R8A3")
notary_profile := env("RELOPS_NOTARY_PROFILE", "relops-notary")
quick_account := env("RELOPS_QUICK_ACCOUNT", "aerickson@mozilla.com")

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

# Store notarization credentials in Keychain using secure interactive prompts.
notary-setup:
    xcrun notarytool store-credentials {{ quote(notary_profile) }} --team-id {{ quote(team_id) }}

# Sign the macOS binaries only; notarization is still required for distribution.
sign-macos: release
    {{ quote(go) }} run ./cmd/relops-macos --version {{ quote(version) }} --identity {{ quote(sign_identity) }} --team-id {{ quote(team_id) }} --profile {{ quote(notary_profile) }} --sign-only

# Sign, submit, wait for Apple acceptance, and verify online notarization tickets.
notarize-macos: release
    {{ quote(go) }} run ./cmd/relops-macos --version {{ quote(version) }} --identity {{ quote(sign_identity) }} --team-id {{ quote(team_id) }} --profile {{ quote(notary_profile) }}

# Prepare the production download site without rebuilding away its signatures.
signed-site: notarize-macos
    {{ quote(go) }} run ./cmd/relops-site --version {{ quote(version) }} --require-notarized
    node --check dist/site/js/main.js
    node --test quick_site/tests/downloads.test.cjs

# Sign, notarize, test the bundle, then publish it using the Mozilla account.
publish: signed-site
    CLOUDSDK_CORE_ACCOUNT={{ quote(quick_account) }} quick deploy dist/site relops-launcher
