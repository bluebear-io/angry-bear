// Package engine implements the core skill enforcement logic for care-bear.
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

// GlobalConfig represents the global care-bear configuration.
type GlobalConfig struct {
	SkillPaths      []string `json:"skill_paths"`
	StateTTLHours   int      `json:"state_ttl_hours"`
	SkillTTLMinutes int      `json:"skill_ttl_minutes"`
	DefaultAgent    string   `json:"default_agent"`
	IgnorePatterns  []string `json:"ignore_patterns"`
}

// RuleSource indicates where a rule was loaded from.
const (
	// SourceRepo means the rule is from the project's .care-bear/ directory (committed to git).
	SourceRepo = "repo"
	// SourceMachine means the rule is from ~/.care-bear/repos/{hash}/ (local to this machine).
	SourceMachine = "machine"
)

// MatchedRule is a Rule paired with its origin.
type MatchedRule struct {
	Rule   Rule
	Source string // SourceRepo or SourceMachine
}

// BlockResult represents the outcome of an enforcement check.
type BlockResult struct {
	Blocked bool
	Reason  string
	Missing []string
}
