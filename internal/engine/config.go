// config.go handles loading and merging enforcement configurations from
// user-level and project-level config files. It implements a two-level
// merge strategy: user-level (~/.care-bare/) rules are loaded first,
// then project-level rules are accumulated by walking up from the start
// directory to the filesystem root or user home directory.
package engine

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// configFileName is the name of the skill enforcement config file.
const configFileName = "skill_enforcement.json"

// configDirName is the name of the care-bare config directory.
const configDirName = ".care-bare"

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
//  1. User-level: ~/.care-bare/skill_enforcement.json
//  2. Project-level: walk up from startDir collecting .care-bare/skill_enforcement.json
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
			log.Printf("warning: cannot determine home directory: %v", err)
			homeDir = ""
		}
	}

	var allRules []MatchedRule

	// 1. Load user-level config from ~/.care-bare/skill_enforcement.json.
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
	// Check existence first with os.Stat to avoid unnecessary reads.
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		if os.IsPermission(err) {
			log.Printf("warning: permission denied reading %s, skipping", path)
			return nil, nil
		}
		// Other stat errors -- skip with warning.
		log.Printf("warning: cannot stat %s: %v, skipping", path, err)
		return nil, nil
	}

	// Read the file.
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsPermission(err) {
			log.Printf("warning: permission denied reading %s, skipping", path)
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
			Source: path,
		})
	}

	return rules, nil
}

// LoadGlobalConfig reads the global config.json from the project root.
// Returns defaults if the file does not exist.
func LoadGlobalConfig(projectRoot string) (*GlobalConfig, error) {
	defaults := &GlobalConfig{
		SkillPaths:     []string{".claude/skills"},
		StateTTLHours:  24,
		DefaultAgent:   "*",
		IgnorePatterns: []string{".git", "node_modules", "vendor", "dist", ".next", "__pycache__", ".venv", "build", "target"},
	}

	configPath := filepath.Join(projectRoot, configDirName, "config.json")
	_, err := os.Stat(configPath)
	if err != nil {
		// File doesn't exist or can't be read -- return defaults.
		return defaults, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return defaults, nil
	}

	var cfg GlobalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("malformed JSON in %s: %w", configPath, err)
	}

	// Fill in defaults for zero-value fields.
	if len(cfg.SkillPaths) == 0 {
		cfg.SkillPaths = defaults.SkillPaths
	}
	if cfg.StateTTLHours == 0 {
		cfg.StateTTLHours = defaults.StateTTLHours
	}
	if cfg.DefaultAgent == "" {
		cfg.DefaultAgent = defaults.DefaultAgent
	}
	if len(cfg.IgnorePatterns) == 0 {
		cfg.IgnorePatterns = defaults.IgnorePatterns
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
