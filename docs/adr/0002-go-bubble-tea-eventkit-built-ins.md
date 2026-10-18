# Go + Bubble Tea v2; Apple sources are darwin-only built-ins via go-eventkit

ghud is written in Go on Bubble Tea v2 (with Lip Gloss v2 and bubbles v2). Once plugins became out-of-process data producers ([ADR 0001](0001-host-drawn-out-of-process-plugins.md)), the framework only had to serve the approved Panels design, and Bubble Tea v2 ships what it needs: layer hit-testing for drag and resize, synchronized output, OSC 8, kitty keyboard flag 8 for lone Shift/Ctrl, a multi-line `textarea`, and `tea.ExecProcess` for the `$EDITOR` handoff. Reminders and Meetings read EventKit through `BRO3886/go-eventkit` (cgo) in files behind `//go:build darwin`, so those Widget types exist only in the macOS binary; the TCC grant goes to the host terminal. This route already ships in `oronbz/nag`.

## Considered Options

- **Ratatui (Rust)**: equal on keyboard and text editing, and `objc2-event-kit` covers EventKit, but hit-testing, sync-output wrapping, OSC 8 and editor suspend are glue we'd write. Its wins (ratatui-image, tachyonfx) are images and effects, which v1 dropped. mirador, the closest prior art, is Rust but reads calendars only from a local `.ics` file, so it offers nothing for Apple sources.
- **OpenTUI, Textual, Ink, libvaxis**: rejected on 0.x churn plus a Bun toolchain, missing key-release events and no single binary, no mouse, and no layout layer respectively.
- **Swift EventKit plugin over the JSON-RPC protocol**: workable, but a second toolchain and binary for no gain over go-eventkit. Third parties can still write one.
- **Google `.ics` secret address or Calendar API instead of EventKit**: the secret address can be disabled by Workspace admins (it is on the author's work account), and the API needs a bring-your-own OAuth client.

## Consequences

- darwin builds need cgo, so the macOS release builds on macOS; linux and windows builds are pure Go.
- Bubble Tea's renderer, Ultraviolet, is untagged; pin versions.
