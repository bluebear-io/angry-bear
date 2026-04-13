// scroll.go provides a reusable scroll viewport for lists.
// All scrollable panels in the TUI should use this instead of ad-hoc
// scroll logic — fixes scrolling once, works everywhere.
package tui

// ScrollView tracks cursor position and viewport offset for a scrollable list.
// The viewport follows the cursor: scrolling up or down always keeps the
// cursor visible, and the viewport stays anchored when all items fit.
type ScrollView struct {
	Cursor int // Index of the focused item
	Offset int // First visible item index (scroll offset)
}

// Update recalculates the viewport offset to keep Cursor visible
// within a viewport of the given height. Call this after any cursor change.
//
// total: number of items in the list
// height: number of visible rows in the viewport
func (sv *ScrollView) Update(total, height int) {
	// Clamp cursor to valid range
	if sv.Cursor < 0 {
		sv.Cursor = 0
	}
	if total > 0 && sv.Cursor >= total {
		sv.Cursor = total - 1
	}

	// If everything fits, no scrolling needed
	if total <= height {
		sv.Offset = 0
		return
	}

	// Cursor above viewport — scroll up
	if sv.Cursor < sv.Offset {
		sv.Offset = sv.Cursor
	}

	// Cursor below viewport — scroll down
	if sv.Cursor >= sv.Offset+height {
		sv.Offset = sv.Cursor - height + 1
	}

	// Clamp offset
	if sv.Offset < 0 {
		sv.Offset = 0
	}
	maxOffset := total - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if sv.Offset > maxOffset {
		sv.Offset = maxOffset
	}
}

// MoveUp moves the cursor up by 1, skipping items via the skip function.
// skip(index) returns true for items that should be skipped (e.g., section headers).
func (sv *ScrollView) MoveUp(total int, skip func(int) bool) {
	next := sv.Cursor - 1
	for next >= 0 && skip(next) {
		next--
	}
	if next >= 0 {
		sv.Cursor = next
	}
}

// MoveDown moves the cursor down by 1, skipping items via the skip function.
func (sv *ScrollView) MoveDown(total int, skip func(int) bool) {
	next := sv.Cursor + 1
	for next < total && skip(next) {
		next++
	}
	if next < total {
		sv.Cursor = next
	}
}

// PageUp moves the cursor up by one page.
func (sv *ScrollView) PageUp(height int) {
	sv.Cursor -= height
	if sv.Cursor < 0 {
		sv.Cursor = 0
	}
}

// PageDown moves the cursor down by one page.
func (sv *ScrollView) PageDown(total, height int) {
	sv.Cursor += height
	if total > 0 && sv.Cursor >= total {
		sv.Cursor = total - 1
	}
}

// JumpTop moves the cursor to the first item.
func (sv *ScrollView) JumpTop() {
	sv.Cursor = 0
}

// JumpBottom moves the cursor to the last item.
func (sv *ScrollView) JumpBottom(total int) {
	if total > 0 {
		sv.Cursor = total - 1
	}
}

// VisibleRange returns the start (inclusive) and end (exclusive) indices
// of the items that should be rendered in the viewport.
func (sv *ScrollView) VisibleRange(total, height int) (start, end int) {
	sv.Update(total, height)
	start = sv.Offset
	end = sv.Offset + height
	if end > total {
		end = total
	}
	return start, end
}
