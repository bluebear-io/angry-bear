// Package engine implements the core skill enforcement logic for angry-bear.
// It handles rule matching, config loading, and the ShouldBlock decision.
package engine

// Rule represents a single enforcement rule from the config file.
type Rule struct {
	Tool  string `json:"tool"`
	Path  string `json:"path"`
	Skill string `json:"skill"`
	Agent string `json:"agent"`
}

// Config represents the enforcement configuration file.
type Config struct {
	Version int    `json:"version"`
	Tools   []Rule `json:"tools"`
}

// GlobalConfig represents the global angry-bear configuration.
type GlobalConfig struct {
	SkillPaths      []string `json:"skill_paths"`
	StateTTLHours   int      `json:"state_ttl_hours"`
	SkillTTLMinutes int      `json:"skill_ttl_minutes"`
	DefaultAgent    string   `json:"default_agent"`
	IgnorePatterns  []string `json:"ignore_patterns"`
}

// RuleSource indicates where a rule was loaded from.
const (
	// SourceRepo means the rule is from the project's .angry-bear/ directory (committed to git).
	SourceRepo = "repo"
	// SourceMachine means the rule is from ~/.angry-bear/repos/{hash}/ (local to this machine).
	SourceMachine = "machine"
)

// MatchedRule is a Rule paired with its origin.
type MatchedRule struct {
	Rule   Rule
	Source string // SourceRepo or SourceMachine
}

// DeduplicateRules removes exact duplicate rules from a config, preserving order.
func DeduplicateRules(cfg *Config) {
	seen := make(map[Rule]bool)
	var unique []Rule
	for _, r := range cfg.Tools {
		if !seen[r] {
			seen[r] = true
			unique = append(unique, r)
		}
	}
	cfg.Tools = unique
}

// BlockResult represents the outcome of an enforcement check.
type BlockResult struct {
	Blocked bool
	Reason  string
	Missing []string
}
