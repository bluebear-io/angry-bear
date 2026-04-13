// rm.go implements the care-bare rm command for removing enforcement rules.
// It removes all rules for a given skill, or a subset when --tool and --path
// filters are provided.
package cli

import (
	"fmt"
	"strings"

	"github.com/Blue-Bear-Security/care-bare/internal/engine"
	"github.com/spf13/cobra"
)

// NewRmCommand returns the rm subcommand for removing enforcement rules.
// It accepts a skill name as a positional argument and removes matching rules
// from the config file. Optional --tool and --path flags narrow the removal.
func NewRmCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <skill>",
		Short: "Remove enforcement rules for a skill",
		Long: `Remove enforcement rules for a skill.

Removes all rules matching the given skill name. Use --tool and --path flags
to narrow which rules are removed.

Examples:
  care-bare rm linear
  care-bare rm go-standards --tool Edit --path "**/*.go"`,
		Args:              cobra.ExactArgs(1),
		RunE:              runRm,
		ValidArgsFunction: completeSkillNames,
	}

	cmd.Flags().String("tool", "", "Only remove rules matching this tool (comma-separated)")
	cmd.Flags().String("path", "", "Only remove rules matching this path (comma-separated)")

	// Register completions for flag values.
	_ = cmd.RegisterFlagCompletionFunc("tool", completeToolNames)

	return cmd
}

// runRm is the main handler for the rm command. It loads config, filters out
// matching rules, saves the updated config, and reports how many were removed.
func runRm(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	skillName := args[0]

	toolFlag, _ := cmd.Flags().GetString("tool")
	pathFlag, _ := cmd.Flags().GetString("path")

	// Build filter sets from comma-separated flags.
	toolFilter := buildFilterSet(toolFlag)
	pathFilter := buildPathFilterSet(pathFlag)

	// Resolve config path.
	configPath, err := resolveConfigPath(cmd)
	if err != nil {
		return err
	}

	// Load existing config.
	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return err
	}

	originalCount := len(cfg.Tools)

	// Keep rules that do NOT match the removal criteria.
	var kept []engine.Rule
	for _, r := range cfg.Tools {
		if shouldRemoveRule(r, skillName, toolFilter, pathFilter) {
			continue
		}
		kept = append(kept, r)
	}

	removed := originalCount - len(kept)

	if removed == 0 {
		fmt.Fprintf(out, "No matching rules found for skill %q\n", skillName)
		return nil
	}

	cfg.Tools = kept

	// Save the updated config.
	err = saveConfig(configPath, cfg)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Removed %d rules for skill %q\n", removed, skillName)
	return nil
}

// shouldRemoveRule checks whether a rule matches the removal criteria.
// A rule matches if its skill matches and, when tool/path filters are provided,
// its tool and path also match.
func shouldRemoveRule(r engine.Rule, skill string, toolFilter, pathFilter map[string]bool) bool {
	if r.Skill != skill {
		return false
	}

	// If tool filter is provided, the rule's tool must be in the filter set.
	if len(toolFilter) > 0 && !toolFilter[r.Tool] {
		return false
	}

	// If path filter is provided, the rule's path must be in the filter set.
	if len(pathFilter) > 0 && !pathFilter[r.Path] {
		return false
	}

	return true
}

// buildFilterSet creates a set from comma-separated values. Returns an empty
// map when the input is empty (meaning "no filter").
func buildFilterSet(csv string) map[string]bool {
	if csv == "" {
		return nil
	}
	parts := splitCSV(csv)
	set := make(map[string]bool, len(parts))
	for _, p := range parts {
		set[p] = true
	}
	return set
}

// buildPathFilterSet creates a set from comma-separated path values,
// normalizing each path via NormalizeGlob. Returns nil when the input
// is empty (meaning "no filter").
func buildPathFilterSet(csv string) map[string]bool {
	if csv == "" {
		return nil
	}
	parts := splitCSV(csv)
	set := make(map[string]bool, len(parts))
	for _, p := range parts {
		set[engine.NormalizeGlob(strings.TrimSpace(p))] = true
	}
	return set
}
