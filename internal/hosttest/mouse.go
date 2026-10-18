package hosttest

import tea "charm.land/bubbletea/v2"

// Click presses and releases the left button at a cell, the way an SGR
// terminal reports it.
func (h *Host) Click(x, y int) {
	h.t.Helper()
	h.Send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	h.Send(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// RightClick presses and releases the right button at a cell.
func (h *Host) RightClick(x, y int) {
	h.t.Helper()
	h.Send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseRight})
	h.Send(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseRight})
}

// Hover moves the pointer to a cell with no button held (mode 1003 motion).
func (h *Host) Hover(x, y int) {
	h.t.Helper()
	h.Send(tea.MouseMotionMsg{X: x, Y: y})
}

// Grab presses the left button at a cell and holds it.
func (h *Host) Grab(x, y int) {
	h.t.Helper()
	h.Send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// DragTo moves the pointer to a cell with the left button held.
func (h *Host) DragTo(x, y int) {
	h.t.Helper()
	h.Send(tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// Drop releases the left button at a cell.
func (h *Host) Drop(x, y int) {
	h.t.Helper()
	h.Send(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// Drag grabs a cell, drags to another and drops there.
func (h *Host) Drag(fromX, fromY, toX, toY int) {
	h.t.Helper()
	h.Grab(fromX, fromY)
	h.DragTo(toX, toY)
	h.Drop(toX, toY)
}

// Wheel turns the wheel over a cell: n notches down, or -n up.
func (h *Host) Wheel(x, y, n int) {
	h.t.Helper()
	b := tea.MouseWheelDown
	if n < 0 {
		b, n = tea.MouseWheelUp, -n
	}
	for range n {
		h.Send(tea.MouseWheelMsg{X: x, Y: y, Button: b})
	}
}
