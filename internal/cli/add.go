// add.go implements the care-bare add command for creating enforcement rules.
// It generates the cartesian product of tools x paths x agents as separate
// rules and appends them to the config file, deduplicating against existing rules.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blue-Bear-Security/care-bare/internal/adapter"
	"github.com/Blue-Bear-Security/care-bare/internal/engine"
	"github.com/Blue-Bear-Security/care-bare/internal/scanner"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

// validToolNames lists the recognized tool names for flag validation and completion.
var validToolNames = []string{"Edit", "Write", "Bash", "Read", "Glob", "Grep", "Agent", "*"}

// validAgentNames lists the recognized agent names for flag validation and completion.
var validAgentNames = []string{"claude", "cursor", "*"}

// NewAddCommand returns the add subcommand for creating enforcement rules.
// It accepts a skill name as a positional argument, generates the cartesian
// product of --tool, --path, and --agent values as separate rules, and saves
// them to the config file.
func NewAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [skill]",
		Short: "Add enforcement rules for a skill",
		Long: `Add enforcement rules for a skill.

With no arguments, launches an interactive picker to select skill, tools,
paths, and agents. With arguments, creates rules from the cartesian product
of --tool, --path, and --agent values.

Examples:
  care-bare add                    # Interactive mode
  care-bare add go-standards --tool Edit,Write --path "**/*.go" --agent claude
  care-bare add linear --tool Edit --path "**/*.py,**/*.ts"
  care-bare add sst-architect --path "bluebear-backend/stacks/**"`,
		Args:              cobra.MaximumNArgs(1),
		RunE:              runAdd,
		ValidArgsFunction: completeSkillNames,
	}

	cmd.Flags().String("tool", "*", "Comma-separated tool names: Edit, Write, Bash, Read, Glob, Grep, Agent, *")
	cmd.Flags().String("path", "**", "Comma-separated glob patterns")
	cmd.Flags().String("agent", "*", "Comma-separated agent names: claude, cursor, *")

	// Register completions for flag values.
	_ = cmd.RegisterFlagCompletionFunc("tool", completeToolNames)
	_ = cmd.RegisterFlagCompletionFunc("agent", completeAgentNames)

	return cmd
}

// runAdd is the main handler for the add command. With no args, launches
// interactive mode. With a skill name arg, uses flags for one-liner mode.
func runAdd(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	// Auto-install hooks on first use
	ensureHooksInstalled()

	var skillName string
	var tools, paths, agents []string

	if len(args) == 0 {
		// Interactive mode
		var err error
		skillName, tools, paths, agents, err = runAddInteractive(cmd)
		if err != nil {
			return err
		}
	} else {
		skillName = args[0]
		toolFlag, _ := cmd.Flags().GetString("tool")
		pathFlag, _ := cmd.Flags().GetString("path")
		agentFlag, _ := cmd.Flags().GetString("agent")
		tools = splitCSV(toolFlag)
		paths = splitCSV(pathFlag)
		agents = splitCSV(agentFlag)
	}

	// Generate the cartesian product of tools x paths x agents.
	var newRules []engine.Rule
	for _, tool := range tools {
		for _, path := range paths {
			normalizedPath := engine.NormalizeGlob(path)
			for _, agent := range agents {
				newRules = append(newRules, engine.Rule{
					Tool:  tool,
					Path:  normalizedPath,
					Skill: skillName,
					Agent: agent,
				})
			}
		}
	}

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

	// Deduplicate: only add rules that do not already exist.
	existingSet := buildRuleSet(cfg.Tools)
	added := 0
	for _, r := range newRules {
		key := ruleKey(r)
		if existingSet[key] {
			continue
		}
		cfg.Tools = append(cfg.Tools, r)
		existingSet[key] = true
		added++
	}

	if added == 0 {
		fmt.Fprintf(out, "No new rules added for skill %q (all already exist)\n", skillName)
		return nil
	}

	// Save the updated config.
	err = saveConfig(configPath, cfg)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Added %d rules for skill %q\n", added, skillName)
	return nil
}

// splitCSV splits a comma-separated string into trimmed, non-empty parts.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// resolveConfigPath determines the config file path from the --config flag
// or by resolving the repo identity and project root.
func resolveConfigPath(cmd *cobra.Command) (string, error) {
	configPath, _ := cmd.Flags().GetString("config")
	if configPath != "" {
		return configPath, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	projectRoot := engine.ResolveProjectRoot(cwd)

	// Try repo-keyed config dir first.
	repo := engine.ResolveRepoIdentity(projectRoot)
	if repo != nil {
		home, err := os.UserHomeDir()
		if err == nil {
			repoConfigDir := engine.RepoConfigDir(home, repo)
			return filepath.Join(repoConfigDir, "skill_enforcement.json"), nil
		}
	}

	// Fall back to project-level config.
	return filepath.Join(projectRoot, ".care-bare", "skill_enforcement.json"), nil
}

// loadOrCreateConfig reads an existing config file or returns a new empty config.
func loadOrCreateConfig(path string) (*engine.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &engine.Config{Version: 1}, nil
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg engine.Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("malformed JSON in %s: %w", path, err)
	}

	return &cfg, nil
}

