// Package scanner discovers skill definitions from configurable directory paths.
// It supports Claude Code SKILL.md files and Cursor .mdc rule files,
// extracting metadata from YAML frontmatter when present.
package scanner

// Skill represents a discovered skill definition from the filesystem.
type Skill struct {
	Name        string // Skill identifier (from frontmatter or directory name)
	Description string // Human-readable description (from frontmatter, may be empty)
	Source      string // Absolute file path where this skill was discovered
	Agent       string // Which agent owns this skill: "claude", "cursor", or ""
}

// skillFrontmatter is an internal struct used to unmarshal YAML frontmatter
// from skill definition files. It maps the name and description fields
// found between --- delimiters at the top of a skill file.
type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}
