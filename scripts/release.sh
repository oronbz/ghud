#!/bin/bash
# Usage: scripts/release.sh [--dry-run] <version>
#
# Releases ghud from this Mac: verifies a snapshot build, tags v<version>, then
# GoReleaser publishes the GitHub release and the oronbz/homebrew-tap formula
# with the local gh login. --dry-run stops after the snapshot verification and
# never tags, pushes or publishes.
set -euo pipefail

REPO="oronbz/ghud"
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
    local out status=0
    out=$(goreleaser check 2>&1) || status=$?
    # check fails on any deprecation, and brews is deliberate; go run hides the
    # distinct exit code, so read the verdict from the output.
    local others
    others=$(printf '%s\n' "$out" | grep 'DEPRECATED:' | grep -v 'DEPRECATED:  *brews ' || true)
    if [[ $status -ne 0 ]] && ! [[ "$out" == *"configuration is valid, but uses deprecated properties"* && -z "$others" ]]; then
        printf '%s\n' "$out" >&2
        die "goreleaser check failed"
    fi
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

verify_snapshot() {
    local want="$VERSION-snapshot" bin
    bin=$(binary 'ghud-universal_darwin_all' ghud)
    expect "darwin archs" "$(lipo -info "$bin")" "x86_64 arm64"
    expect "darwin cgo" "$(go version -m "$bin")" "CGO_ENABLED=1"
    expect "darwin --version" "$("$bin" --version)" "ghud $want"
    for target in linux_amd64 linux_arm64 windows_amd64 windows_arm64; do
        local name=ghud
        if [[ "$target" == windows_* ]]; then name=ghud.exe; fi
        bin=$(binary "ghud-other_${target}*" "$name")
        expect "$target pure Go" "$(go version -m "$bin")" "CGO_ENABLED=0"
        if [[ "$target" == linux_* ]]; then
            expect "$target linking" "$(file "$bin")" "statically linked"
        fi
    done
    bin=$(binary 'ghud-weather-universal_darwin_all' ghud-weather)
    expect "weather darwin archs" "$(lipo -info "$bin")" "x86_64 arm64"
    for target in linux_amd64 linux_arm64 windows_amd64 windows_arm64; do
        local name=ghud-weather
        if [[ "$target" == windows_* ]]; then name=ghud-weather.exe; fi
        bin=$(binary "ghud-weather-other_${target}*" "$name")
        expect "weather $target pure Go" "$(go version -m "$bin")" "CGO_ENABLED=0"
    done
    expect "darwin archive" "$(tar -tzf "dist/ghud_${want}_darwin_all.tar.gz")" "ghud-weather"
    expect "linux archive" "$(tar -tzf "dist/ghud_${want}_linux_amd64.tar.gz")" "ghud-weather"
    expect "windows archive" "$(unzip -l "dist/ghud_${want}_windows_amd64.zip")" "ghud-weather.exe"
    expect "formula" "$(cat dist/homebrew/ghud.rb)" 'bin.install "ghud-weather"'
    for pattern in "ghud_${want}_darwin_all.tar.gz" "ghud_${want}_linux_amd64.tar.gz" "ghud_${want}_linux_arm64.tar.gz" \
        "ghud_${want}_windows_amd64.zip" "ghud_${want}_windows_arm64.zip" '*.deb' '*.rpm' '*.apk' checksums.txt homebrew/ghud.rb; do
        compgen -G "dist/$pattern" >/dev/null || die "Missing dist/$pattern"
    done
    green "  archives, packages, checksums and ghud.rb present"
}

if $DRY_RUN; then bold "Releasing ghud $TAG (dry run)"; else bold "Releasing ghud $TAG"; fi

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
git tag -a "$TAG" -m "ghud $TAG"
git push origin "$TAG"
if ! GITHUB_TOKEN="$(gh auth token)" goreleaser release --clean; then
    red "GoReleaser failed after $TAG was pushed. Fix the cause and rerun:" >&2
    red "  GITHUB_TOKEN=\$(gh auth token) go run github.com/goreleaser/goreleaser/v2@$GORELEASER_VERSION release --clean" >&2
    red "or drop the tag: git push --delete origin $TAG && git tag -d $TAG" >&2
    exit 1
fi

echo
green "========================================="
green "  Released ghud $TAG"
green "========================================="
echo
echo "  GitHub:   https://github.com/$REPO/releases/tag/$TAG"
echo "  Homebrew: brew update && brew install oronbz/tap/ghud"
echo
