// enable.go implements care-bear enable and care-bear disable commands.
// Enable installs hooks into all detected agent configs.
// Disable removes ALL care-bear hooks from all agent configs.
package cli

import (
	"fmt"
	"os"

	"github.com/Blue-Bear-Security/care-bear/internal/adapter"
	"github.com/spf13/cobra"
)

// NewEnableCommand returns the enable subcommand that installs hooks.
func NewEnableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "Install care-bear hooks into all AI agent configs",
		Long:  "Installs PreToolUse hooks into Claude Code and Cursor global configs.\nSafe to run multiple times — existing hooks are not duplicated.",
		RunE:  runEnable,
	}
}

// NewDisableCommand returns the disable subcommand that removes hooks.
func NewDisableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Remove ALL care-bear hooks from AI agent configs",
		Long:  "Removes all care-bear hook entries from Claude Code and Cursor.\nEnforcement stops immediately. Rules are preserved — run 'enable' to re-activate.",
		RunE:  runDisable,
	}
}

func runEnable(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	result := EnsureHooksInstalled()

	for _, name := range result.Installed {
		fmt.Fprintf(out, "  \u2713 Hooks installed for %s\n", name)
	}
	for _, name := range result.Skipped {
		fmt.Fprintf(out, "  - %s not detected on this machine\n", name)
	}
	for _, w := range result.Warnings {
		fmt.Fprintf(out, "  ! %s\n", w)
	}

	if len(result.Installed) == 0 && len(result.Warnings) == 0 {
		fmt.Fprintln(out, "  Hooks already installed for all detected agents.")
	}

	fmt.Fprintln(out, "\nEnforcement is active.")
	return nil
}

func runDisable(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	registry := adapter.NewRegistry()
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	removed := 0
	for _, name := range registry.Names() {
		a, err := registry.Get(name)
		if err != nil {
			continue
		}

		// Skip agents not present on this machine
		configDir := fmt.Sprintf("%s/.%s", home, name)
		if _, err := os.Stat(configDir); err != nil {
			continue
		}

		err = a.UninstallHook()
		if err != nil {
			fmt.Fprintf(out, "  ! Failed to remove hooks for %s: %v\n", name, err)
			continue
		}
		fmt.Fprintf(out, "  \u2713 Hooks removed for %s\n", name)
		removed++
	}

	if removed == 0 {
		fmt.Fprintln(out, "  No hooks found to remove.")
	} else {
		fmt.Fprintln(out, "\nEnforcement is disabled. Rules are preserved.")
		fmt.Fprintln(out, "Run 'care-bear enable' to re-activate.")
	}
	return nil
}
