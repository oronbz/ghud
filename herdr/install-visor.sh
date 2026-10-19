#!/usr/bin/env bash
# herdr build step: install visor with Homebrew when it's missing. It never
# upgrades an existing visor and never asks anything.
set -euo pipefail

formula="oronbz/tap/visor"

if command -v visor > /dev/null 2>&1; then
  echo "visor is already installed."
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
  done <<< "${VISOR_BREW_FALLBACKS-/opt/homebrew/bin/brew:/usr/local/bin/brew:/home/linuxbrew/.linuxbrew/bin/brew}:"
  return 1
}

if ! brew="$(find_brew)"; then
  read -r -d '' msg <<MSG || true
visor isn't installed, and Homebrew wasn't found on PATH or in its usual places.
Install visor one of these ways, then run herdr plugin install oronbz/visor/herdr again:

  Homebrew (macOS and Linux): install it from https://brew.sh, then run
    brew install $formula
  Linux packages (deb, rpm, apk) and archives for every platform:
    https://github.com/oronbz/visor/releases
MSG
  printf '%s\n' "$msg" >&2
  exit 1
fi

# herdr's build environment may not have brew's bin on PATH.
if [ -x "$("$brew" --prefix)/bin/visor" ]; then
  echo "visor is already installed in $("$brew" --prefix)/bin."
  exit 0
fi

# brew's own output (auto-update news, hints, other taps' warnings) is shown
# only when the install fails.
echo "Installing visor with Homebrew..."
if ! out="$(HOMEBREW_NO_INSTALL_UPGRADE=1 HOMEBREW_NO_ENV_HINTS=1 "$brew" install "$formula" 2>&1)"; then
  printf 'brew install %s failed:\n%s\n' "$formula" "$out" >&2
  exit 1
fi
echo "visor is installed. Run visor, or bind it to a key in herdr (see https://github.com/oronbz/visor#herdr)."
