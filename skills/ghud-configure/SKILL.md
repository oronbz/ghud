---
name: ghud-configure
description: Set up or change ghud, the terminal dashboard. Use when adding, removing or tuning ghud Widgets and their settings, changing its theme or colours, or binding a herdr key that opens ghud.
---

# Configure ghud

ghud reads one hand-edited file, `config.toml`, and owns one machine-written file, `layout.json`, beside it. You edit `config.toml` only, the way a careful person would: surgically, keeping every comment and blank line.

## 1. Find the file

The first that applies:

1. the `--config <path>` the user runs ghud with;
2. `$GHUD_CONFIG`;
3. `$XDG_CONFIG_HOME/ghud/config.toml`, which defaults to `~/.config/ghud/config.toml` on every OS (`%AppData%\ghud\config.toml` on Windows).

No file yet: `ghud config init` writes the full reference there, every line commented out. Uncomment what the user wants.

## 2. Read the reference

Run `ghud config default`. It prints every global option, every Widget type (built-in and from installed Plugins) with each setting's kind, default, choices and the reason to change it. It is the source of truth: read it before every edit and take keys and values from it, never from memory. It reflects the ghud on this machine, Plugins included.

## 3. Edit

- **Change a setting:** edit or uncomment that one line.
- **Add a Widget:** append a `[[widget]]` block with `type` and only the settings that differ from their defaults.
- **Remove a Widget:** comment out its block with `# ` on each line.
- **The default set:** with no `[[widget]]` blocks, ghud shows one Widget of every default type. The first block you add replaces that set, so when adding the first block, also add blocks for the defaults the user wants to keep.
- **Two Widgets of one type** (two Jira queries, weather for two cities) each need their own `id`. `title` names them in the dashboard.
- **Colours:** `theme` is a string (`"ghud"` or `"terminal"`). Per-colour overrides go in `[colors.dark]` and `[colors.light]` tables, keyed by the token names the reference lists.
- **Credentials** stay out of `config.toml`. GitHub uses `gh auth login` or `GH_TOKEN`; Jira uses `ghud auth jira`, which prompts for a masked token, so ask the user to run it in their own terminal.
- **Arrangement** (order, sizes, hidden Widgets) lives in `layout.json`, which ghud rewrites. Leave it to ghud and tell the user the keys: `m` arranges the focused Widget (`hjkl` move, `HJKL` resize, `⏎` keep), dragging a title or border with the mouse does the same, `w` shows or hides Widgets.

Done when: the file parses as TOML, every key you wrote appears for its Widget type in `ghud config default`, and nothing outside your edit changed (diff it).

Running instances pick up the change at once. A key ghud can't use shows as a footer toast `config.toml:LINE: …` and puts that Widget in *needs setup*, which tells you what to fix.

## herdr key

When the user runs herdr and wants a key that opens ghud: check `herdr plugin list` shows the `ghud` plugin, and have the user run `herdr plugin install oronbz/ghud/herdr` (or `herdr plugin update ghud` when `herdr plugin action list --plugin ghud` lacks `toggle`) if not. Then append to herdr's `~/.config/herdr/config.toml`, after checking that file doesn't bind the key already, and run `herdr server reload-config`:

```toml
[[keys.command]]
key = "cmd+shift+g"
type = "plugin_action"
command = "ghud.toggle"
description = "ghud"
```

The action opens ghud over the current tab and closes it on the next press, while herdr's sidebar stays clickable. herdr handles the key, so ghud needs no `--close-key` here.

Only when the user wants no herdr plugin, bind a popup instead. It covers the sidebar and keeps all input until ghud exits, so `--close-key` must repeat the same key:

```toml
[[keys.command]]
key = "cmd+shift+g"
type = "popup"
command = "ghud --close-key cmd+shift+g"
description = "ghud"
width = "100%"
height = "100%"
```

`--close-key` takes herdr's key syntax and repeats; a two-key sequence such as `--close-key 'ctrl+b d'` also works.
