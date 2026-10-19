#!/bin/bash
# Usage: scripts/release.sh [--dry-run] <version>
#
# Releases visor from this Mac: verifies a snapshot build, tags v<version>, then
# GoReleaser publishes the GitHub release and the oronbz/homebrew-tap cask
# with the local gh login. --dry-run stops after the snapshot verification and
# never tags, pushes or publishes.
set -euo pipefail

REPO="oronbz/visor"
GORELEASER_VERSION="v2.18.2"

red()   { printf '\033[1;31m%s\033[0m\n' "$*"; }
green() { printf '\033[1;32m%s\033[0m\n' "$*"; }
bold()  { printf '\033[1m%s\033[0m\n' "$*"; }
die()   { red "Error: $*" >&2; exit 1; }
warn()  { red "Warning: $*" >&2; }

goreleaser() {
    go run "github.com/goreleaser/goreleaser/v2@$GORELEASER_VERSION" "$@"
}

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
    DRY_RUN=true
    shift
fi
VERSION="${1:-}"
[[ $# -le 1 ]] || die "Usage: scripts/release.sh [--dry-run] <version>"
[[ -n "$VERSION" ]] || die "Usage: scripts/release.sh [--dry-run] <version>"
VERSION="${VERSION#v}"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || die "Version must be semver (e.g. 1.0.0)"
TAG="v$VERSION"

command -v go   >/dev/null 2>&1 || die "go is required"
command -v gh   >/dev/null 2>&1 || die "gh CLI is required (brew install gh)"
command -v lipo >/dev/null 2>&1 || die "lipo is required (Xcode command line tools); releases run on macOS"
[[ -f .goreleaser.yaml && -f go.mod ]] || die "Run this script from the repo root"

# A dry run is how a branch checks its release config, so only a real release
# insists on a clean main that matches origin.
preflight() {
    if [[ -n "$(git status --porcelain)" ]]; then
        if $DRY_RUN; then warn "working tree is not clean"; else die "Working tree is not clean"; fi
    fi
    if [[ "$(git branch --show-current)" != "main" ]]; then
        if $DRY_RUN; then warn "not on main"; else die "Release from main"; fi
    fi
    if ! $DRY_RUN; then
        git fetch origin main --quiet
        [[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] || die "main is not in sync with origin/main"
        [[ -z "$(git ls-remote --tags origin "refs/tags/$TAG")" ]] || die "$TAG is already tagged on origin"
        gh auth status >/dev/null 2>&1 || die "gh is not logged in (gh auth login)"
    fi
}

check_config() {
    goreleaser check || die "goreleaser check failed"
}

binary() {
    local dir
    dir=$(find dist -maxdepth 1 -type d -name "$1" | head -1)
    [[ -n "$dir" && -f "$dir/$2" ]] || die "No $2 under dist/$1"
    printf '%s\n' "$dir/$2"
}

expect() {
    [[ "$2" == *"$3"* ]] || die "$1: expected \"$3\" in: $2"
    green "  $1: $3"
}

# lipo lists a fat binary's slices in the order they were joined, which varies.
archs() {
    lipo -archs "$1" | tr ' ' '\n' | sort | paste -sd ' ' -
}

verify_snapshot() {
    local want="$VERSION-snapshot" bin
    bin=$(binary 'visor-universal_darwin_all' visor)
    expect "darwin archs" "$(archs "$bin")" "arm64 x86_64"
    expect "darwin cgo" "$(go version -m "$bin")" "CGO_ENABLED=1"
    expect "darwin --version" "$("$bin" --version)" "visor $want"
    for target in linux_amd64 linux_arm64 windows_amd64 windows_arm64; do
        local name=visor
        if [[ "$target" == windows_* ]]; then name=visor.exe; fi
        bin=$(binary "visor-other_${target}*" "$name")
        expect "$target pure Go" "$(go version -m "$bin")" "CGO_ENABLED=0"
        if [[ "$target" == linux_* ]]; then
            expect "$target linking" "$(file "$bin")" "statically linked"
        fi
    done
    bin=$(binary 'visor-weather-universal_darwin_all' visor-weather)
    expect "weather darwin archs" "$(archs "$bin")" "arm64 x86_64"
    for target in linux_amd64 linux_arm64 windows_amd64 windows_arm64; do
        local name=visor-weather
        if [[ "$target" == windows_* ]]; then name=visor-weather.exe; fi
        bin=$(binary "visor-weather-other_${target}*" "$name")
        expect "weather $target pure Go" "$(go version -m "$bin")" "CGO_ENABLED=0"
    done
    expect "darwin archive" "$(tar -tzf "dist/visor_${want}_darwin_all.tar.gz")" "visor-weather"
    expect "linux archive" "$(tar -tzf "dist/visor_${want}_linux_amd64.tar.gz")" "visor-weather"
    expect "windows archive" "$(unzip -l "dist/visor_${want}_windows_amd64.zip")" "visor-weather.exe"
    expect "cask" "$(cat dist/homebrew/Casks/visor.rb)" 'binary "visor-weather"'
    expect "cask quarantine hook" "$(cat dist/homebrew/Casks/visor.rb)" 'com.apple.quarantine'
    for pattern in "visor_${want}_darwin_all.tar.gz" "visor_${want}_linux_amd64.tar.gz" "visor_${want}_linux_arm64.tar.gz" \
        "visor_${want}_windows_amd64.zip" "visor_${want}_windows_arm64.zip" '*.deb' '*.rpm' '*.apk' checksums.txt homebrew/Casks/visor.rb; do
        compgen -G "dist/$pattern" >/dev/null || die "Missing dist/$pattern"
    done
    green "  archives, packages, checksums and the cask present"
}

if $DRY_RUN; then bold "Releasing visor $TAG (dry run)"; else bold "Releasing visor $TAG"; fi

bold "[1/5] Preflight"
preflight

bold "[2/5] go vet and go test"
go vet ./...
go test ./...

bold "[3/5] goreleaser check ($GORELEASER_VERSION)"
check_config

bold "[4/5] Snapshot build and verification"
GORELEASER_CURRENT_TAG="$TAG" goreleaser release --snapshot --clean
verify_snapshot

if $DRY_RUN; then
    echo
    green "Dry run passed. Artifacts are in dist/. Nothing was tagged, pushed or published."
    exit 0
fi

bold "[5/5] Tagging $TAG and publishing"
git tag -a "$TAG" -m "visor $TAG"
git push origin "$TAG"
if ! GITHUB_TOKEN="$(gh auth token)" goreleaser release --clean; then
    red "GoReleaser failed after $TAG was pushed. Fix the cause and rerun:" >&2
    red "  GITHUB_TOKEN=\$(gh auth token) go run github.com/goreleaser/goreleaser/v2@$GORELEASER_VERSION release --clean" >&2
    red "or drop the tag: git push --delete origin $TAG && git tag -d $TAG" >&2
    exit 1
fi

echo
green "========================================="
green "  Released visor $TAG"
green "========================================="
echo
echo "  GitHub:   https://github.com/$REPO/releases/tag/$TAG"
echo "  Homebrew: brew update && brew install oronbz/tap/visor"
echo
