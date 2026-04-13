// rules.go implements the care-bare rules command for listing enforcement rules.
// It displays rules in a table format by default, with optional JSON output
// for scripting and a --skill filter for showing rules for a specific skill.
package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Blue-Bear-Security/care-bare/internal/engine"
	"github.com/spf13/cobra"
)

// NewRulesCommand returns the rules subcommand for listing enforcement rules.
// It loads config from the repo config dir or project-level directory and
// displays rules in table or JSON format.
func NewRulesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "List enforcement rules",
		Long: `List all configured enforcement rules.

Shows rules in a table format by default. Use --json for machine-readable output.
Use --skill to filter rules for a specific skill name.

Examples:
  care-bare rules
  care-bare rules --skill go-standards
  care-bare rules --json`,
		RunE: runRules,
	}

	cmd.Flags().String("skill", "", "Filter rules by skill name")
	cmd.Flags().Bool("json", false, "Output rules as JSON for scripting")

	return cmd
}

// runRules is the main handler for the rules command. It loads config,
// optionally filters by skill, and prints rules in the requested format.
func runRules(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	skillFilter, _ := cmd.Flags().GetString("skill")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	// Resolve config path.
	configPath, err := resolveConfigPath(cmd)
	if err != nil {
		return err
	}

	// Load config.
	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return err
	}

	// Filter by skill if requested.
	rules := cfg.Tools
	if skillFilter != "" {
		var filtered []engine.Rule
		for _, r := range rules {
			if r.Skill == skillFilter {
				filtered = append(filtered, r)
			}
		}
		rules = filtered
	}

	// Output in JSON format if requested.
	if jsonOutput {
		return printRulesJSON(out, rules, configPath)
	}

	// Default: table format.
	return printRulesTable(out, rules, configPath, skillFilter)
}

// printRulesJSON outputs rules as a JSON array to the writer.
func printRulesJSON(out io.Writer, rules []engine.Rule, configPath string) error {
	output := struct {
		Source string        `json:"source"`
		Rules  []engine.Rule `json:"rules"`
	}{
		Source: configPath,
		Rules:  rules,
	}

	// Ensure empty rules array is [] not null.
	if output.Rules == nil {
		output.Rules = []engine.Rule{}
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON output: %w", err)
	}

	fmt.Fprintln(out, string(data))
	return nil
}

// printRulesTable outputs rules in a human-readable table format.
func printRulesTable(out io.Writer, rules []engine.Rule, configPath, skillFilter string) error {
	if len(rules) == 0 {
		if skillFilter != "" {
			fmt.Fprintf(out, "No rules found for skill %q\n", skillFilter)
		} else {
			fmt.Fprintln(out, "No enforcement rules configured.")
		}
		fmt.Fprintf(out, "Config: %s\n", configPath)
		return nil
	}

	fmt.Fprintln(out, "Enforcement Rules")
	fmt.Fprintln(out, "=================")

	for i, r := range rules {
		fmt.Fprintf(out, "  [%d] Skill: %s | Tool: %s | Path: %s | Agent: %s\n",
			i+1, r.Skill, r.Tool, r.Path, r.Agent)
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "%d rules from %s\n", len(rules), configPath)
	return nil
}
