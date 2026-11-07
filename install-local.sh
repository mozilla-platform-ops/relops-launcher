#!/bin/sh
# Install the retained notarized release, or build the current checkout with --dev.
set -eu

case ${1-} in
    '') mode=release ;;
    --dev) mode=dev ;;
    -h|--help) echo 'Usage: ./install-local.sh [--dev]'; exit 0 ;;
    *) echo 'Usage: ./install-local.sh [--dev]' >&2; exit 2 ;;
esac
if [ "$#" -gt 1 ]; then
    echo 'Usage: ./install-local.sh [--dev]' >&2
    exit 2
fi

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
install_dir=${HOME:?HOME must be set}/.local/bin
version=$(cat "$project_dir/VERSION")
mkdir -p "$install_dir"
stage=$(mktemp -d "$install_dir/.relops-install.XXXXXX")
trap 'rm -rf "$stage"' EXIT
trap 'exit 1' HUP INT TERM

if [ "$mode" = dev ]; then
    (cd "$project_dir" && CGO_ENABLED=0 go build -trimpath \
        -ldflags "-X main.version=$version" -o "$stage/relops" ./cmd/relops)
else
    if [ "$(uname -s)" != Darwin ] || [ "$(uname -m)" != arm64 ]; then
        echo 'The notarized release is available for macOS arm64 only; use --dev for a local build.' >&2
        exit 1
    fi
    release_dir="$project_dir/dist/releases/v$version"
    if [ ! -f "$release_dir/relops-darwin-arm64" ] || [ ! -f "$release_dir/SHA256SUMS" ]; then
        echo "Retained release v$version not found in $release_dir; prepare it with just signed-site." >&2
        exit 1
    fi
    cp "$release_dir/relops-darwin-arm64" "$stage/relops-darwin-arm64"
    awk '$2 == "relops-darwin-arm64" {print}' "$release_dir/SHA256SUMS" > "$stage/SHA256SUMS"
    if [ "$(wc -l < "$stage/SHA256SUMS" | tr -d ' ')" != 1 ]; then
        echo 'Expected one macOS checksum in the retained release.' >&2
        exit 1
    fi
    (cd "$stage" && shasum -a 256 -c SHA256SUMS)
    codesign --verify --strict --check-notarization \
        '-R=anchor apple generic and certificate leaf[subject.OU] = "W5BHZ9R8A3" and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and notarized' \
        "$stage/relops-darwin-arm64"
    mv "$stage/relops-darwin-arm64" "$stage/relops"
fi
chmod 755 "$stage/relops"
mv -f "$stage/relops" "$install_dir/relops"
printf 'Installed relops %s (%s) to %s/relops\n' "$version" "$mode" "$install_dir"
case ":$PATH:" in
    *":$install_dir:"*) ;;
    *) printf 'Add %s to your PATH to run relops by name.\n' "$install_dir" ;;
esac
