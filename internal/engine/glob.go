// glob.go handles glob pattern normalization, file path normalization,
// path matching, and project root resolution for consistent rule matching
// across platforms.
package engine

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// NormalizeGlob prepends "**/" to relative glob patterns so they match
// file paths at any depth. Absolute patterns, patterns already prefixed
// with "**/", and special values ("", "*", "**") are returned as-is.
func NormalizeGlob(pattern string) string {
	// Empty string or wildcard -- return as-is (matches everything).
	if pattern == "" || pattern == "*" || pattern == "**" {
		return pattern
	}

	// Absolute path pattern -- return as-is.
	if strings.HasPrefix(pattern, "/") {
		return pattern
	}

	// Already has depth-agnostic prefix -- return as-is.
	if strings.HasPrefix(pattern, "**/") {
		return pattern
	}

	// If the pattern contains a slash, it's a specific path (e.g., "console/**",
	// "services/bff/**"). Don't add **/ — the user picked this path intentionally.
	if strings.Contains(pattern, "/") {
		return pattern
	}

	// Simple filename pattern (e.g., "*.go") -- prepend **/ so it matches at any depth.
	return "**/" + pattern
}

// NormalizeFilePath normalizes a file path from agent input into a
// repo-relative, forward-slash path suitable for glob matching.
// Steps: convert backslashes to forward slashes, clean .. segments,
// strip project root prefix from absolute paths, trim leading slash.
func NormalizeFilePath(filePath string, projectRoot string) string {
	if filePath == "" {
		return ""
	}

	// Convert backslashes to forward slashes (Windows compatibility).
	filePath = strings.ReplaceAll(filePath, "\\", "/")
	projectRoot = strings.ReplaceAll(projectRoot, "\\", "/")

	// Clean to resolve .. segments.
	filePath = filepath.Clean(filePath)
	projectRoot = filepath.Clean(projectRoot)

	// Ensure forward slashes after Clean (Clean may reintroduce OS separators).
	filePath = filepath.ToSlash(filePath)
	projectRoot = filepath.ToSlash(projectRoot)

	// Strip trailing slash from project root for consistent prefix matching.
	projectRoot = strings.TrimSuffix(projectRoot, "/")

	// If the path is absolute and starts with projectRoot, strip the prefix.
	if projectRoot != "" && strings.HasPrefix(filePath, projectRoot+"/") {
		filePath = filePath[len(projectRoot)+1:]
	}

	// Trim any leading slash from the result.
	filePath = strings.TrimPrefix(filePath, "/")

	return filePath
}

// MatchPath checks if a normalized file path matches a normalized glob pattern.
// Uses bmatcuk/doublestar/v4 for matching, which supports ** patterns.
func MatchPath(pattern, filePath string) (bool, error) {
	return doublestar.Match(pattern, filePath)
}

// ResolveProjectRoot finds the project root directory by walking up from startDir.
// Uses .git/ directory as the project root marker. All care-bear data lives
// under ~/.care-bear/, not in project directories.
//
// Resolution order:
//  1. Nearest directory containing .git/
//  2. startDir itself (fallback)
//
// The walk stops at the filesystem root (when parent == current).
func ResolveProjectRoot(startDir string) string {
	current := startDir
	for {
		// Check for .git/ directory — the project root marker.
		gitDir := filepath.Join(current, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return current
		}

		// Move to parent directory.
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root.
			break
		}
		current = parent
	}

	// No .git/ found — fall back to startDir.
	return startDir
}
