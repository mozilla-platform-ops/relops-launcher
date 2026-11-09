set shell := ["sh", "-eu", "-c"]

version := trim(read("VERSION"))
go := env("GO", "go")
sign_identity := env("RELOPS_SIGN_IDENTITY", "Developer ID Application: Andrew Erickson (W5BHZ9R8A3)")
team_id := env("RELOPS_TEAM_ID", "W5BHZ9R8A3")
notary_profile := env("RELOPS_NOTARY_PROFILE", "relops-notary")
quick_account := env("RELOPS_QUICK_ACCOUNT", "aerickson@mozilla.com")
update_key := if path_exists("release-public-key.txt") == "true" { trim(read("release-public-key.txt")) } else { "" }
update_ldflags := " -X github.com/mozilla-platform-ops/relops-launcher/internal/selfupdate.PublicKey=" + update_key
retained_release := "dist/releases/v" + version

# List available recipes.
default:
    @just --list

# Build the native launcher with the release version.
build:
    mkdir -p bin
    CGO_ENABLED=0 {{ quote(go) }} build -trimpath -ldflags {{ quote("-X main.version=relops-launcher-version:" + version + ":end" + update_ldflags) }} -o bin/relops ./cmd/relops

# Run the Go integration tests.
test:
    {{ quote(go) }} test ./...

# Check formatting, vet, and run tests with the race detector.
check:
    just --fmt --check
    test -z "$({{ quote(go) }} fmt ./...)"
    {{ quote(go) }} vet ./...
    {{ quote(go) }} test -race ./...

# Build the macOS Apple silicon and Linux amd64 release binaries and their checksums.
release:
    #!/usr/bin/env sh
    set -eu
    mkdir -p dist
    for platform in darwin/arm64 linux/amd64; do
        target_os=${platform%/*}
        target_arch=${platform#*/}
        CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" {{ quote(go) }} build -trimpath \
            -ldflags {{ quote("-s -w -X main.version=relops-launcher-version:" + version + ":end" + update_ldflags) }} \
            -o "dist/relops-$target_os-$target_arch" ./cmd/relops
    done
    cd dist
    shasum -a 256 relops-darwin-arm64 relops-linux-amd64 > SHA256SUMS

# Assemble the complete Quick download site under dist/site.
site: release
    {{ quote(go) }} run ./cmd/relops-site --version {{ quote(version) }}

# Serve the existing generated site at http://localhost:8080.
serve:
    python3 -m http.server 8080 --bind 127.0.0.1 --directory dist/site

# Rebuild the local preview on source changes (run alongside just serve).
watch-site:
    watchexec --poll 1s --shell=none -w quick_site -w internal -w cmd/relops -w cmd/relops-site -w VERSION -w go.mod -w justfile -- just site

# Watch, rebuild, and serve the local preview in one terminal.
dev:
    watchexec --poll 1s --restart --shell=none -w quick_site -w internal -w cmd/relops -w cmd/relops-site -w VERSION -w go.mod -w justfile -- just _serve-preview

[private]
_serve-preview:
    just site
    just serve

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
    just version={{ quote(version) }} retain-release

# Keep verified release bytes separate from preview builds. Never replace a version.
retain-release:
    #!/usr/bin/env sh
    set -eu
    {{ quote(go) }} run ./cmd/relops-site --version {{ quote(version) }} --require-notarized
    destination={{ quote(retained_release) }}
    if [ -d "$destination" ]; then
        for name in relops-darwin-arm64 relops-linux-amd64; do
            if ! cmp -s "dist/$name" "$destination/$name"; then
                echo "Retained release differs: $destination/$name; use a new release version." >&2
                exit 1
            fi
        done
        echo "Retained release already exists: $destination"
        exit 0
    fi
    mkdir -p dist/releases
    stage=$(mktemp -d dist/releases/.retain-XXXXXX)
    trap 'rm -rf "$stage"' EXIT
    cp dist/relops-darwin-arm64 dist/relops-linux-amd64 dist/macos-notarization.json dist/site/downloads/v{{ version }}/SHA256SUMS "$stage/"
    mv "$stage" "$destination"
    echo "Retained release: $destination"

# Rebuild and test web assets using retained downloads, without building binaries.
site-from-release:
    test -d {{ quote(retained_release) }} || { echo 'No retained release; run just signed-site first.' >&2; exit 1; }
    cd {{ quote(retained_release) }} && shasum -a 256 -c SHA256SUMS
    {{ quote(go) }} run ./cmd/relops-site --version {{ quote(version) }} --binaries {{ quote(retained_release) }} --require-notarized
    node --check {{ quote(retained_release + "/site/js/main.js") }}
    RELOPS_SITE_DIR={{ quote(retained_release + "/site") }} node --test quick_site/tests/downloads.test.cjs

# Check live tools and publish site changes using retained release binaries.
publish-site: check-tools site-from-release
    CLOUDSDK_CORE_ACCOUNT={{ quote(quick_account) }} quick deploy {{ quote(retained_release + "/site") }} relops-launcher

# Sign, notarize, test the bundle, then publish it using the Mozilla account.
publish: check-tools signed-site
    CLOUDSDK_CORE_ACCOUNT={{ quote(quick_account) }} quick deploy dist/site relops-launcher

# Check live catalog refs and paths using GitHub SSH access; never runs tools.
check-tools:
    {{ quote(go) }} run ./cmd/relops-site --check-tools

# Inspect catalog sources for advisory compatibility and documentation warnings.
inspect-tools:
    {{ quote(go) }} run ./cmd/relops-site --inspect-tools

# Authenticate retained artifacts locally. Private key stays outside the checkout.
prepare-github-release:
    test -n "{{ update_key }}" || { echo 'Create release-public-key.txt first; see docs/github-releases.md.' >&2; exit 1; }
    test -n "$RELOPS_UPDATE_PRIVATE_KEY"
    cd {{ quote(retained_release) }} && shasum -a 256 -c SHA256SUMS
    {{ quote(go) }} run ./cmd/relops-release --version {{ quote(version) }} --dir {{ quote(retained_release) }} --private-key "$RELOPS_UPDATE_PRIVATE_KEY"

# Upload a draft only; review and publish separately. Requires the pushed version tag.
github-release-draft: prepare-github-release
    gh release create v{{ version }} --repo mozilla-platform-ops/relops-launcher --verify-tag --draft --title 'relops v{{ version }}' --generate-notes {{ quote(retained_release + "/relops-darwin-arm64") }} {{ quote(retained_release + "/relops-linux-amd64") }} {{ quote(retained_release + "/SHA256SUMS") }} {{ quote(retained_release + "/update.json") }} {{ quote(retained_release + "/update.json.sig") }} LICENSE
