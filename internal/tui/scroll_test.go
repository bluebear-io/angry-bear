package tui

import "testing"

func TestScrollView_AllItemsFit(t *testing.T) {
	sv := ScrollView{Cursor: 3}
	start, end := sv.VisibleRange(5, 10)
	if start != 0 || end != 5 {
		t.Errorf("got range [%d,%d), want [0,5)", start, end)
	}
	if sv.Offset != 0 {
		t.Errorf("offset = %d, want 0 when all items fit", sv.Offset)
	}
}

func TestScrollView_ScrollDown(t *testing.T) {
	sv := ScrollView{Cursor: 0}
	// Move cursor past visible area
	sv.Cursor = 12
	start, end := sv.VisibleRange(20, 10)
	if sv.Cursor < start || sv.Cursor >= end {
		t.Errorf("cursor %d not in visible range [%d,%d)", sv.Cursor, start, end)
	}
}

func TestScrollView_ScrollUp(t *testing.T) {
	sv := ScrollView{Cursor: 15, Offset: 10}
	// Move cursor above viewport
	sv.Cursor = 5
	start, end := sv.VisibleRange(20, 10)
	if sv.Cursor < start || sv.Cursor >= end {
		t.Errorf("cursor %d not in visible range [%d,%d)", sv.Cursor, start, end)
	}
}

func TestScrollView_MoveUpSkips(t *testing.T) {
	sv := ScrollView{Cursor: 3}
	// Skip index 2
	sv.MoveUp(5, func(i int) bool { return i == 2 })
	if sv.Cursor != 1 {
		t.Errorf("cursor = %d, want 1 (skipped 2)", sv.Cursor)
	}
}

func TestScrollView_MoveDownSkips(t *testing.T) {
	sv := ScrollView{Cursor: 1}
	// Skip index 2
	sv.MoveDown(5, func(i int) bool { return i == 2 })
	if sv.Cursor != 3 {
		t.Errorf("cursor = %d, want 3 (skipped 2)", sv.Cursor)
	}
}

func TestScrollView_PageUpDown(t *testing.T) {
	sv := ScrollView{Cursor: 15}
	sv.PageUp(10)
	if sv.Cursor != 5 {
		t.Errorf("after PageUp: cursor = %d, want 5", sv.Cursor)
	}
	sv.PageDown(20, 10)
	if sv.Cursor != 15 {
		t.Errorf("after PageDown: cursor = %d, want 15", sv.Cursor)
	}
}

func TestScrollView_JumpTopBottom(t *testing.T) {
	sv := ScrollView{Cursor: 10}
	sv.JumpTop()
	if sv.Cursor != 0 {
		t.Errorf("JumpTop: cursor = %d, want 0", sv.Cursor)
	}
	sv.JumpBottom(20)
	if sv.Cursor != 19 {
		t.Errorf("JumpBottom: cursor = %d, want 19", sv.Cursor)
	}
}

func TestScrollView_ClampsCursor(t *testing.T) {
	sv := ScrollView{Cursor: 50}
	sv.Update(10, 5)
	if sv.Cursor != 9 {
		t.Errorf("cursor = %d, want 9 (clamped to total-1)", sv.Cursor)
	}

	sv.Cursor = -5
	sv.Update(10, 5)
	if sv.Cursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped to 0)", sv.Cursor)
	}
}

func TestScrollView_PageUpClampsToZero(t *testing.T) {
	sv := ScrollView{Cursor: 3}
	sv.PageUp(10) // 3 - 10 = -7, should clamp to 0
	if sv.Cursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped after large PageUp)", sv.Cursor)
	}
}

func TestScrollView_PageDownClampsToMax(t *testing.T) {
	sv := ScrollView{Cursor: 8}
	sv.PageDown(10, 5) // 8 + 5 = 13, should clamp to 9
	if sv.Cursor != 9 {
		t.Errorf("cursor = %d, want 9 (clamped after PageDown past end)", sv.Cursor)
	}
}

func TestScrollView_PageUpAlreadyAtTop(t *testing.T) {
	sv := ScrollView{Cursor: 0}
	sv.PageUp(5)
	if sv.Cursor != 0 {
		t.Errorf("cursor = %d, want 0 (already at top)", sv.Cursor)
	}
}

func TestScrollView_PageDownAlreadyAtBottom(t *testing.T) {
	sv := ScrollView{Cursor: 9}
	sv.PageDown(10, 5)
	if sv.Cursor != 9 {
		t.Errorf("cursor = %d, want 9 (already at bottom)", sv.Cursor)
	}
}

func TestScrollView_JumpBottomZeroTotal(t *testing.T) {
	sv := ScrollView{Cursor: 0}
	sv.JumpBottom(0)
	if sv.Cursor != 0 {
		t.Errorf("cursor = %d, want 0 (no items)", sv.Cursor)
	}
}

func TestScrollView_MoveUpAtTopStays(t *testing.T) {
	sv := ScrollView{Cursor: 0}
	noSkip := func(int) bool { return false }
	sv.MoveUp(5, noSkip)
	if sv.Cursor != 0 {
		t.Errorf("cursor = %d, want 0 (at top, should not go negative)", sv.Cursor)
	}
}

func TestScrollView_MoveDownAtBottomStays(t *testing.T) {
	sv := ScrollView{Cursor: 4}
	noSkip := func(int) bool { return false }
	sv.MoveDown(5, noSkip)
	if sv.Cursor != 4 {
		t.Errorf("cursor = %d, want 4 (at bottom, should not exceed total-1)", sv.Cursor)
	}
}

func TestScrollView_UpdateWithZeroTotal(t *testing.T) {
	sv := ScrollView{Cursor: 5}
	sv.Update(0, 10)
	// With zero items, cursor should be clamped. The code checks total > 0,
	// so with total=0 it just clamps offset.
	if sv.Offset != 0 {
		t.Errorf("offset = %d, want 0 for zero total", sv.Offset)
	}
}

func TestScrollView_VisibleRangeClamps(t *testing.T) {
	sv := ScrollView{Cursor: 5}
	start, end := sv.VisibleRange(3, 10) // cursor way beyond total
	// Should clamp and return valid range
	if start < 0 || end > 3 {
		t.Errorf("range [%d,%d) out of bounds for total=3", start, end)
	}
}
