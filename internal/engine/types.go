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

// MatchedRule is a Rule paired with the file path it was loaded from.
type MatchedRule struct {
	Rule   Rule
	Source string
}

// BlockResult represents the outcome of an enforcement check.
type BlockResult struct {
	Blocked bool
	Reason  string
	Missing []string
}
