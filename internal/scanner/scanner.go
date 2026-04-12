// scanner.go provides skill discovery by walking configurable directory paths.
// It discovers Claude Code SKILL.md files and Cursor .mdc rule files,
// returning a deduplicated, alphabetically sorted list of skills.
package scanner

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
)

// ScanSkills walks the given directory paths and discovers skill definitions.
// It looks for SKILL.md files (Claude Code pattern) and .mdc files (Cursor pattern).
// Returns a deduplicated, name-sorted list of discovered skills.
// Nonexistent or unreadable paths are silently skipped (logged at debug level).
//
// Parameters:
//   - paths: list of directory paths to scan for skill files
//
// Returns:
//   - []Skill: deduplicated, alphabetically sorted list of discovered skills
//   - error: only returned for unexpected fatal errors (not for missing paths)
func ScanSkills(paths []string) ([]Skill, error) {
	// Deduplication map keyed by skill name; first-discovered wins.
	seen := make(map[string]Skill)

	for _, root := range paths {
		// Check if path exists; skip nonexistent paths with a log warning.
		if _, err := os.Stat(root); os.IsNotExist(err) {
			log.Printf("warning: skill path does not exist, skipping: %s", root)
			continue
		}

		// Walk the directory tree looking for skill files.
		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// Permission error on a subdirectory: skip the subtree.
				if os.IsPermission(err) {
					log.Printf("warning: permission denied, skipping: %s", path)
					if d != nil && d.IsDir() {
						return fs.SkipDir
					}
					return nil
				}
				return err
			}

			// Skip directories -- we only care about files.
			if d.IsDir() {
				return nil
			}

			// Determine agent type based on file pattern.
			agent := detectAgent(d.Name())
			if agent == "" {
				// Not a skill file, skip.
				return nil
			}

			// Parse the skill file to extract metadata.
			skill, parseErr := ParseSkillFile(path)
			if parseErr != nil {
				log.Printf("warning: failed to parse skill file %s: %v", path, parseErr)
				return nil
			}

			skill.Agent = agent

			// Deduplicate by name: first-discovered wins.
			if _, exists := seen[skill.Name]; !exists {
				seen[skill.Name] = *skill
			}

			return nil
		})

		if walkErr != nil {
			return nil, walkErr
		}
	}

	// Collect all discovered skills into a sorted slice.
	result := make([]Skill, 0, len(seen))
	for _, skill := range seen {
		result = append(result, skill)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// detectAgent determines which agent a file belongs to based on its filename.
// Returns "claude" for SKILL.md files, "cursor" for .mdc files, or "" for
// unrecognized file types.
func detectAgent(filename string) string {
	if filename == "SKILL.md" {
		return "claude"
	}
	if filepath.Ext(filename) == ".mdc" {
		return "cursor"
	}
	return ""
}
