// config.go handles loading and merging enforcement configurations from
// user-level and project-level config files. It implements a two-level
// merge strategy: user-level (~/.care-bear/) rules are loaded first,
// then project-level rules are accumulated by walking up from the start
// directory to the filesystem root or user home directory.
package engine

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// configFileName is the name of the skill enforcement config file.
const configFileName = "skill_enforcement.json"

// configDirName is the name of the care-bear config directory.
const configDirName = ".care-bear"

// ConfigOption configures the behavior of LoadConfig.
type ConfigOption func(*configOptions)

// configOptions holds internal options for LoadConfig.
type configOptions struct {
	homeDir string
}

// WithHomeDir overrides the user home directory for testing.
// This avoids relying on os.UserHomeDir() in tests.
func WithHomeDir(dir string) ConfigOption {
	return func(o *configOptions) {
		o.homeDir = dir
	}
}

// LoadConfig loads and merges skill enforcement rules from user-level and
// project-level config files. Returns all accumulated rules.
//
// The loading order is:
//  1. User-level: ~/.care-bear/skill_enforcement.json
//  2. Project-level: walk up from startDir collecting .care-bear/skill_enforcement.json
//     files at each directory level, stopping at filesystem root or user home.
//
// Behavior:
//   - Missing config files are silently skipped (fail-open).
//   - Permission-denied on file read is logged as warning and skipped.
//   - Malformed JSON is a hard error (returned to caller).
//   - Unsupported config version is a hard error.
//   - Each rule's Path field is normalized via NormalizeGlob.
func LoadConfig(startDir string, opts ...ConfigOption) ([]MatchedRule, error) {
	options := &configOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Resolve home directory.
	homeDir := options.homeDir
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			// Cannot determine home directory -- skip user-level config.
			slog.Warn("cannot determine home directory, skipping user-level config", "error", err)
			homeDir = ""
		}
	}

	var allRules []MatchedRule

	// 1. Load user-level config from ~/.care-bear/skill_enforcement.json.
	if homeDir != "" {
		userConfigPath := filepath.Join(homeDir, configDirName, configFileName)
		rules, err := loadConfigFile(userConfigPath)
		if err != nil {
			return nil, err
		}
		allRules = append(allRules, rules...)
	}

	// 2. Walk up from startDir collecting project-level configs.
	current, err := filepath.Abs(startDir)
	if err != nil {
		current = startDir
	}

	// Normalize home dir for comparison.
	absHome := ""
	if homeDir != "" {
		absHome, _ = filepath.Abs(homeDir)
	}

	for {
		configPath := filepath.Join(current, configDirName, configFileName)
		rules, err := loadConfigFile(configPath)
		if err != nil {
			return nil, err
		}
		allRules = append(allRules, rules...)

		// Stop if we've reached the home directory.
		if absHome != "" && current == absHome {
			break
		}

		// Move to parent directory.
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root.
			break
		}
		current = parent
	}

	return allRules, nil
}

// loadConfigFile reads and parses a single skill_enforcement.json file.
// Returns nil, nil if the file does not exist or is unreadable (permission denied).
// Returns an error if the file exists but contains malformed JSON or
// an unsupported config version.
func loadConfigFile(path string) ([]MatchedRule, error) {
	return loadConfigFileWithSource(path, SourceMachine)
}

// loadConfigFileWithSource reads and parses a single skill_enforcement.json file,
// tagging each rule with the given source (SourceRepo or SourceMachine).
func loadConfigFileWithSource(path, source string) ([]MatchedRule, error) {
	// Check existence first with os.Stat to avoid unnecessary reads.
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		if os.IsPermission(err) {
			slog.Warn("permission denied reading config, skipping", "path", path)
			return nil, nil
		}
		// Other stat errors -- skip with warning.
		slog.Warn("cannot stat config file, skipping", "path", path, "error", err)
		return nil, nil
	}

	// Read the file.
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsPermission(err) {
			slog.Warn("permission denied reading config file, skipping", "path", path)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	// Parse JSON.
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("malformed JSON in %s: %w", path, err)
	}

	// Validate config version.
	if cfg.Version != 1 {
		return nil, fmt.Errorf("unsupported config version %d in %s, expected 1", cfg.Version, path)
	}

	// Convert rules to MatchedRules with source tracking and path normalization.
	rules := make([]MatchedRule, 0, len(cfg.Tools))
	for _, rule := range cfg.Tools {
		rule.Path = NormalizeGlob(rule.Path)
		rules = append(rules, MatchedRule{
			Rule:   rule,
			Source: source,
		})
	}

	return rules, nil
}

