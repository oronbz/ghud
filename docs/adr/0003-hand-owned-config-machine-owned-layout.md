# The user owns `config.toml`; visor owns `layout.json`

visor keeps Widgets and their settings in a hand-edited, fully commented `config.toml` and keeps the Layout (rows, weights, Widget order, hidden Widgets) in a separate `layout.json`, both in `~/.config/visor`. Arranging, which writes on every drag and resize, only ever rewrites `layout.json`. The config is never re-serialized: the TUI only appends a `[[widget]]` block when adding a Widget and comments one out when removing it, as text edits verified by re-parsing, so the user's comments and formatting survive. The two files join on Widget ids, which visor always writes explicitly and which default to the Widget type name when a hand-written block omits one.

## Considered Options

- **Layout inside the config (mirador)**: one file to read, but every drag becomes a text edit of the file the user hand-writes, and no mature Go TOML library round-trips comments (only pre-1.0 `creachadair/tomledit` does).
- **JSONC via `tailscale/hujson`**: byte-exact edits, but less pleasant to hand-write than TOML, which `visor-plugin.toml` and herdr already use.

## Consequences

- A Widget in the config but not in the Layout is placed at the end; a Layout entry with no matching Widget is dropped. Renaming an id orphans its saved position and cached frame.
- New options reach users through `visor config default` (the full commented reference), never by visor rewriting their config.

## Notes

- 2026-10-20: The Layout is no longer only rows and weights. A cell is a Widget (`{"id", "weight"}`) or a Stack (`{"weight", "rows": [...]}`) holding rows of its own, to any depth, which makes full-height columns possible (#58). Today's `layout.json` files are unchanged by this: they load as they are and save back byte for byte. visor normalises the Layout after every change, so the file never collects empty rows, one-row Stacks or rows holding only a Stack.
