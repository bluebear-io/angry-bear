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

	// Relative pattern -- prepend **/ so it matches at any directory depth.
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
// Resolution order:
//  1. Nearest directory containing .care-bare/
//  2. Nearest directory containing .git/
//  3. startDir itself (fallback)
//
// The walk stops at the filesystem root (when parent == current).
func ResolveProjectRoot(startDir string) string {
	// Record the first directory containing .git/ as a fallback.
	gitRoot := ""

	current := startDir
	for {
		// Check for .care-bare/ directory -- highest priority, return immediately.
		careBareDir := filepath.Join(current, ".care-bare")
		if info, err := os.Stat(careBareDir); err == nil && info.IsDir() {
			return current
		}

		// Check for .git/ directory -- record as fallback.
		gitDir := filepath.Join(current, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			if gitRoot == "" {
				gitRoot = current
			}
		}

		// Move to parent directory.
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root.
			break
		}
		current = parent
	}

	// Fall back to .git/ parent if found.
	if gitRoot != "" {
		return gitRoot
	}

	// Final fallback: return the original startDir.
	return startDir
}