// saveConfig writes the config to disk, creating parent directories as needed.
func saveConfig(path string, cfg *engine.Config) error {
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	err = os.WriteFile(path, data, 0o644)
	if err != nil {
		return fmt.Errorf("writing config %s: %w", path, err)
	}

	return nil
}

// ruleKey returns a string key for deduplication of rules.
func ruleKey(r engine.Rule) string {
	return r.Tool + "|" + r.Path + "|" + r.Skill + "|" + r.Agent
}

// buildRuleSet creates a set of rule keys for fast lookup.
func buildRuleSet(rules []engine.Rule) map[string]bool {
	set := make(map[string]bool, len(rules))
	for _, r := range rules {
		set[ruleKey(r)] = true
	}
	return set
}

// completeSkillNames provides shell completion for skill name arguments
// by scanning skill directories for discovered skill names.
func completeSkillNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		// Only one skill name argument expected.
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	projectRoot := engine.ResolveProjectRoot(cwd)

	// Load global config to get skill paths.
	globalCfg, err := engine.LoadGlobalConfig(projectRoot)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Resolve and scan skill paths.
	var skillPaths []string
	for _, sp := range globalCfg.SkillPaths {
		if filepath.IsAbs(sp) {
			skillPaths = append(skillPaths, sp)
		} else {
			skillPaths = append(skillPaths, filepath.Join(projectRoot, sp))
		}
	}

	skills, err := scanner.ScanSkills(skillPaths)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, s := range skills {
		if strings.HasPrefix(s.Name, toComplete) {
			names = append(names, s.Name)
		}
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeToolNames provides shell completion for the --tool flag.
func completeToolNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var matches []string
	for _, name := range validToolNames {
		if strings.HasPrefix(name, toComplete) {
			matches = append(matches, name)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}

// completeAgentNames provides shell completion for the --agent flag.
func completeAgentNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var matches []string
	for _, name := range validAgentNames {
		if strings.HasPrefix(name, toComplete) {
			matches = append(matches, name)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}

// runAddInteractive launches an interactive form to select skill, tools, paths, agents.
func runAddInteractive(cmd *cobra.Command) (skill string, tools, paths, agents []string, err error) {
	// Discover available skills
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("getting working directory: %w", err)
	}
	projectRoot := engine.ResolveProjectRoot(cwd)
	globalCfg, _ := engine.LoadGlobalConfig(projectRoot)

	var skillPaths []string
	for _, sp := range globalCfg.SkillPaths {
		if filepath.IsAbs(sp) {
			skillPaths = append(skillPaths, sp)
		} else {
			skillPaths = append(skillPaths, filepath.Join(projectRoot, sp))
		}
	}
	discoveredSkills, _ := scanner.ScanSkills(skillPaths)

	// Build skill options
	skillOpts := make([]huh.Option[string], 0, len(discoveredSkills)+1)
	for _, s := range discoveredSkills {
		label := s.Name
		if s.Description != "" {
			label = s.Name + "  — " + s.Description
			if len(label) > 70 {
				label = label[:67] + "..."
			}
		}
		skillOpts = append(skillOpts, huh.NewOption(label, s.Name))
	}
	if len(skillOpts) == 0 {
		return "", nil, nil, nil, fmt.Errorf("no skills found in %v — create skills first", globalCfg.SkillPaths)
	}

	// Build tool options
	toolOpts := make([]huh.Option[string], len(validToolNames))
	for i, t := range validToolNames {
		toolOpts[i] = huh.NewOption(t, t)
	}

	// Build agent options
	agentOpts := make([]huh.Option[string], len(validAgentNames))
	for i, a := range validAgentNames {
		agentOpts[i] = huh.NewOption(a, a)
	}

	var selectedSkill string
	var selectedTools []string
	var selectedAgents []string
	var pathInput string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a skill to enforce").
				Options(skillOpts...).
				Value(&selectedSkill),
		),
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select tools to enforce").
				Description("Which tools require this skill?").
				Options(toolOpts...).
				Value(&selectedTools),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("File path patterns").
				Description("Comma-separated globs (e.g., **/*.go, stacks/**)").
				Placeholder("**").
				Value(&pathInput),
		),
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select agents").
				Description("Which agents should enforce this?").
				Options(agentOpts...).
				Value(&selectedAgents),
		),
	).WithTheme(huh.ThemeDracula())

	err = form.Run()
	if err != nil {
		return "", nil, nil, nil, err
	}

	// Defaults
	if len(selectedTools) == 0 {
		selectedTools = []string{"*"}
	}
	if pathInput == "" {
		pathInput = "**"
	}
	if len(selectedAgents) == 0 {
		selectedAgents = []string{"*"}
	}

	return selectedSkill, selectedTools, splitCSV(pathInput), selectedAgents, nil
}

// ensureHooksInstalled checks if care-bare hooks are installed in agent configs.
// If not, installs them silently. Called on first `care-bare add`.
func ensureHooksInstalled() {
	registry := adapter.NewRegistry()
	for _, name := range registry.Names() {
		a, err := registry.Get(name)
		if err != nil {
			continue
		}
		// Check if agent is present on this machine
		home, err := os.UserHomeDir()
		if err != nil {
			continue
		}
		configDir := filepath.Join(home, "."+name)
		if _, err := os.Stat(configDir); err != nil {
			continue
		}
		// Install hook (idempotent)
		_ = a.InstallHook("")
	}
}
