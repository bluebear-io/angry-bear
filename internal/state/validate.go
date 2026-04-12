// validate.go provides session ID sanitization for care-bare state management.
// It ensures session IDs are safe for use as filenames and rejects path traversal attacks.
package state

import (
	"fmt"
	"regexp"
)

// sessionIDPattern matches valid session IDs: alphanumeric with dots, hyphens, underscores.
// This inherently rejects path separators and traversal sequences.
var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// maxSessionIDLength is the maximum allowed length for a session ID.
const maxSessionIDLength = 128

// ValidateSessionID checks that a session ID is safe to use as a filename.
// Returns an error if the ID is empty, exceeds 128 characters, contains
// characters outside the allowed set [A-Za-z0-9._-], or equals "." or ".."
// (which have special filesystem meaning).
func ValidateSessionID(id string) error {
	if id == "" {
		return fmt.Errorf("session ID must not be empty")
	}
	if len(id) > maxSessionIDLength {
		return fmt.Errorf("session ID exceeds maximum length of %d characters", maxSessionIDLength)
	}
	if !sessionIDPattern.MatchString(id) {
		return fmt.Errorf("session ID contains invalid characters: must match [A-Za-z0-9._-]")
	}
	if id == "." || id == ".." {
		return fmt.Errorf("session ID must not be %q (reserved filesystem name)", id)
	}
	return nil
}
