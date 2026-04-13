// root_internal_test.go contains unit tests for unexported functions in root.go.
// These tests live in package cli to access internal functions directly.
package cli

import (
	"log/slog"
	"os"
	"os/exec"
	"testing"

	"github.com/Blue-Bear-Security/care-bear/internal/adapter"
	"github.com/Blue-Bear-Security/care-bear/internal/engine"
)

// TestResolveCheckoutPath_NilProject verifies that resolveCheckoutPath returns
// the selectedPath unchanged when project is nil.
func TestResolveCheckoutPath_NilProject(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	result, err := resolveCheckoutPath("/some/path", nil, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "/some/path" {
		t.Errorf("expected /some/path, got %s", result)
	}
}

// TestResolveCheckoutPath_SinglePath verifies that resolveCheckoutPath returns
// the selectedPath when the project has only one local path.
func TestResolveCheckoutPath_SinglePath(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	project := &adapter.MergedProject{
		Name:       "test-repo",
		Path:       "/path/one",
		LocalPaths: []string{"/path/one"},
		Agents:     []string{"claude"},
	}

	result, err := resolveCheckoutPath("/path/one", project, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "/path/one" {
		t.Errorf("expected /path/one, got %s", result)
	}
}

// TestResolveCheckoutPath_MultiPathNoPreference verifies that resolveCheckoutPath
// falls back to the selectedPath when there are multiple local paths but the
// selected path is not a git repo (so no repo identity can be resolved for preference lookup).
func TestResolveCheckoutPath_MultiPathNoGitRepo(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	dir := t.TempDir()
	dir2 := t.TempDir()

	project := &adapter.MergedProject{
		Name:       "test-repo",
		Path:       dir,
		LocalPaths: []string{dir, dir2},
		Agents:     []string{"claude"},
	}

	// Neither path is a git repo, so ResolveRepoIdentity returns nil.
	// resolveCheckoutPath should return selectedPath.
	result, err := resolveCheckoutPath(dir, project, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != dir {
		t.Errorf("expected %s, got %s", dir, result)
	}
}

// TestResolveCheckoutPath_MultiPathNoGitRepoReturnsSelected verifies that
// when a project has multiple paths but the selected path is not a git repo,
// resolveCheckoutPath returns the selectedPath.
func TestResolveCheckoutPath_MultiPathNoGitRepoReturnsSelected(t *testing.T) {
	t.Parallel()

	checkout1 := t.TempDir()
	checkout2 := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	project := &adapter.MergedProject{
		Name:       "test-repo",
		Path:       checkout1,
		LocalPaths: []string{checkout1, checkout2},
		Agents:     []string{"claude"},
	}

	// Without git repo identity, resolveCheckoutPath returns selectedPath.
	result, err := resolveCheckoutPath(checkout1, project, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != checkout1 {
		t.Errorf("expected %s (no git repo, returns selected), got %s", checkout1, result)
	}
}

// TestResolveCheckoutPath_MultiPathWithGitRepoAndPreference verifies that
// when a preferred path is saved in preferences and the selected path is
// a git repo, the preferred path is returned if it matches.
func TestResolveCheckoutPath_MultiPathWithGitRepoAndPreference(t *testing.T) {
	// Cannot run in parallel: creates git repos and modifies HOME.
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Create two checkout directories with git repos.
	checkout1 := t.TempDir()
	checkout2 := t.TempDir()

	// Initialize git repos with a fake remote.
	initGitRepo(t, checkout1, "https://github.com/test-org/test-repo.git")
	initGitRepo(t, checkout2, "https://github.com/test-org/test-repo.git")

	// Resolve repo identity to get the slug.
	repo := engine.ResolveRepoIdentity(checkout1)
	if repo == nil {
		t.Skip("git not available or could not resolve repo identity")
	}

	// Save a preference for checkout2.
	repoConfigDir := engine.RepoConfigDir(dir, repo)
	prefs := &engine.RepoPreferences{
		PreferredPath: checkout2,
	}
	if err := engine.SaveRepoPreferences(repoConfigDir, prefs); err != nil {
		t.Fatalf("failed to save preferences: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	project := &adapter.MergedProject{
		Name:       "test-repo",
		Path:       checkout1,
		LocalPaths: []string{checkout1, checkout2},
		Agents:     []string{"claude"},
	}

	result, err := resolveCheckoutPath(checkout1, project, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != checkout2 {
		t.Errorf("expected preferred path %s, got %s", checkout2, result)
	}
}

// TestResolveCheckoutPath_PreferredPathNotInDiscoveredPaths verifies that
// when a preferred path exists but does NOT match any discovered path,
// it falls through to the interactive picker (which we can't test, so
// it will return selectedPath via the error path when form.Run() fails
// in non-interactive mode).
func TestResolveCheckoutPath_PreferredPathNotInDiscoveredPaths(t *testing.T) {
	// Cannot run in parallel: creates git repos and modifies HOME.
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	checkout1 := t.TempDir()
	checkout2 := t.TempDir()

	initGitRepo(t, checkout1, "https://github.com/test-org/stale-pref-repo.git")

	repo := engine.ResolveRepoIdentity(checkout1)
	if repo == nil {
		t.Skip("git not available or could not resolve repo identity")
	}

	// Save a preference for a path that's NOT in the discovered list.
	repoConfigDir := engine.RepoConfigDir(dir, repo)
	prefs := &engine.RepoPreferences{
		PreferredPath: "/nonexistent/path",
	}
	if err := engine.SaveRepoPreferences(repoConfigDir, prefs); err != nil {
		t.Fatalf("failed to save preferences: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	project := &adapter.MergedProject{
		Name:       "stale-pref-repo",
		Path:       checkout1,
		LocalPaths: []string{checkout1, checkout2},
		Agents:     []string{"claude"},
	}

	// The preferred path doesn't match, so it falls through to form.Run().
	// In a non-interactive test, form.Run() returns an error, and the function
	// returns selectedPath.
	result, err := resolveCheckoutPath(checkout1, project, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return selectedPath since preference doesn't match.
	if result != checkout1 {
		t.Errorf("expected %s (stale preference), got %s", checkout1, result)
	}
}

// TestResolveCheckoutPath_EmptyPreference verifies that when preferences exist
// but PreferredPath is empty, it falls through to the interactive picker.
func TestResolveCheckoutPath_EmptyPreference(t *testing.T) {
	// Cannot run in parallel.
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	checkout1 := t.TempDir()
	checkout2 := t.TempDir()

	initGitRepo(t, checkout1, "https://github.com/test-org/empty-pref-repo.git")

	repo := engine.ResolveRepoIdentity(checkout1)
	if repo == nil {
		t.Skip("git not available or could not resolve repo identity")
	}

	// Save empty preference.
	repoConfigDir := engine.RepoConfigDir(dir, repo)
	prefs := &engine.RepoPreferences{
		PreferredPath: "",
	}
	if err := engine.SaveRepoPreferences(repoConfigDir, prefs); err != nil {
		t.Fatalf("failed to save preferences: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	project := &adapter.MergedProject{
		Name:       "empty-pref-repo",
		Path:       checkout1,
		LocalPaths: []string{checkout1, checkout2},
		Agents:     []string{"claude"},
	}

	result, err := resolveCheckoutPath(checkout1, project, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty preference falls to form.Run() which fails non-interactively.
	if result != checkout1 {
		t.Errorf("expected %s (empty preference), got %s", checkout1, result)
	}
}

// initGitRepo initializes a git repo in dir with the given remote URL.
func initGitRepo(t *testing.T, dir, remoteURL string) {
	t.Helper()

	cmds := [][]string{
		{"git", "-C", dir, "init"},
		{"git", "-C", dir, "remote", "add", "origin", remoteURL},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to run %v: %v\n%s", args, err, out)
		}
	}
}
