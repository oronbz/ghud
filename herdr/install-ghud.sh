#!/usr/bin/env bash
# herdr build step: install ghud with Homebrew when it's missing. It never
# upgrades an existing ghud and never asks anything.
set -euo pipefail

formula="oronbz/tap/ghud"

if command -v ghud > /dev/null 2>&1; then
  echo "ghud is already installed."
  exit 0
fi

find_brew() {
  command -v brew && return 0
  local candidate
  while IFS= read -r -d : candidate; do
    if [ -n "$candidate" ] && [ -x "$candidate" ]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done <<< "${GHUD_BREW_FALLBACKS-/opt/homebrew/bin/brew:/usr/local/bin/brew:/home/linuxbrew/.linuxbrew/bin/brew}:"
  return 1
}

if ! brew="$(find_brew)"; then
  read -r -d '' msg <<MSG || true
ghud isn't installed, and Homebrew wasn't found on PATH or in its usual places.
Install ghud one of these ways, then run herdr plugin install oronbz/ghud/herdr again:

  Homebrew (macOS and Linux): install it from https://brew.sh, then run
    brew install $formula
  Linux packages (deb, rpm, apk) and archives for every platform:
    https://github.com/oronbz/ghud/releases
MSG
  printf '%s\n' "$msg" >&2
  exit 1
fi

# herdr's build environment may not have brew's bin on PATH.
if [ -x "$("$brew" --prefix)/bin/ghud" ]; then
  echo "ghud is already installed in $("$brew" --prefix)/bin."
  exit 0
fi

echo "Installing ghud: $brew install $formula"
HOMEBREW_NO_INSTALL_UPGRADE=1 "$brew" install "$formula"
echo "ghud is installed. Run ghud, or bind it to a key in herdr (see https://github.com/oronbz/ghud#herdr)."
