// clean.go implements the care-bare clean command for state file cleanup.
// It supports three modes: pruning expired sessions (default), cleaning all
// sessions (--all), or cleaning a specific session (--session).
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blue-Bear-Security/care-bare/internal/engine"
	"github.com/Blue-Bear-Security/care-bare/internal/state"
	"github.com/spf13/cobra"
)

// NewCleanCommand returns the clean subcommand.
// It prunes expired state files, with flags for cleaning all or a specific session.
func NewCleanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean up session state files",
		Long: `Clean up session state files in .care-bare/state/.

With no flags, prunes sessions that have expired based on the configured TTL.
Use --all to remove all sessions regardless of TTL.
Use --session to remove a specific session by ID.`,
		RunE: runClean,
	}
	cmd.Flags().Bool("all", false, "Remove all state files regardless of TTL")
	cmd.Flags().String("session", "", "Remove a specific session's state")
	return cmd
}

// runClean is the main handler for the clean command. It resolves the project
// root, validates flags, and dispatches to the appropriate cleaning mode.
func runClean(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	cleanAll, _ := cmd.Flags().GetBool("all")
	sessionID, _ := cmd.Flags().GetString("session")

	// Validate mutually exclusive flags.
	if cleanAll && sessionID != "" {
		return fmt.Errorf("flags --all and --session are mutually exclusive")
	}

	// Resolve project root from cwd.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	projectRoot := engine.ResolveProjectRoot(cwd)

	// Check if state directory exists.
	stateDir := filepath.Join(projectRoot, ".care-bare", "state")
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		fmt.Fprintln(out, "No state directory found. Run 'care-bare init' first.")
		return nil
	}

	if sessionID != "" {
		return cleanSession(out, stateDir, sessionID)
	}

	if cleanAll {
		return cleanAllSessions(out, stateDir)
	}

	return cleanExpired(out, projectRoot, stateDir)
}

// cleanSession validates and removes a specific session's state files.
func cleanSession(out io.Writer, stateDir, sessionID string) error {
	err := state.ValidateSessionID(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}

	mgr := state.NewStateManager(stateDir)
	err = mgr.Clean(sessionID)
	if err != nil {
		return fmt.Errorf("cleaning session %s: %w", sessionID, err)
	}

	fmt.Fprintf(out, "Cleaned session: %s\n", sessionID)
	return nil
}

// cleanAllSessions lists all session JSON files and removes each one.
func cleanAllSessions(out io.Writer, stateDir string) error {
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return fmt.Errorf("reading state directory: %w", err)
	}

	mgr := state.NewStateManager(stateDir)
	count := 0

	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		sessionID := strings.TrimSuffix(entry.Name(), ".json")
		err := mgr.Clean(sessionID)
		if err != nil {
			return fmt.Errorf("cleaning session %s: %w", sessionID, err)
		}
		count++
	}

	fmt.Fprintf(out, "Cleaned %d sessions\n", count)
	return nil
}

// cleanExpired loads the TTL from config and runs PruneExpired directly,
// ignoring the throttle mechanism used in the hot path.
func cleanExpired(out io.Writer, projectRoot, stateDir string) error {
	globalCfg, err := engine.LoadGlobalConfig(projectRoot)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ttl := time.Duration(globalCfg.StateTTLHours) * time.Hour
	err = state.PruneExpired(stateDir, ttl)
	if err != nil {
		return fmt.Errorf("pruning expired sessions: %w", err)
	}

	fmt.Fprintf(out, "Pruned expired sessions (TTL: %s)\n", ttl)
	return nil
}
