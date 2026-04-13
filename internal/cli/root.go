// root.go defines the root Cobra command and registers all subcommands.
// When invoked with no subcommand, it launches the interactive TUI.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/Blue-Bear-Security/care-bare/internal/adapter"
	"github.com/Blue-Bear-Security/care-bare/internal/engine"
	"github.com/Blue-Bear-Security/care-bare/internal/scanner"
	"github.com/Blue-Bear-Security/care-bare/internal/state"
	"github.com/Blue-Bear-Security/care-bare/internal/tui"
)

// NewRootCommand builds and returns the root command with all subcommands.
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "care-bare",
		Short: "Enforce skill-loading requirements for AI coding agents",
		RunE:  tuiRunE,
	}

	rootCmd.PersistentFlags().String("config", "", "Override config file path")
	rootCmd.PersistentFlags().Bool("verbose", false, "Enable debug logging to stderr")

	rootCmd.AddCommand(NewHookCommand())
	rootCmd.AddCommand(NewInitCommand())
	rootCmd.AddCommand(NewStatusCommand())
	rootCmd.AddCommand(NewCleanCommand())
	rootCmd.AddCommand(NewDoctorCommand())
	rootCmd.AddCommand(NewVersionCommand())

	return rootCmd
}

// tuiRunE launches the interactive TUI when care-bare is run with no subcommand.
// First shows a project picker, then loads the selected project's config and skills.
func tuiRunE(cmd *cobra.Command, args []string) error {
	for {
		tui.SwitchRequested = false
		if err := tuiRunOnce(cmd, args); err != nil {
			return err
		}
		if !tui.SwitchRequested {
			return nil
		}
	}
}

func tuiRunOnce(cmd *cobra.Command, args []string) error {
	// 1. Discover all projects on the machine via adapter registry.
	registry := adapter.NewRegistry()
	projects, err := registry.ScanAllProjects()
	if err != nil {
		return fmt.Errorf("scanning projects: %w", err)
	}

	var projectRoot string

	if len(projects) == 0 {
		// No projects found — fall back to cwd
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		projectRoot = engine.ResolveProjectRoot(cwd)
	} else {
		// Build options for the project picker
		opts := make([]huh.Option[string], len(projects))
		for i, p := range projects {
			agents := strings.Join(p.Agents, ", ")
			copies := ""
			if len(p.LocalPaths) > 1 {
				copies = fmt.Sprintf(", %d copies", len(p.LocalPaths))
			}
			label := fmt.Sprintf("%s  (%s%s)", p.Name, agents, copies)
			opts[i] = huh.NewOption(label, p.Path)
		}

		var selectedPath string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Select a project").
					Description("Showing all projects with Claude Code or Cursor sessions").
					Options(opts...).
					Value(&selectedPath),
			),
		).WithTheme(huh.ThemeDracula())

		err := form.Run()
		if err != nil {
			return err
		}
		projectRoot = selectedPath
	}

	// 2. Resolve repo identity and load config from repo-keyed dir.
	repo := engine.ResolveRepoIdentity(projectRoot)
	var repoConfigDir string
	if repo != nil {
		home, _ := os.UserHomeDir()
		repoConfigDir = engine.RepoConfigDir(home, repo)
		os.MkdirAll(repoConfigDir, 0755)
	}

	// Load rules: repo config dir first, then project-level fallback
	var rules []engine.MatchedRule
	if repoConfigDir != "" {
		rules, _ = engine.LoadConfigFromDir(repoConfigDir)
	}
	if len(rules) == 0 {
		rules, err = engine.LoadConfig(projectRoot)
	}
	if err != nil {
		return fmt.Errorf("loading enforcement config: %w", err)
	}

	// Build a Config struct from loaded rules.
	cfg := engine.Config{Version: 1}
	for _, mr := range rules {
		cfg.Tools = append(cfg.Tools, mr.Rule)
	}

	// Determine the config file path for saving.
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		if repoConfigDir != "" {
			configPath = filepath.Join(repoConfigDir, "skill_enforcement.json")
		} else {
			configPath = filepath.Join(projectRoot, ".care-bare", "skill_enforcement.json")
		}
	}

	// 3. Load global config for skill paths.
	globalCfg, err := engine.LoadGlobalConfig(projectRoot)
	if err != nil {
		return fmt.Errorf("loading global config: %w", err)
	}

	// 4. Resolve skill paths relative to project root.
	var skillPaths []string
	for _, sp := range globalCfg.SkillPaths {
		if filepath.IsAbs(sp) {
			skillPaths = append(skillPaths, sp)
		} else {
			skillPaths = append(skillPaths, filepath.Join(projectRoot, sp))
		}
	}

	// 5. Scan skills from configured paths.
	skills, err := scanner.ScanSkills(skillPaths)
	if err != nil {
		return fmt.Errorf("scanning skills: %w", err)
	}

	// 6. Collect loaded skills from all active sessions.
	loadedSkills := collectLoadedSkills(filepath.Join(projectRoot, ".care-bare", "state"))

	// 7. Create TUI model and load event log.
	model := tui.NewApp(cfg, configPath, skills, loadedSkills)
	model.LoadEvents(projectRoot)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// collectLoadedSkills reads all session state files and returns skills
// with their agent information.
func collectLoadedSkills(stateDir string) map[string]*tui.SkillStatus {
	loaded := make(map[string]*tui.SkillStatus)

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return loaded
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(stateDir, e.Name()))
		if err != nil {
			continue
		}
		var ss state.SessionState
		if err := json.Unmarshal(data, &ss); err != nil {
			continue
		}
		agent := ss.Agent
		if agent == "" {
			agent = "unknown"
		}
		for _, skill := range ss.InvokedSkills {
			if loaded[skill] == nil {
				loaded[skill] = &tui.SkillStatus{}
			}
			found := false
			for _, a := range loaded[skill].Agents {
				if a == agent {
					found = true
					break
				}
			}
			if !found {
				loaded[skill].Agents = append(loaded[skill].Agents, agent)
			}
		}
	}

	return loaded
}

// Execute runs the root command.
func Execute() error {
	return NewRootCommand().Execute()
}
