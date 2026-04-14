// event_parser_test.go tests the structured parser for pipe-delimited event log lines.
package tui

import "testing"

func TestParseEventLine_8ColFormat(t *testing.T) {
	line := "2026-04-13T00:00:00Z | blueden | claude | abc12 | Edit | src/main.go | BLOCK | git"
	ev, ok := parseEventLine(line, 5)

	if !ok {
		t.Fatal("expected parse to succeed for 8-col format")
	}
	if ev.Project != "blueden" {
		t.Errorf("Project = %q, want %q", ev.Project, "blueden")
	}
	if ev.Agent != "claude" {
		t.Errorf("Agent = %q, want %q", ev.Agent, "claude")
	}
	if ev.Session != "abc12" {
		t.Errorf("Session = %q, want %q", ev.Session, "abc12")
	}
	if ev.Tool != "Edit" {
		t.Errorf("Tool = %q, want %q", ev.Tool, "Edit")
	}
	if ev.Path != "src/main.go" {
		t.Errorf("Path = %q, want %q", ev.Path, "src/main.go")
	}
	if ev.Skill != "git" {
		t.Errorf("Skill = %q, want %q", ev.Skill, "git")
	}
	if ev.Action != "BLOCK" {
		t.Errorf("Action = %q, want %q", ev.Action, "BLOCK")
	}
	if !ev.IsBlock {
		t.Error("IsBlock should be true for BLOCK line")
	}
	if ev.IsLoad {
		t.Error("IsLoad should be false for BLOCK line")
	}
	if ev.LineIdx != 5 {
		t.Errorf("LineIdx = %d, want 5", ev.LineIdx)
	}
}

func TestParseEventLine_7ColFormat(t *testing.T) {
	line := "2026-04-13T00:00:00Z | claude | abc12 | Write | test.ts | ALLOW | linear"
	ev, ok := parseEventLine(line, 0)

	if !ok {
		t.Fatal("expected parse to succeed for 7-col format")
	}
	if ev.Agent != "claude" {
		t.Errorf("Agent = %q, want %q", ev.Agent, "claude")
	}
	if ev.Session != "abc12" {
		t.Errorf("Session = %q, want %q", ev.Session, "abc12")
	}
	if ev.Tool != "Write" {
		t.Errorf("Tool = %q, want %q", ev.Tool, "Write")
	}
	if ev.Path != "test.ts" {
		t.Errorf("Path = %q, want %q", ev.Path, "test.ts")
	}
	if ev.Skill != "linear" {
		t.Errorf("Skill = %q, want %q", ev.Skill, "linear")
	}
	if ev.Action != "ALLOW" {
		t.Errorf("Action = %q, want %q", ev.Action, "ALLOW")
	}
	if ev.IsBlock {
		t.Error("IsBlock should be false for ALLOW line")
	}
}

func TestParseEventLine_6ColFormat(t *testing.T) {
	line := "2026-04-13T00:00:00Z | cursor | Bash | /tmp/test | ALLOW | sst"
	ev, ok := parseEventLine(line, 2)

	if !ok {
		t.Fatal("expected parse to succeed for 6-col format")
	}
	if ev.Agent != "cursor" {
		t.Errorf("Agent = %q, want %q", ev.Agent, "cursor")
	}
	if ev.Tool != "Bash" {
		t.Errorf("Tool = %q, want %q", ev.Tool, "Bash")
	}
	if ev.Path != "/tmp/test" {
		t.Errorf("Path = %q, want %q", ev.Path, "/tmp/test")
	}
	if ev.Skill != "sst" {
		t.Errorf("Skill = %q, want %q", ev.Skill, "sst")
	}
}

func TestParseEventLine_SkillLoad(t *testing.T) {
	line := "2026-04-13T00:00:00Z | blueden | claude | abc12 | SKILL-LOAD | | LOAD | linear"
	ev, ok := parseEventLine(line, 0)

	if !ok {
		t.Fatal("expected parse to succeed for SKILL-LOAD line")
	}
	if ev.Action != "LOAD" {
		t.Errorf("Action = %q, want %q", ev.Action, "LOAD")
	}
	if !ev.IsLoad {
		t.Error("IsLoad should be true for SKILL-LOAD line")
	}
	if ev.IsBlock {
		t.Error("IsBlock should be false for LOAD line")
	}
	// LOAD events replace tool with em-dash and clear path
	if ev.Tool != "\u2014" {
		t.Errorf("Tool = %q, want em-dash for LOAD events", ev.Tool)
	}
	if ev.Path != "" {
		t.Errorf("Path = %q, want empty for LOAD events", ev.Path)
	}
}