// RepoPreferences holds per-repo user preferences such as the preferred
// local checkout path. Stored at ~/.care-bear/repos/{hash}-{slug}/preferences.json.
type RepoPreferences struct {
	PreferredPath string `json:"preferred_path"`
}

// LoadRepoPreferences reads preferences.json from a repo config directory.
// Returns an empty RepoPreferences (not nil) if the file does not exist.
func LoadRepoPreferences(repoConfigDir string) (*RepoPreferences, error) {
	prefsPath := filepath.Join(repoConfigDir, "preferences.json")
	_, err := os.Stat(prefsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &RepoPreferences{}, nil
		}
		return &RepoPreferences{}, nil
	}

	data, err := os.ReadFile(prefsPath)
	if err != nil {
		return nil, fmt.Errorf("reading preferences %s: %w", prefsPath, err)
	}

	var prefs RepoPreferences
	err = json.Unmarshal(data, &prefs)
	if err != nil {
		return nil, fmt.Errorf("malformed JSON in %s: %w", prefsPath, err)
	}

	return &prefs, nil
}

// SaveRepoPreferences writes preferences.json to a repo config directory.
// Creates the directory if it does not exist.
func SaveRepoPreferences(repoConfigDir string, prefs *RepoPreferences) error {
	err := os.MkdirAll(repoConfigDir, 0o755)
	if err != nil {
		return fmt.Errorf("creating repo config dir %s: %w", repoConfigDir, err)
	}

	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling preferences: %w", err)
	}

	prefsPath := filepath.Join(repoConfigDir, "preferences.json")
	err = os.WriteFile(prefsPath, data, 0o644)
	if err != nil {
		return fmt.Errorf("writing preferences %s: %w", prefsPath, err)
	}

	return nil
}

// globalConfigDefaults returns a GlobalConfig with sensible defaults.
func globalConfigDefaults() *GlobalConfig {
	return &GlobalConfig{
		SkillPaths:     []string{".claude/skills"},
		StateTTLHours:  24,
		DefaultAgent:   "*",
		IgnorePatterns: []string{".git", "node_modules", "vendor", "dist", ".next", "__pycache__", ".venv", "build", "target"},
	}
}

// LoadGlobalDefaults reads ~/.care-bear/config.json (machine-level defaults).
// Returns built-in defaults if the file does not exist.
func LoadGlobalDefaults() (*GlobalConfig, error) {
	defaults := globalConfigDefaults()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("cannot determine home directory for global defaults", "error", err)
		return defaults, nil
	}

	configPath := filepath.Join(homeDir, configDirName, "config.json")
	return loadGlobalConfigFile(configPath, defaults)
}

// LoadGlobalConfig reads project-level config.json from the project root and
// merges it on top of the global defaults from ~/.care-bear/config.json.
// Project-level non-zero fields override global values.
// Returns defaults if neither file exists.
func LoadGlobalConfig(projectRoot string) (*GlobalConfig, error) {
	// Load global defaults first.
	base, err := LoadGlobalDefaults()
	if err != nil {
		return nil, fmt.Errorf("loading global defaults: %w", err)
	}

	// Load project-level config on top.
	configPath := filepath.Join(projectRoot, configDirName, "config.json")
	project, err := loadGlobalConfigFile(configPath, nil)
	if err != nil {
		return nil, fmt.Errorf("loading project config %s: %w", configPath, err)
	}

	// If no project file was found, return global defaults.
	if project == nil {
		return base, nil
	}

	// Merge: project non-zero fields override global.
	merged := *base
	if len(project.SkillPaths) > 0 {
		merged.SkillPaths = project.SkillPaths
	}
	if project.StateTTLHours != 0 {
		merged.StateTTLHours = project.StateTTLHours
	}
	// SkillTTLMinutes: 0 means "no expiry" so we only override when project
	// explicitly sets a positive value.
	if project.SkillTTLMinutes > 0 {
		merged.SkillTTLMinutes = project.SkillTTLMinutes
	}
	if project.DefaultAgent != "" {
		merged.DefaultAgent = project.DefaultAgent
	}
	if len(project.IgnorePatterns) > 0 {
		merged.IgnorePatterns = project.IgnorePatterns
	}

	return &merged, nil
}

