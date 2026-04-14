// prune.go handles TTL-based cleanup of expired session state files.
// It uses file modification times (mtime) for expiry checks to avoid parsing JSON.
// A .last-prune file throttles pruning to at most once per hour.
package state

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// pruneThrottleInterval is the minimum duration between automatic prune runs.
const pruneThrottleInterval = 1 * time.Hour

// lastPruneFile is the name of the file that tracks the last prune timestamp.
const lastPruneFile = ".last-prune"

// validStateFilePattern matches files that are safe to prune: session ID chars + .json or .lock extension.
var validStateFilePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+\.(json|lock)$`)

// PruneExpired removes state files older than the given TTL based on file mtime.
// It also removes corresponding .lock files. Only files matching the expected naming
// pattern are considered for removal. Updates the .last-prune timestamp after running.
func PruneExpired(stateDir string, ttl time.Duration) error {
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return err
	}

	now := time.Now()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Skip the .last-prune file and any dotfiles.
		if strings.HasPrefix(name, ".") {
			continue
		}

		// Only process .json files for expiry checks. Lock files are removed
		// as companions to their corresponding .json files.
		if !strings.HasSuffix(name, ".json") {
			continue
		}

		// Ensure the filename matches the expected pattern.
		if !validStateFilePattern.MatchString(name) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if now.Sub(info.ModTime()) > ttl {
			// Remove the expired .json file.
			jsonPath := filepath.Join(stateDir, name)
			if err := os.Remove(jsonPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}

			// Remove the corresponding .lock file.
			baseName := strings.TrimSuffix(name, ".json")
			lockPath := filepath.Join(stateDir, baseName+".lock")
			if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}

	// Update the .last-prune timestamp.
	return writeLastPrune(stateDir)
}

// PruneIfDue runs PruneExpired only if more than 1 hour has passed since the last prune.
// This is the function called from the hook path to avoid an O(n) directory scan on
// every invocation. On first run (no .last-prune file), pruning always runs.
func PruneIfDue(stateDir string, ttl time.Duration) error {
	lastPrune, err := readLastPrune(stateDir)
	if err != nil {
		// First time -- no .last-prune file exists, run pruning.
		return PruneExpired(stateDir, ttl)
	}

	if time.Since(lastPrune) < pruneThrottleInterval {
		// Throttled -- not enough time has passed since last prune.
		return nil
	}

	return PruneExpired(stateDir, ttl)
}

// readLastPrune reads the .last-prune file and parses the RFC 3339 timestamp.
// Returns an error if the file doesn't exist or the timestamp is unparseable.
func readLastPrune(stateDir string) (time.Time, error) {
	data, err := os.ReadFile(filepath.Join(stateDir, lastPruneFile))
	if err != nil {
		return time.Time{}, err
	}

	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Time{}, err
	}

	return ts, nil
}

// writeLastPrune writes the current UTC time as an RFC 3339 timestamp to the .last-prune file.
func writeLastPrune(stateDir string) error {
	timestamp := time.Now().UTC().Format(time.RFC3339)
	return os.WriteFile(filepath.Join(stateDir, lastPruneFile), []byte(timestamp), 0o600)
}