func TestParseEventLine_SkillTTLExpire(t *testing.T) {
	line := "2026-04-14T09:28:17Z | Blue-Bear-Security/blueden | claude | real- | SKILL-TTL | | EXPIR | linear"
	ev, ok := parseEventLine(line, 3)

	if !ok {
		t.Fatal("expected parse to succeed for SKILL-TTL line")
	}
	if ev.Action != "EXPIR" {
		t.Errorf("Action = %q, want %q", ev.Action, "EXPIR")
	}
	if !ev.IsExpire {
		t.Error("IsExpire should be true for SKILL-TTL line")
	}
	if ev.IsBlock {
		t.Error("IsBlock should be false for EXPIR line")
	}
	if ev.IsLoad {
		t.Error("IsLoad should be false for EXPIR line")
	}
	if ev.Tool != "SKILL-TTL" {
		t.Errorf("Tool = %q, want %q for EXPIR events", ev.Tool, "SKILL-TTL")
	}
	if ev.Path != "" {
		t.Errorf("Path = %q, want empty for EXPIR events", ev.Path)
	}
	if ev.Skill != "linear" {
		t.Errorf("Skill = %q, want %q", ev.Skill, "linear")
	}
	if ev.Time != "09:28" {
		t.Errorf("Time = %q, want %q", ev.Time, "09:28")
	}
}

func TestParseEventLine_TooFewColumns(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"empty string", ""},
		{"single value", "just-a-timestamp"},
		{"5 columns", "ts | a | b | c | d"},
		{"4 columns", "ts | a | b | c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := parseEventLine(tt.line, 0)
			if ok {
				t.Errorf("expected parse to fail for %q", tt.line)
			}
		})
	}
}

func TestParseEventLine_BlockDetection(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		isBlock bool
		action  string
	}{
		{
			name:    "contains BLOCK",
			line:    "ts | proj | claude | s | Edit | f.go | BLOCK | skill",
			isBlock: true,
			action:  "BLOCK",
		},
		{
			name:    "contains ALLOW",
			line:    "ts | proj | claude | s | Edit | f.go | ALLOW | skill",
			isBlock: false,
			action:  "ALLOW",
		},
		{
			name:    "contains SKILL-LOAD",
			line:    "ts | proj | claude | s | SKILL-LOAD | | LOAD | skill",
			isBlock: false,
			action:  "LOAD",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, ok := parseEventLine(tt.line, 0)
			if !ok {
				t.Fatalf("parse failed for %q", tt.line)
			}
			if ev.IsBlock != tt.isBlock {
				t.Errorf("IsBlock = %v, want %v", ev.IsBlock, tt.isBlock)
			}
			if ev.Action != tt.action {
				t.Errorf("Action = %q, want %q", ev.Action, tt.action)
			}
		})
	}
}

func TestParseAllEvents(t *testing.T) {
	lines := []string{
		"2026-04-13T00:00:00Z | proj | claude | abc12 | Edit | test.go | BLOCK | git",
		"not enough columns",
		"2026-04-13T00:00:01Z | proj | claude | abc12 | Write | main.go | ALLOW | linear",
	}
	events := parseAllEvents(lines)

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2 (one unparseable line should be skipped)", len(events))
	}

	// Verify the LineIdx reflects original positions in the input slice
	if events[0].LineIdx != 0 {
		t.Errorf("events[0].LineIdx = %d, want 0", events[0].LineIdx)
	}
	if events[1].LineIdx != 2 {
		t.Errorf("events[1].LineIdx = %d, want 2", events[1].LineIdx)
	}
}

func TestParseAllEvents_EmptyInput(t *testing.T) {
	events := parseAllEvents(nil)
	if len(events) != 0 {
		t.Errorf("got %d events, want 0 for nil input", len(events))
	}

	events = parseAllEvents([]string{})
	if len(events) != 0 {
		t.Errorf("got %d events, want 0 for empty input", len(events))
	}
}

func TestParseAllEvents_AllUnparseable(t *testing.T) {
	lines := []string{"bad", "also bad", "nope"}
	events := parseAllEvents(lines)
	if len(events) != 0 {
		t.Errorf("got %d events, want 0 when all lines are unparseable", len(events))
	}
}

func TestParseEventLine_ExtractsTime(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantTime string
	}{
		{
			name:     "standard RFC3339 timestamp",
			line:     "2026-04-14T07:43:33Z | Blue-Bear-Security/blueden | claude | abc12 | Edit | test.go | BLOCK | linear",
			wantTime: "07:43",
		},
		{
			name:     "different time",
			line:     "2026-04-14T15:30:00Z | proj | claude | sess1 | SKILL-LOAD | | LOAD | git",
			wantTime: "15:30",
		},
		{
			name:     "short timestamp fallback",
			line:     "2026-04-14 | proj | claude | s | Edit | f | BLOCK | x",
			wantTime: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, ok := parseEventLine(tt.line, 0)
			if !ok {
				t.Fatal("parseEventLine returned false")
			}
			if ev.Time != tt.wantTime {
				t.Errorf("Time = %q, want %q", ev.Time, tt.wantTime)
			}
		})
	}
}
