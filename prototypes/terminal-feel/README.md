# PROTOTYPE: terminal feel spike (throwaway)

Answers [Terminal feel spike](https://github.com/oronbz/ghud/issues/11). Not production code.

    cd prototypes/terminal-feel && go run .

Run it in Ghostty standalone, then inside a herdr pane. The **Probe** panel shows the kitty flags the terminal replied with, held modifiers, the last hit-test zone and an event log.

Checklist:

1. **Lone modifiers**: tap and hold Shift, then Ctrl. The footer swaps to the SHIFT / CTRL hints while held and returns on release; Probe logs `press leftshift (lone modifier)` and `release leftshift`.
2. **Drag to move**: drag a panel title. A dashed ghost follows, a heavy bar marks the drop side; drop left/right joins that row, top/bottom opens a new row.
3. **Drag to resize**: hover a shared vertical border (both edges light up), drag it; same for the border between rows. Content reflows live; the clock and CPU chart keep animating.
4. **Keyboard**: starts unfocused. `Tab`/`S-Tab` next/previous row, `h/l` (←→) Widgets in the row, `esc` unfocus, `j/k` select PRs, `m` arrange (`hjkl` move, `HJKL` resize, `⏎` keep, `esc` revert).
5. **$EDITOR**: focus Notes (`3` or click), `E` opens `$EDITOR`; quit it and the dashboard must come back intact with the edited text. `i` edits inline, `esc` saves.
6. Wheel over the PR list scrolls without moving focus; double-click and right-click show a message in the footer. Inside herdr the spike opts its pane into right-click passthrough on start and restores it on exit.
