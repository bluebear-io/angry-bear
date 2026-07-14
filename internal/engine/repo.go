// repo.go provides Git repository identity resolution.
// All enforcement, config, and state is keyed by repo identity (org/repo),
// not by local directory path. This means multiple clones of the same repo
// share the same enforcement rules.
package engine

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Pre-compiled regex patterns for Git remote URL normalization.
// Compiled once at package init to avoid repeated compilation per call.
var (
	sshPattern      = regexp.MustCompile(`git@[^:]+:(.+)`)
	httpsPattern    = regexp.MustCompile(`https?://[^/]+/(.+)`)
	sshProtoPattern = regexp.MustCompile(`ssh://[^/]+/(.+)`)
)

// RepoIdentity represents a normalized Git repository.
type RepoIdentity struct {
	// Slug is the normalized org/repo identifier (e.g., "bluebear-io/blueden").
	Slug string
	// Hash is a short hash of the slug for filesystem-safe directory names.
	Hash string
	// RemoteURL is the raw Git remote URL.
	RemoteURL string
}

// ResolveRepoIdentity determines the Git repo identity for a given directory.
// It extracts the remote URL and normalizes it to an org/repo slug.
// Returns nil if the directory is not a Git repo or has no remote.
func ResolveRepoIdentity(dir string) *RepoIdentity {
	// Get the git remote URL
	cmd := exec.Command("git", "-C", dir, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	remoteURL := strings.TrimSpace(string(out))
	if remoteURL == "" {
		return nil
	}

	slug := NormalizeRemoteURL(remoteURL)
	if slug == "" {
		return nil
	}

	hash := ShortHash(slug)

	return &RepoIdentity{
		Slug:      slug,
		Hash:      hash,
		RemoteURL: remoteURL,
	}
}

// NormalizeRemoteURL extracts the org/repo slug from any Git remote URL format.
// Handles: https://, git@, ssh://, and URLs with tokens or custom hostnames.
//
// Examples:
//   - https://github.com/bluebear-io/blueden.git → bluebear-io/blueden
//   - git@github.com:bluebear-io/blueden.git → bluebear-io/blueden
//   - git@github.com-bluebear:bluebear-io/blueden.git → bluebear-io/blueden
//   - https://x-access-token:TOKEN@github.com/bluebear-io/blueden.git → bluebear-io/blueden
func NormalizeRemoteURL(url string) string {
	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	// SSH format: git@host:org/repo or git@host-alias:org/repo
	if m := sshPattern.FindStringSubmatch(url); len(m) == 2 {
		return m[1]
	}

	// HTTPS format: https://[user:pass@]host/org/repo
	// Extract the last two path components as org/repo
	if m := httpsPattern.FindStringSubmatch(url); len(m) == 2 {
		return m[1]
	}

	// SSH:// format: ssh://git@host/org/repo
	if m := sshProtoPattern.FindStringSubmatch(url); len(m) == 2 {
		return m[1]
	}

	return ""
}

// ShortHash returns the first 8 chars of the SHA-256 hash of a string.
// Used for filesystem-safe directory names.
func ShortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:4])
}

// RepoStateDir returns the path to a repo's session state directory.
// State is stored at ~/.angry-bear/repos/{hash}-{slug}/state/
// Returns empty string if repo identity cannot be resolved.
func RepoStateDir(homeDir string, repo *RepoIdentity) string {
	return filepath.Join(RepoConfigDir(homeDir, repo), "state")
}

// ResolveStateDir resolves the state directory for a project.
// Uses ~/.angry-bear/repos/{hash}/state/ when the project has a git remote,
// falls back to {projectRoot}/.angry-bear/state/ for repos without a remote.
func ResolveStateDir(projectRoot string) string {
	repo := ResolveRepoIdentity(projectRoot)
	if repo != nil {
		home, err := os.UserHomeDir()
		if err == nil {
			return RepoStateDir(home, repo)
		}
	}
	return filepath.Join(projectRoot, ".angry-bear", "state")
}

// RepoConfigDir returns the path to a repo's angry-bear config directory.
// Config is stored at ~/.angry-bear/repos/{hash}-{slug}/
func RepoConfigDir(homeDir string, repo *RepoIdentity) string {
	// Use hash-slug for both uniqueness and readability
	safeName := strings.ReplaceAll(repo.Slug, "/", "-")
	dirName := fmt.Sprintf("%s-%s", repo.Hash, safeName)
	return filepath.Join(homeDir, ".angry-bear", "repos", dirName)
}