// loadGlobalConfigFile reads a single config.json file. Returns nil, nil when
// the file does not exist (and defaults is nil). When defaults is non-nil, missing
// files return defaults. Malformed JSON is always an error.
func loadGlobalConfigFile(path string, defaults *GlobalConfig) (*GlobalConfig, error) {
	_, err := os.Stat(path)
	if err != nil {
		// File doesn't exist or can't be read.
		return defaults, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return defaults, nil
	}

	var cfg GlobalConfig
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("malformed JSON in %s: %w", path, err)
	}

	return &cfg, nil
}

// LoadConfigFromDir loads enforcement rules from a specific directory.
// The directory should contain skill_enforcement.json directly.
// Returns nil rules if the file doesn't exist.
func LoadConfigFromDir(dir string) ([]MatchedRule, error) {
	configPath := filepath.Join(dir, configFileName)
	return loadConfigFile(configPath)
}

// LoadMergedConfig loads enforcement rules from both the project repo directory
// (committed to git, tagged as SourceRepo) and the machine-level config directory
// (local to this machine, tagged as SourceMachine). Rules from both sources are
// merged into a single slice. Repo rules come first.
func LoadMergedConfig(projectRoot, repoConfigDir string) ([]MatchedRule, error) {
	var allRules []MatchedRule

	// Load repo-level rules from {project}/.care-bear/skill_enforcement.json
	repoPath := filepath.Join(projectRoot, configDirName, configFileName)
	repoRules, err := loadConfigFileWithSource(repoPath, SourceRepo)
	if err != nil {
		return nil, fmt.Errorf("loading repo config %s: %w", repoPath, err)
	}
	allRules = append(allRules, repoRules...)

	// Also check .bluebear/ for backward compatibility
	bluebearPath := filepath.Join(projectRoot, ".bluebear", configFileName)
	bluebearRules, err := loadConfigFileWithSource(bluebearPath, SourceRepo)
	if err != nil {
		slog.Warn("failed to load .bluebear config", "path", bluebearPath, "error", err)
	} else {
		allRules = append(allRules, bluebearRules...)
	}

	// Load machine-level rules, skipping any that already exist from repo sources.
	if repoConfigDir != "" {
		machineConfigPath := filepath.Join(repoConfigDir, configFileName)
		machineRules, mErr := loadConfigFileWithSource(machineConfigPath, SourceMachine)
		if mErr != nil {
			slog.Warn("failed to load machine config", "path", machineConfigPath, "error", mErr)
		} else {
			seen := make(map[Rule]bool, len(allRules))
			for _, r := range allRules {
				seen[r.Rule] = true
			}
			for _, mr := range machineRules {
				if !seen[mr.Rule] {
					allRules = append(allRules, mr)
				}
			}
		}
	}

	return allRules, nil
}

// LoadGlobalConfigFromDir reads config.json directly from the given directory.
// Used for repo-keyed config dirs where config.json is at the root level
// (not inside a .care-bear/ subdirectory).
func LoadGlobalConfigFromDir(dir string) (*GlobalConfig, error) {
	base, err := LoadGlobalDefaults()
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return base, nil
	}
	configPath := filepath.Join(dir, "config.json")
	project, err := loadGlobalConfigFile(configPath, nil)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return base, nil
	}
	// Merge
	if project.SkillTTLMinutes > 0 {
		base.SkillTTLMinutes = project.SkillTTLMinutes
	}
	if project.StateTTLHours > 0 {
		base.StateTTLHours = project.StateTTLHours
	}
	if project.DefaultAgent != "" {
		base.DefaultAgent = project.DefaultAgent
	}
	if len(project.SkillPaths) > 0 {
		base.SkillPaths = project.SkillPaths
	}
	if len(project.IgnorePatterns) > 0 {
		base.IgnorePatterns = project.IgnorePatterns
	}
	return base, nil
}
