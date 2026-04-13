// hook_internal_test.go contains unit tests for unexported functions in hook.go
// and other internal CLI functions. These tests live in package cli to access
// internal functions directly.
package cli

import (
	"testing"
)

// TestDetectSkillFromPath verifies that detectSkillFromPath correctly extracts
// skill names from SKILL.md file paths and returns empty for non-skill paths.
func TestDetectSkillFromPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "relative path with .claude/skills prefix",
			path:     ".claude/skills/run-migration/SKILL.md",
			expected: "run-migration",
		},
		{
			name:     "absolute path with .claude/skills",
			path:     "/abs/path/.claude/skills/git/SKILL.md",
			expected: "git",
		},
		{
			name:     "skill with hyphenated name",
			path:     "/home/user/project/.claude/skills/go-coding-standards/SKILL.md",
			expected: "go-coding-standards",
		},
		{
			name:     "regular Go file returns empty",
			path:     "some/other/file.go",
			expected: "",
		},
		{
			name:     "markdown file that is not SKILL.md",
			path:     ".claude/skills/run-migration/README.md",
			expected: "",
		},
		{
			name:     "SKILL.md at root returns empty (dir component is dot)",
			path:     "SKILL.md",
			expected: "",
		},
		{
			name:     "case insensitive skill.md",
			path:     "/project/.claude/skills/testing/skill.md",
			expected: "testing",
		},
		{
			name:     "case insensitive SKILL.MD uppercase",
			path:     "/project/.claude/skills/testing/SKILL.MD",
			expected: "testing",
		},
		{
			name:     "Windows-style backslash path",
			path:     "C:\\Users\\dev\\.claude\\skills\\my-skill\\SKILL.md",
			expected: "my-skill",
		},
		{
			name:     "empty path returns empty",
			path:     "",
			expected: "",
		},
		{
			name:     "path with only SKILL.md after slash",
			path:     "/SKILL.md",
			expected: "",
		},
		{
			name:     "deeply nested skill path",
			path:     "/a/b/c/d/.claude/skills/deep-skill/SKILL.md",
			expected: "deep-skill",
		},
		{
			name:     "skill outside .claude/skills still works",
			path:     "/custom/path/my-custom-skill/SKILL.md",
			expected: "my-custom-skill",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := detectSkillFromPath(tc.path)
			if result != tc.expected {
				t.Errorf("detectSkillFromPath(%q) = %q, want %q", tc.path, result, tc.expected)
			}
		})
	}
}

// TestTruncateSessionID verifies that truncateSessionID returns the first
// 5 characters of a session ID, or the full ID if shorter.
func TestTruncateSessionID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "long session ID truncated to 5 chars",
			input:    "abcdefghij-1234567890",
			expected: "abcde",
		},
		{
			name:     "exactly 5 characters",
			input:    "abcde",
			expected: "abcde",
		},
		{
			name:     "shorter than 5 characters",
			input:    "abc",
			expected: "abc",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single character",
			input:    "x",
			expected: "x",
		},
		{
			name:     "UUID-style session ID",
			input:    "550e8400-e29b-41d4-a716-446655440000",
			expected: "550e8",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := truncateSessionID(tc.input)
			if result != tc.expected {
				t.Errorf("truncateSessionID(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// TestExitError_ErrorMessage verifies that ExitError.Error() returns a
// human-readable description of the exit code.
func TestExitError_ErrorMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     int
		expected string
	}{
		{
			name:     "exit code 2",
			code:     2,
			expected: "exit code 2",
		},
		{
			name:     "exit code 1",
			code:     1,
			expected: "exit code 1",
		},
		{
			name:     "exit code 0",
			code:     0,
			expected: "exit code 0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := &ExitError{Code: tc.code}
			if err.Error() != tc.expected {
				t.Errorf("ExitError{Code: %d}.Error() = %q, want %q", tc.code, err.Error(), tc.expected)
			}
		})
	}
}
