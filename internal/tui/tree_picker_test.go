// tree_picker_test.go tests the file tree browser for path selection.
// Covers navigation, selection, directory traversal, glob pattern generation,
// extension selection, and ignore set filtering.
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// --- NewTreePicker tests ---

func TestNewTreePicker_ListsEntries(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("pkg"), 0o644)

	tp := NewTreePicker(dir, DefaultStyles())
	if len(tp.entries) < 2 {
		t.Errorf("expected at least 2 entries (dir + file), got %d", len(tp.entries))
	}

	// Directories should come first
	if len(tp.entries) > 0 && !tp.entries[0].isDir {
		t.Error("expected first entry to be a directory")
	}
}

func TestNewTreePicker_FiltersIgnoredDirs(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)

	tp := NewTreePicker(dir, DefaultStyles())
	for _, entry := range tp.entries {
		if entry.name == "node_modules" || entry.name == ".git" {
			t.Errorf("expected %q to be filtered out", entry.name)
		}
	}
}

func TestNewTreePickerWithIgnore_CustomIgnoreSet(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.MkdirAll(filepath.Join(dir, "custom_ignore"), 0o755)

	ignore := map[string]bool{"custom_ignore": true}
	tp := NewTreePickerWithIgnore(dir, ignore, DefaultStyles())
	for _, entry := range tp.entries {
		if entry.name == "custom_ignore" {
			t.Error("expected custom_ignore to be filtered out by custom ignore set")
		}
	}
}

// --- Navigation tests ---

func TestTreePicker_UpDownNavigation(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "c.go"), []byte(""), 0o644)

	tp := NewTreePicker(dir, DefaultStyles())
	if tp.focusIndex != 0 {
		t.Fatalf("initial focusIndex = %d, want 0", tp.focusIndex)
	}

	// Down
	m, _ := tp.Update(tea.KeyMsg{Type: tea.KeyDown})
	tp = m.(TreePicker)
	if tp.focusIndex != 1 {
		t.Errorf("focusIndex = %d, want 1 after down", tp.focusIndex)
	}

	// Up
	m, _ = tp.Update(tea.KeyMsg{Type: tea.KeyUp})
	tp = m.(TreePicker)
	if tp.focusIndex != 0 {
		t.Errorf("focusIndex = %d, want 0 after up", tp.focusIndex)
	}

	// Up at 0 stays at 0
	m, _ = tp.Update(tea.KeyMsg{Type: tea.KeyUp})
	tp = m.(TreePicker)
	if tp.focusIndex != 0 {
		t.Errorf("focusIndex = %d, want 0 (should not go negative)", tp.focusIndex)
	}
}

func TestTreePicker_DownAtBottomStays(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "only.go"), []byte(""), 0o644)

	tp := NewTreePicker(dir, DefaultStyles())
	// Only 1 entry, so down should stay at 0
	m, _ := tp.Update(tea.KeyMsg{Type: tea.KeyDown})
	tp = m.(TreePicker)
	if tp.focusIndex != 0 {
		t.Errorf("focusIndex = %d, want 0 (at bottom)", tp.focusIndex)
	}
}

// --- Enter on directory descends ---

func TestTreePicker_EnterOnDirDescends(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "subdir", "nested.go"), []byte(""), 0o644)

	tp := NewTreePicker(dir, DefaultStyles())
	// First entry should be "subdir" (directory first)
	if len(tp.entries) == 0 || !tp.entries[0].isDir {
		t.Skip("no directory entry to test")
	}

	m, _ := tp.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tp = m.(TreePicker)
	if tp.currentDir != filepath.Join(dir, "subdir") {
		t.Errorf("currentDir = %q, want subdir path", tp.currentDir)
	}
	if tp.focusIndex != 0 {
		t.Errorf("focusIndex = %d, want 0 after descending", tp.focusIndex)
	}
}

// --- Enter on file selects and returns pattern ---

func TestTreePicker_EnterOnFileSelectsPattern(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(""), 0o644)

	tp := NewTreePicker(dir, DefaultStyles())
	// File should be the only entry
	if len(tp.entries) == 0 || tp.entries[0].isDir {
		t.Skip("no file entry to test")
	}

	_, cmd := tp.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from selecting a file")
	}
	msg := cmd()
	done, ok := msg.(treePickerDoneMsg)
	if !ok {
		t.Fatalf("expected treePickerDoneMsg, got %T", msg)
	}
	if done.pattern != "main.go" {
		t.Errorf("pattern = %q, want %q", done.pattern, "main.go")
	}
}

// --- Backspace goes up ---

func TestTreePicker_BackspaceGoesUp(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "child"), 0o755)

	tp := NewTreePicker(dir, DefaultStyles())
	// Descend first
	tp.currentDir = filepath.Join(dir, "child")
	tp.entries = tp.listEntries(tp.currentDir)

	m, _ := tp.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	tp = m.(TreePicker)
	if tp.currentDir != dir {
		t.Errorf("currentDir = %q, want %q after backspace", tp.currentDir, dir)
	}
}

