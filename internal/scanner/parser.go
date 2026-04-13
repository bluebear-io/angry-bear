// parser.go extracts skill metadata from YAML frontmatter in skill files.
// It supports both SKILL.md (Claude Code) and .mdc (Cursor) file formats.
// When frontmatter is missing or unparseable, it falls back to deriving the
// skill name from the directory name (for SKILL.md) or file stem (for .mdc).
package scanner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// utf8BOM is the byte order mark that some editors prepend to UTF-8 files.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// frontmatterDelimiter is the YAML frontmatter boundary marker.
const frontmatterDelimiter = "---"

// ParseSkillFile reads a skill definition file and extracts metadata from
// YAML frontmatter (delimited by --- lines). If no frontmatter is found,
// falls back to using the parent directory name (for SKILL.md) or
// file stem (for .mdc) as the skill name.
//
// Parameters:
//   - path: absolute or relative file path to the skill file
//
// Returns:
//   - *Skill: parsed skill metadata with Source set to the file path
//   - error: only returned if the file cannot be read
func ParseSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Strip UTF-8 BOM if present
	data = bytes.TrimPrefix(data, utf8BOM)

	skill := &Skill{
		Source: path,
	}

	// Try to extract frontmatter
	fm, ok := extractFrontmatter(data)
	if ok {
		var parsed skillFrontmatter
		if yamlErr := yaml.Unmarshal([]byte(fm), &parsed); yamlErr != nil {
			// Malformed YAML: fall back to directory/file name.
		} else {
			skill.Name = strings.TrimSpace(parsed.Name)
			skill.Description = strings.TrimSpace(parsed.Description)
		}
	}

	// Fall back to directory name or file stem if name is still empty
	if skill.Name == "" {
		skill.Name = fallbackName(path)
	}

	return skill, nil
}

// extractFrontmatter checks if the content starts with a --- delimiter and
// has a matching closing --- delimiter. If found, it returns the YAML content
// between the two delimiters and true. Otherwise, it returns empty string and false.
func extractFrontmatter(data []byte) (string, bool) {
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", false
	}

	// Must start with ---
	if !strings.HasPrefix(content, frontmatterDelimiter) {
		return "", false
	}

	// Find the end of the first line (the opening ---)
	afterOpening := strings.Index(content, "\n")
	if afterOpening == -1 {
		// File is just "---" with no newline -- no closing delimiter
		return "", false
	}

	// Find the closing --- delimiter
	rest := content[afterOpening+1:]
	closingIdx := strings.Index(rest, frontmatterDelimiter)
	if closingIdx == -1 {
		// No closing delimiter found
		return "", false
	}

	// Verify the closing delimiter is at the start of a line
	yamlBlock := rest[:closingIdx]

	return yamlBlock, true
}

// fallbackName derives a skill name from the file path when frontmatter
// is missing or lacks a name field.
// For SKILL.md files: uses the parent directory name (e.g., "go-standards" from
// ".claude/skills/go-standards/SKILL.md").
// For .mdc files: uses the file stem without extension (e.g., "security" from
// ".cursor/rules/security.mdc").
func fallbackName(path string) string {
	base := filepath.Base(path)

	if base == "SKILL.md" {
		// Use parent directory name
		return filepath.Base(filepath.Dir(path))
	}

	// For .mdc and other files, use file stem (name without extension)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}
