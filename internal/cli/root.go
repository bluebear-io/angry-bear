// root.go defines the root Cobra command and registers all subcommands.
// When invoked with no subcommand, it launches the interactive TUI.
package cli

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/Blue-Bear-Security/care-bear/internal/adapter"
	"github.com/Blue-Bear-Security/care-bear/internal/engine"
	"github.com/Blue-Bear-Security/care-bear/internal/scanner"
	"github.com/Blue-Bear-Security/care-bear/internal/state"
	"github.com/Blue-Bear-Security/care-bear/internal/tui"
)

// NewRootCommand builds and returns the root command with all subcommands.
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "care-bear",
		Short: "Enforce skill-loading requirements for AI coding agents",
		RunE:  tuiRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Auto-install hooks on every command except hook/completion
			// (hook would cause infinite recursion, completion is non-interactive)
			name := cmd.Name()
			if name == "hook" || name == "completion" || name == "version" || name == "enable" || name == "disable" || name == "doctor" {
				return nil
			}
			result := EnsureHooksInstalled()
			PrintHookSetup(result)
			return nil
		},
	}

	rootCmd.PersistentFlags().String("config", "", "Override config file path")
	rootCmd.PersistentFlags().Bool("verbose", false, "Enable debug logging to stderr")

	rootCmd.AddCommand(NewHookCommand())
	rootCmd.AddCommand(NewStatusCommand())
	rootCmd.AddCommand(NewCleanCommand())
	rootCmd.AddCommand(NewDoctorCommand())
	rootCmd.AddCommand(NewVersionCommand())
	rootCmd.AddCommand(NewAddCommand())
	rootCmd.AddCommand(NewRulesCommand())
	rootCmd.AddCommand(NewRmCommand())
	rootCmd.AddCommand(NewEnableCommand())
	rootCmd.AddCommand(NewDisableCommand())

	return rootCmd
}

// tuiRunE launches the interactive TUI when care-bear is run with no subcommand.
// First shows a project picker, then loads the selected project's config and skills.
func tuiRunE(cmd *cobra.Command, args []string) error {
	for {
		switchRequested, err := tuiRunOnce(cmd, args)
		if err != nil {
			return err
		}
		if !switchRequested {
			return nil
		}
	}
}

func tuiRunOnce(cmd *cobra.Command, args []string) (bool, error) {
	logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelWarn}))

	// 1. Discover all projects on the machine via adapter registry.
	registry := adapter.NewRegistry()
	projects, err := registry.ScanAllProjects()
	if err != nil {
		return false, fmt.Errorf("scanning projects: %w", err)
	}

	// Show logo
	printLogo()

	var projectRoot string
	var selectedProject *adapter.MergedProject

	if len(projects) == 0 {
		// No projects found -- fall back to cwd
		cwd, err := os.Getwd()
		if err != nil {
			return false, fmt.Errorf("getting working directory: %w", err)
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

		err = form.Run()
		if err != nil {
			return false, err
		}

		// Find the matching MergedProject for available paths.
		for i := range projects {
			if projects[i].Path == selectedPath {
				selectedProject = &projects[i]
				break
			}
		}

		// For multi-checkout repos without a preferred path, let user pick.
		projectRoot, err = resolveCheckoutPath(selectedPath, selectedProject, logger)
		if err != nil {
			return false, err
		}
	}

	// 2. Resolve repo identity and load config from repo-keyed dir.
	repo := engine.ResolveRepoIdentity(projectRoot)
	var repoConfigDir string
	if repo != nil {
		home, err := os.UserHomeDir()
		if err != nil {
			logger.Warn("failed to get home directory for repo config", "error", err)
		} else {
			repoConfigDir = engine.RepoConfigDir(home, repo)
			err = os.MkdirAll(repoConfigDir, 0o755)
			if err != nil {
				logger.Warn("failed to create repo config directory", "path", repoConfigDir, "error", err)
			}
		}
	}

	// Load rules: repo config dir first, then project-level fallback
	var rules []engine.MatchedRule
	if repoConfigDir != "" {
		rules, err = engine.LoadConfigFromDir(repoConfigDir)
		if err != nil {
			logger.Warn("failed to load repo config, trying project config", "error", err)
			rules = nil
		}
	}
	if len(rules) == 0 {
		rules, err = engine.LoadConfig(projectRoot)
	}
	if err != nil {
		return false, fmt.Errorf("loading enforcement config: %w", err)
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
			configPath = filepath.Join(projectRoot, ".care-bear", "skill_enforcement.json")
		}
	}

	// 3. Load global config for skill paths.
	globalCfg, err := engine.LoadGlobalConfig(projectRoot)
	if err != nil {
		return false, fmt.Errorf("loading global config: %w", err)
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
		return false, fmt.Errorf("scanning skills: %w", err)
	}

	// 6. Collect loaded skills from all active sessions.
	loadedSkills := state.CollectLoadedSkills(filepath.Join(projectRoot, ".care-bear", "state"))

	// 7. Build available local paths list for the TUI settings view.
	var availablePaths []string
	if selectedProject != nil {
		availablePaths = selectedProject.LocalPaths
	}

	// 8. Create TUI model and load event log.
	model := tui.NewApp(cfg, configPath, projectRoot, skills, loadedSkills, globalCfg, repoConfigDir, availablePaths)
	model.LoadEvents(projectRoot)
	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return false, err
	}
	if app, ok := finalModel.(tui.App); ok {
		return app.SwitchRequested(), nil
	}
	return false, nil
}

// resolveCheckoutPath handles multi-checkout repos. If a preferred path is already
// set and valid, it is used. Otherwise, for repos with multiple checkouts, a
// sub-menu lets the user pick which path to use, then saves the preference.
func resolveCheckoutPath(selectedPath string, project *adapter.MergedProject, logger *slog.Logger) (string, error) {
	if project == nil || len(project.LocalPaths) <= 1 {
		return selectedPath, nil
	}

	// Check for existing preferred path.
	home, err := os.UserHomeDir()
	if err != nil {
		return selectedPath, nil
	}

	repo := engine.ResolveRepoIdentity(selectedPath)
	if repo == nil {
		return selectedPath, nil
	}

	repoConfigDir := engine.RepoConfigDir(home, repo)
	prefs, err := engine.LoadRepoPreferences(repoConfigDir)
	if err != nil {
		logger.Warn("failed to load repo preferences", "error", err)
		return selectedPath, nil
	}

	// If a preferred path exists and is among the discovered paths, use it.
	if prefs.PreferredPath != "" {
		for _, lp := range project.LocalPaths {
			if lp == prefs.PreferredPath {
				return prefs.PreferredPath, nil
			}
		}
	}

	// No valid preference -- show sub-menu for path selection.
	pathOpts := make([]huh.Option[string], len(project.LocalPaths))
	for i, lp := range project.LocalPaths {
		pathOpts[i] = huh.NewOption(lp, lp)
	}

	var chosenPath string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Multiple checkouts for %s", project.Name)).
				Description("Select which local copy to use (will be saved as default)").
				Options(pathOpts...).
				Value(&chosenPath),
		),
	).WithTheme(huh.ThemeDracula())

	err = form.Run()
	if err != nil {
		return selectedPath, nil
	}

	// Save the chosen path as preferred.
	prefs.PreferredPath = chosenPath
	err = engine.SaveRepoPreferences(repoConfigDir, prefs)
	if err != nil {
		logger.Warn("failed to save repo preferences", "error", err)
	}

	return chosenPath, nil
}

// Execute runs the root command.
func Execute() error {
	return NewRootCommand().Execute()
}