func TestTreePicker_BackspaceAtRootDoesNothing(t *testing.T) {
	dir := t.TempDir()
	tp := NewTreePicker(dir, DefaultStyles())

	m, _ := tp.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	tp = m.(TreePicker)
	if tp.currentDir != tp.rootDir {
		t.Errorf("currentDir = %q, want %q (should not go above root)", tp.currentDir, tp.rootDir)
	}
}

// --- 'd' selects directory as glob ---

func TestTreePicker_DKeySelectsCurrentDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)

	tp := NewTreePicker(dir, DefaultStyles())
	tp.currentDir = filepath.Join(dir, "sub")

	_, cmd := tp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatal("expected command from 'd'")
	}
	msg := cmd()
	done := msg.(treePickerDoneMsg)
	if done.pattern != "sub/**" {
		t.Errorf("pattern = %q, want %q", done.pattern, "sub/**")
	}
}

func TestTreePicker_DKeyAtRootSelectsAllFiles(t *testing.T) {
	dir := t.TempDir()
	tp := NewTreePicker(dir, DefaultStyles())

	_, cmd := tp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatal("expected command from 'd' at root")
	}
	msg := cmd()
	done := msg.(treePickerDoneMsg)
	if done.pattern != "**" {
		t.Errorf("pattern = %q, want %q", done.pattern, "**")
	}
}

// --- 'e' selects by extension ---

func TestTreePicker_EKeySelectsByExtension(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "test.go"), []byte(""), 0o644)

	tp := NewTreePicker(dir, DefaultStyles())
	// Focus should be on test.go
	if len(tp.entries) == 0 {
		t.Skip("no entries")
	}

	_, cmd := tp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if cmd == nil {
		t.Fatal("expected command from 'e' on file with extension")
	}
	msg := cmd()
	done := msg.(treePickerDoneMsg)
	if done.pattern != "**/*.go" {
		t.Errorf("pattern = %q, want %q", done.pattern, "**/*.go")
	}
}

func TestTreePicker_EKeyOnDirDoesNothing(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)

	tp := NewTreePicker(dir, DefaultStyles())
	// Focus is on subdir (a directory)
	if len(tp.entries) == 0 || !tp.entries[0].isDir {
		t.Skip("no directory entry")
	}

	_, cmd := tp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if cmd != nil {
		t.Error("expected no command from 'e' on directory")
	}
}

// --- Esc cancels ---

func TestTreePicker_EscCancels(t *testing.T) {
	dir := t.TempDir()
	tp := NewTreePicker(dir, DefaultStyles())

	_, cmd := tp.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected command from esc")
	}
	msg := cmd()
	done, ok := msg.(treePickerDoneMsg)
	if !ok {
		t.Fatalf("expected treePickerDoneMsg, got %T", msg)
	}
	if done.pattern != "" {
		t.Errorf("pattern = %q, want empty (cancel)", done.pattern)
	}
}

// --- View tests ---

func TestTreePicker_ViewShowsHeader(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "test.go"), []byte(""), 0o644)

	tp := NewTreePicker(dir, DefaultStyles())
	output := tp.View()

	if !strings.Contains(output, "Select Path") {
		t.Error("expected 'Select Path' header")
	}
}

func TestTreePicker_ViewEmptyDir(t *testing.T) {
	dir := t.TempDir()
	tp := NewTreePicker(dir, DefaultStyles())
	output := tp.View()

	if !strings.Contains(output, "empty directory") {
		t.Error("expected 'empty directory' message for empty dir")
	}
}

func TestTreePicker_ViewShowsBackspaceHintWhenNotAtRoot(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "child"), 0o755)

	tp := NewTreePicker(dir, DefaultStyles())
	tp.currentDir = filepath.Join(dir, "child")
	tp.entries = tp.listEntries(tp.currentDir)

	output := tp.View()
	if !strings.Contains(output, "backspace") {
		t.Error("expected backspace hint when not at root")
	}
}

// --- toRelativePath tests ---

func TestToRelativePath(t *testing.T) {
	tp := TreePicker{rootDir: "/home/user/project"}

	got := tp.toRelativePath("/home/user/project/src/main.go")
	if got != "src/main.go" {
		t.Errorf("toRelativePath = %q, want %q", got, "src/main.go")
	}

	got = tp.toRelativePath("/home/user/project")
	if got != "." {
		t.Errorf("toRelativePath(root) = %q, want %q", got, ".")
	}
}

// --- isIgnored tests ---

func TestIsIgnored(t *testing.T) {
	tp := TreePicker{ignoreSet: map[string]bool{"node_modules": true, ".git": true}}

	if !tp.isIgnored("node_modules") {
		t.Error("expected node_modules to be ignored")
	}
	if !tp.isIgnored(".git") {
		t.Error("expected .git to be ignored")
	}
	if tp.isIgnored("src") {
		t.Error("expected src to not be ignored")
	}
}

// --- listEntries sorting ---

func TestListEntries_DirsBeforeFiles(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "aaa.go"), []byte(""), 0o644)
	os.MkdirAll(filepath.Join(dir, "zzz_dir"), 0o755)

	tp := TreePicker{rootDir: dir, ignoreSet: DefaultIgnoreSet}
	entries := tp.listEntries(dir)

	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}

	// Directory should come before file regardless of name
	if !entries[0].isDir {
		t.Error("expected directory to come first")
	}
}
