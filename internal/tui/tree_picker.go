// tree_picker.go implements a filtered file tree browser for selecting a path
// to use in a rule's glob pattern. It navigates directories starting from
// the project root, hides ignored directories, and generates glob patterns
// from user selections.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// DefaultIgnorePatterns lists directory names that are hidden by default
// in the tree picker to reduce noise.
var DefaultIgnorePatterns = []string{
	".git", "node_modules", "vendor", "dist", ".next",
	"__pycache__", ".venv", "build", "target",
}

// treeEntry represents a single entry (file or directory) in the tree picker.
type treeEntry struct {
	name  string // Display name (basename)
	path  string // Absolute path
	isDir bool   // Whether this entry is a directory
}

// TreePicker provides a file tree browser for selecting paths.
// It implements tea.Model for use as a child model within the App.
type TreePicker struct {
	rootDir        string        // Project root directory
	currentDir     string        // Currently displayed directory
	entries        []treeEntry   // Visible entries in currentDir
	focusIndex     int           // Currently focused entry index
	ignorePatterns []string      // Directory names to hide
	selectedPath   string        // Final selection (absolute path)
	generatedPattern string      // Glob pattern generated from selection
	confirmed      bool          // Whether the user has confirmed the selection
	styles         Styles        // Style definitions
}

// NewTreePicker creates a new TreePicker rooted at the given directory.
// It uses DefaultIgnorePatterns for filtering.
func NewTreePicker(rootDir string, styles Styles) TreePicker {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		absRoot = rootDir
	}

	tp := TreePicker{
		rootDir:        absRoot,
		currentDir:     absRoot,
		ignorePatterns: DefaultIgnorePatterns,
		styles:         styles,
	}
	tp.entries = tp.listEntries(absRoot)
	return tp
}

// NewTreePickerWithIgnore creates a TreePicker with custom ignore patterns.
func NewTreePickerWithIgnore(rootDir string, ignorePatterns []string, styles Styles) TreePicker {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		absRoot = rootDir
	}

	tp := TreePicker{
		rootDir:        absRoot,
		currentDir:     absRoot,
		ignorePatterns: ignorePatterns,
		styles:         styles,
	}
	tp.entries = tp.listEntries(absRoot)
	return tp
}

// listEntries reads the given directory and returns visible entries,
// filtering out ignored directories. Results are sorted: directories first,
// then files, both alphabetically.
func (tp TreePicker) listEntries(dir string) []treeEntry {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var dirs, files []treeEntry
	for _, de := range dirEntries {
		name := de.Name()

		// Skip hidden files/dirs (starting with .) unless they're in special cases.
		if strings.HasPrefix(name, ".") && tp.isIgnored(name) {
			continue
		}

		// Skip ignored directories.
		if de.IsDir() && tp.isIgnored(name) {
			continue
		}

		entry := treeEntry{
			name:  name,
			path:  filepath.Join(dir, name),
			isDir: de.IsDir(),
		}

		if de.IsDir() {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}

	// Sort each group alphabetically.
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

	// Directories first, then files.
	return append(dirs, files...)
}

// isIgnored checks if a name matches any of the ignore patterns.
func (tp TreePicker) isIgnored(name string) bool {
	for _, pattern := range tp.ignorePatterns {
		if name == pattern {
			return true
		}
	}
	return false
}

// Init returns the initial command for the tree picker.
func (tp TreePicker) Init() tea.Cmd {
	return nil
}

// Update handles key input for the tree picker.
func (tp TreePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Cancel and return with no selection.
			return tp, func() tea.Msg {
				return treePickerDoneMsg{pattern: ""}
			}

		case "up", "k":
			if tp.focusIndex > 0 {
				tp.focusIndex--
			}
			return tp, nil

		case "down", "j":
			if tp.focusIndex < len(tp.entries)-1 {
				tp.focusIndex++
			}
			return tp, nil

		case "enter":
			if tp.focusIndex >= 0 && tp.focusIndex < len(tp.entries) {
				entry := tp.entries[tp.focusIndex]
				if entry.isDir {
					// Descend into the directory.
					tp.currentDir = entry.path
					tp.entries = tp.listEntries(entry.path)
					tp.focusIndex = 0
					return tp, nil
				}
				// File selected: generate relative path pattern.
				pattern := tp.toRelativePath(entry.path)
				return tp, func() tea.Msg {
					return treePickerDoneMsg{pattern: pattern}
				}
			}
			return tp, nil

		case "backspace":
			// Go up to parent directory if not at root.
			if tp.currentDir != tp.rootDir {
				tp.currentDir = filepath.Dir(tp.currentDir)
				tp.entries = tp.listEntries(tp.currentDir)
				tp.focusIndex = 0
			}
			return tp, nil

		case "d":
			// Select the current directory as a glob pattern.
			pattern := tp.toRelativePath(tp.currentDir) + "/**"
			if tp.currentDir == tp.rootDir {
				pattern = "**"
			}
			return tp, func() tea.Msg {
				return treePickerDoneMsg{pattern: pattern}
			}

		case "e":
			// Select by extension if a file is focused.
			if tp.focusIndex >= 0 && tp.focusIndex < len(tp.entries) {
				entry := tp.entries[tp.focusIndex]
				if !entry.isDir {
					ext := filepath.Ext(entry.name)
					if ext != "" {
						pattern := "**/*" + ext
						return tp, func() tea.Msg {
							return treePickerDoneMsg{pattern: pattern}
						}
					}
				}
			}
			return tp, nil
		}
	}

	return tp, nil
}

// View renders the tree picker showing the current directory contents.
func (tp TreePicker) View() string {
	var b strings.Builder

	// Header with current directory path.
	relDir := tp.toRelativePath(tp.currentDir)
	if relDir == "" {
		relDir = "."
	}
	header := tp.styles.Header.Render(fmt.Sprintf("Select Path - %s", relDir))
	b.WriteString(header)
	b.WriteString("\n")

	if tp.currentDir != tp.rootDir {
		b.WriteString(tp.styles.Description.Render("  (backspace to go up)"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if len(tp.entries) == 0 {
		b.WriteString(tp.styles.Description.Render("  (empty directory)"))
		b.WriteString("\n")
		return b.String()
	}

	for i, entry := range tp.entries {
		prefix := "  "
		if i == tp.focusIndex {
			prefix = "> "
		}

		icon := "  "
		if entry.isDir {
			icon = "/ "
		}

		line := fmt.Sprintf("%s%s%s", prefix, entry.name, icon)

		if i == tp.focusIndex {
			line = tp.styles.Selected.Render(line)
		} else if entry.isDir {
			line = tp.styles.SkillName.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(tp.styles.Description.Render("  enter: select file | d: select current dir | e: select by extension | backspace: go up"))

	return b.String()
}

// toRelativePath converts an absolute path to a path relative to rootDir,
// using forward slashes for glob compatibility.
func (tp TreePicker) toRelativePath(absPath string) string {
	rel, err := filepath.Rel(tp.rootDir, absPath)
	if err != nil {
		return absPath
	}
	// Convert to forward slashes for cross-platform glob patterns.
	return filepath.ToSlash(rel)
}

// GeneratePattern generates a glob pattern from the given absolute path.
// Directories get "/**" appended, files are returned as relative paths.
func (tp TreePicker) GeneratePattern(absPath string, isDir bool) string {
	rel := tp.toRelativePath(absPath)
	if isDir {
		return rel + "/**"
	}
	return rel
}
