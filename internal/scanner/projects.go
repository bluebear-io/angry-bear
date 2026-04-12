// projects.go discovers AI coding agent projects on the machine by scanning
// ~/.claude/projects/ and ~/.cursor/projects/ directories.
package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Project represents a discovered project with AI agent sessions.
type Project struct {
	Name     string   // Human-readable name (last path component)
	Path     string   // Decoded absolute path to the project
	Agents   []string // Which agents use this project ("claude", "cursor")
	Encoded  string   // Encoded directory name
}

// ScanProjects discovers all projects that have Claude or Cursor sessions.
// Returns deduplicated projects sorted by name, with agents merged.
func ScanProjects() ([]Project, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	projectMap := make(map[string]*Project) // keyed by decoded path

	// Scan Claude projects
	claudeDir := filepath.Join(home, ".claude", "projects")
	scanAgentProjects(claudeDir, "claude", projectMap)

	// Scan Cursor projects
	cursorDir := filepath.Join(home, ".cursor", "projects")
	scanAgentProjects(cursorDir, "cursor", projectMap)

	// Convert to sorted slice, only include projects whose paths exist on disk
	var projects []Project
	for _, p := range projectMap {
		if _, err := os.Stat(p.Path); err == nil {
			projects = append(projects, *p)
		}
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})

	return projects, nil
}

// scanAgentProjects scans an agent's projects directory and adds to the map.
func scanAgentProjects(dir string, agent string, projectMap map[string]*Project) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		encoded := e.Name()
		decoded := decodeProjectPath(encoded)
		if decoded == "" {
			continue
		}

		if existing, ok := projectMap[decoded]; ok {
			// Add agent if not already present
			hasAgent := false
			for _, a := range existing.Agents {
				if a == agent {
					hasAgent = true
					break
				}
			}
			if !hasAgent {
				existing.Agents = append(existing.Agents, agent)
			}
		} else {
			projectMap[decoded] = &Project{
				Name:    filepath.Base(decoded),
				Path:    decoded,
				Agents:  []string{agent},
				Encoded: encoded,
			}
		}
	}
}

// decodeProjectPath converts an encoded project directory name back to an
// absolute path. The encoding replaces / with - and prepends -.
// This is tricky because actual path components may contain dashes.
// We try the most literal interpretation first: replace leading - with /
// and all remaining - with /. If that path exists, use it. Otherwise
// try variations.
func decodeProjectPath(encoded string) string {
	if encoded == "" {
		return ""
	}

	// Simple decode: leading - becomes /, all - become /
	decoded := "/" + strings.ReplaceAll(strings.TrimPrefix(encoded, "-"), "-", "/")

	// Check if it exists
	if _, err := os.Stat(decoded); err == nil {
		return decoded
	}

	// Try without leading / (for Cursor which doesn't prefix with -)
	if !strings.HasPrefix(encoded, "-") {
		decoded = "/" + strings.ReplaceAll(encoded, "-", "/")
		if _, err := os.Stat(decoded); err == nil {
			return decoded
		}
	}

	// Try common path patterns by checking parent directories
	// Walk the decoded path and try to find the longest existing prefix
	parts := strings.Split(strings.TrimPrefix(encoded, "-"), "-")
	for i := len(parts); i >= 2; i-- {
		candidate := "/" + strings.Join(parts[:i], "/")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Give up — return the simple decode even if it doesn't exist
	return "/" + strings.ReplaceAll(strings.TrimPrefix(encoded, "-"), "-", "/")
}
