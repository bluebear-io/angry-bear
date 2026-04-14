// version.go implements the angry-bear version command.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version information set by main via SetVersionInfo before Execute.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// SetVersionInfo sets the version information from ldflags.
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	date = d
}

// NewVersionCommand returns the version subcommand.
func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "angry-bear version %s (commit: %s, built: %s)\n", version, commit, date)
			return nil
		},
	}
}
