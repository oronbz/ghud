#!/usr/bin/env bash
# herdr action: open visor over the current tab, or close it when it's already
# open there. visor open in another tab is closed and opened here instead, so
# the key always shows visor where you are.
set -euo pipefail

herdr="${HERDR_BIN_PATH:-herdr}"
state="$HERDR_PLUGIN_STATE_DIR/pane"

if [ -f "$state" ]; then
  pane="$(cat "$state")"
  rm -f "$state"
  # A visor that quit with q left its id behind; pane get fails for it.
  if info="$("$herdr" pane get "$pane" 2> /dev/null)"; then
    "$herdr" plugin pane close "$pane" > /dev/null
    case "$info" in *"\"tab_id\":\"${HERDR_TAB_ID:-}\""*) exit 0 ;; esac
  fi
fi

opened="$("$herdr" plugin pane open --plugin "$HERDR_PLUGIN_ID" --entrypoint visor)"
pane="$(printf '%s' "$opened" | sed -n 's/.*"pane_id":"\([^"]*\)".*/\1/p')"
if [ -z "$pane" ]; then
  printf 'herdr plugin pane open gave no pane id:\n%s\n' "$opened" >&2
  exit 1
fi
printf '%s\n' "$pane" > "$state"
