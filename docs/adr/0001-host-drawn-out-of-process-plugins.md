# Plugins describe, the host draws; external plugins are JSON-RPC child processes

Plugins never draw cells. List Widgets send Items as typed fields with column priorities and the host renders the rows; live views send styled lines with theme tokens and host-expanded helpers (big digits, braille chart, bar). The host owns selection, focus dimming, column dropping, key hints, the Action palette, right-click menus and text input, because those behaviours only work uniformly if the host knows where every Item is. External plugins are a directory with a `visor-plugin.toml` manifest and one long-lived process per Plugin speaking JSON-RPC 2.0 over stdio, unsandboxed: once the host draws, a plugin is only a data producer, so "any executable that speaks JSON" is the lowest authoring bar and keeps plugins independent of the host's language and TUI framework.

## Considered Options

- **Plugins draw into a rect**: rejected; every plugin would reimplement selection, dimming and hit-testing, and the host couldn't build the palette.
- **Fixed host component vocabulary**: rejected; every new view kind would need a host release.
- **WASM (Zellij)**: sandboxed, but forces a WASM toolchain on authors and needs host-granted network and file access.
- **Embedded Lua**: easy to write, but locks plugins into one language and whatever the host exposes.
- **mirador's protocol, or compatibility with it**: no request/response matching (needed for choice menus and Action results), no Items, groups, typed Actions or status; no mirador plugins existed to be compatible with.

## Consequences

- Built-in Widget types implement the same internal contract; a script Widget type (a command plus an interval that prints JSON) is the easy tier on the same contract.
- Installing a plugin means trusting it; there is no sandbox.
- The repo ships a JSON Schema for the protocol and manifest, with `visor plugin check` as a conformance test.
