// engine.go contains the ShouldBlock function, the core enforcement decision
// maker for care-bare. It is a pure function with no side effects that
// determines whether a tool invocation should be blocked based on enforcement
// rules and the set of skills already invoked in the session.
package engine

import (
	"fmt"
	"strings"
)

// ShouldBlock checks whether the given tool invocation should be blocked
// based on enforcement rules and invoked skills.
//
// Algorithm:
//  1. Filter matching rules by checking tool, agent, and path (AND conditions).
//  2. Deduplicate by skill name (first match per skill wins).
//  3. Check which matched skills have NOT been invoked.
//  4. Build a BlockResult with missing skill names and a human-readable reason.
//
// Returns BlockResult with Blocked=false when no enforcement is needed,
// or Blocked=true with Missing skills and a descriptive Reason.
func ShouldBlock(rules []MatchedRule, toolName, filePath, agent string, invokedSkills map[string]bool) BlockResult {
	if len(rules) == 0 {
		return BlockResult{Blocked: false}
	}

	// Track seen skill names for deduplication. First match per skill wins.
	seenSkills := make(map[string]bool)
	var matched []MatchedRule

	for _, mr := range rules {
		rule := mr.Rule

		// Tool match: empty or "*" matches all tools, otherwise exact match.
		if rule.Tool != "" && rule.Tool != "*" && rule.Tool != toolName {
			continue
		}

		// Agent match: empty or "*" matches all agents, otherwise exact match.
		if rule.Agent != "" && rule.Agent != "*" && rule.Agent != agent {
			continue
		}

		// Path match: empty or "*" matches all files. If filePath is empty
		// and rule has a non-wildcard path, skip (no file to match against).
		if rule.Path != "" && rule.Path != "*" {
			if filePath == "" {
				// Rule requires a specific path but no file path provided -- skip.
				continue
			}
			match, err := MatchPath(rule.Path, filePath)
			if err != nil {
				// Glob pattern error -- skip this rule.
				continue
			}
			if !match {
				continue
			}
		}

		// Deduplicate by skill name.
		if seenSkills[rule.Skill] {
			continue
		}
		seenSkills[rule.Skill] = true
		matched = append(matched, mr)
	}

	// Check which matched skills have NOT been invoked.
	var missing []string
	for _, mr := range matched {
		if invokedSkills != nil && invokedSkills[mr.Rule.Skill] {
			continue
		}
		missing = append(missing, mr.Rule.Skill)
	}

	if len(missing) == 0 {
		return BlockResult{Blocked: false}
	}

	// Build the human-readable reason string.
	// Build skill names with load instructions.
	quotedSkills := make([]string, len(missing))
	loadInstructions := make([]string, len(missing))
	for i, s := range missing {
		quotedSkills[i] = fmt.Sprintf("%q", s)
		loadInstructions[i] = fmt.Sprintf("/%s (or read .claude/skills/%s/SKILL.md)", s, s)
	}

	reason := fmt.Sprintf(
		"Blocked by care-bare skill enforcement. Required skills not loaded: %s. "+
			"Load them by running: %s",
		strings.Join(quotedSkills, ", "),
		strings.Join(loadInstructions, "  "),
	)

	return BlockResult{
		Blocked: true,
		Reason:  reason,
		Missing: missing,
	}
}

// MatchedSkills returns the list of skill names that have matching rules for
// the given tool/path/agent combination. Used to determine if an event is
// enforcement-relevant (has rules that apply) even when the skills are loaded.
func MatchedSkills(rules []MatchedRule, toolName, filePath, agent string) []string {
	seenSkills := make(map[string]bool)
	var matched []string

	for _, mr := range rules {
		rule := mr.Rule
		if rule.Tool != "" && rule.Tool != "*" && rule.Tool != toolName {
			continue
		}
		if rule.Agent != "" && rule.Agent != "*" && rule.Agent != agent {
			continue
		}
		if rule.Path != "" && rule.Path != "*" {
			if filePath == "" {
				continue
			}
			match, err := MatchPath(rule.Path, filePath)
			if err != nil || !match {
				continue
			}
		}
		if !seenSkills[rule.Skill] {
			seenSkills[rule.Skill] = true
			matched = append(matched, rule.Skill)
		}
	}
	return matched
}
