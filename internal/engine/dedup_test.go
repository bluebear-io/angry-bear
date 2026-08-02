// dedup_test.go verifies rule deduplication, including that the Command field
// participates in equality so command rules are not collapsed with file rules.
package engine

import "testing"

func TestDeduplicateRules(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Version: 1,
		Tools: []Rule{
			{Tool: "Bash", Path: "**", Command: "git", Skill: "git", Agent: "*"},
			{Tool: "Bash", Path: "**", Command: "git", Skill: "git", Agent: "*"}, // exact dup
			{Tool: "Bash", Path: "**", Command: "aws", Skill: "aws", Agent: "*"}, // different command
			{Tool: "Bash", Path: "**", Command: "", Skill: "git", Agent: "*"},    // no command — distinct
		},
	}

	DeduplicateRules(cfg)

	if len(cfg.Tools) != 3 {
		t.Fatalf("got %d rules after dedup, want 3: %+v", len(cfg.Tools), cfg.Tools)
	}
	// The two distinct commands and the empty-command rule must all survive.
	commands := map[string]bool{}
	for _, r := range cfg.Tools {
		commands[r.Command] = true
	}
	for _, want := range []string{"git", "aws", ""} {
		if !commands[want] {
			t.Errorf("expected a rule with command %q to survive dedup; got %v", want, commands)
		}
	}
}
