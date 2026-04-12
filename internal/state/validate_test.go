// validate_test.go tests session ID validation rules for care-bare state management.
// It ensures session IDs are safe for use as filenames and reject path traversal attacks.
package state

import (
	"strings"
	"testing"
)

// TestValidateSessionID_ValidIDs verifies that well-formed session IDs are accepted.
func TestValidateSessionID_ValidIDs(t *testing.T) {
	t.Parallel()

	validIDs := []struct {
		name string
		id   string
	}{
		{"alphanumeric lowercase", "abc123"},
		{"alphanumeric uppercase", "ABC123"},
		{"mixed case", "AbCdEf"},
		{"with hyphens", "session-001"},
		{"with underscores", "my_session"},
		{"with dots", "my.session"},
		{"mixed separators", "my.session_id-001"},
		{"single character", "a"},
		{"max length 128 chars", strings.Repeat("a", 128)},
	}

	for _, tc := range validIDs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateSessionID(tc.id); err != nil {
				t.Errorf("ValidateSessionID(%q) returned error: %v, want nil", tc.id, err)
			}
		})
	}
}

// TestValidateSessionID_InvalidIDs verifies that dangerous or malformed session IDs are rejected.
func TestValidateSessionID_InvalidIDs(t *testing.T) {
	t.Parallel()

	invalidIDs := []struct {
		name string
		id   string
	}{
		{"empty string", ""},
		{"path traversal", "../x"},
		{"forward slash", "foo/bar"},
		{"backslash", "foo\\bar"},
		{"exceeds 128 chars", strings.Repeat("a", 129)},
		{"contains space", "foo bar"},
		{"contains null byte", "foo\x00bar"},
		{"contains tilde", "foo~bar"},
		{"contains at sign", "foo@bar"},
		{"contains colon", "foo:bar"},
		{"only dots", ".."},
	}

	for _, tc := range invalidIDs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateSessionID(tc.id); err == nil {
				t.Errorf("ValidateSessionID(%q) returned nil, want error", tc.id)
			}
		})
	}
}
