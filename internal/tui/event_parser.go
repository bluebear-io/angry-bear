// event_parser.go provides a structured parser for the pipe-delimited event log
// format used by care-bear hooks. Centralizing the parsing avoids duplicating
// the column-extraction logic across renderEventLog, uniqueColumnValues, and
// jumpToLogEntry in dashboard.go.
package tui

import "strings"

// ParsedEvent holds the structured fields extracted from a single event log line.
type ParsedEvent struct {
	Time     string // HH:MM timestamp
	Action   string // "BLOCK", "ALLOW", "LOAD", or "EXPIR"
	Project  string // Project name
	Session  string // Short session ID
	Agent    string // "claude", "cursor", etc.
	Tool     string // Tool name or "---" for LOAD events
	Skill    string // Skill that triggered the event
	Path     string // File path involved
	IsBlock  bool   // True if BLOCK action
	IsLoad   bool   // True if SKILL-LOAD event
	IsExpire bool   // True if SKILL-TTL expire event
	LineIdx  int    // Original index in eventLines
}

// parseEventLine parses a single pipe-delimited event log line into a ParsedEvent.
// Returns the parsed event and true on success, or a zero ParsedEvent and false
// if the line cannot be parsed.
func parseEventLine(line string, lineIdx int) (ParsedEvent, bool) {
	parts := strings.Split(line, " | ")
	for j := range parts {
		parts[j] = strings.TrimSpace(parts[j])
	}

	ev := ParsedEvent{LineIdx: lineIdx}

	// Extract HH:MM from RFC3339 timestamp (parts[0])
	if len(parts) > 0 && len(parts[0]) >= 16 {
		ev.Time = parts[0][11:16]
	}

	if len(parts) >= 8 {
		// 8-col format: timestamp|project|agent|session|tool|path|action|skill
		ev.Project = parts[1]
		ev.Agent = parts[2]
		ev.Session = parts[3]
		ev.Tool = parts[4]
		ev.Path = parts[5]
		ev.Skill = parts[7]
	} else if len(parts) >= 7 {
		// 7-col format: timestamp|agent|session|tool|path|action|skill
		ev.Agent = parts[1]
		ev.Session = parts[2]
		ev.Tool = parts[3]
		ev.Path = parts[4]
		ev.Skill = parts[6]
	} else if len(parts) >= 6 {
		// 6-col format: timestamp|agent|tool|path|action|skill
		ev.Agent = parts[1]
		ev.Tool = parts[2]
		ev.Path = parts[3]
		ev.Skill = parts[5]
	} else {
		return ParsedEvent{}, false
	}

	ev.IsBlock = strings.Contains(line, "| BLOCK")
	ev.IsLoad = strings.Contains(line, "SKILL-LOAD")
	ev.IsExpire = strings.Contains(line, "SKILL-TTL")

	if ev.IsBlock {
		ev.Action = "BLOCK"
	} else if ev.IsExpire {
		ev.Action = "EXPIR"
		ev.Tool = "SKILL-TTL"
		ev.Path = ""
	} else if ev.IsLoad {
		ev.Action = "LOAD"
		ev.Tool = "\u2014" // em-dash
		ev.Path = ""
	} else {
		ev.Action = "ALLOW"
	}

	return ev, true
}

// parseAllEvents parses all event lines into structured ParsedEvent values.
// Lines that cannot be parsed are silently skipped.
func parseAllEvents(lines []string) []ParsedEvent {
	var events []ParsedEvent
	for i, line := range lines {
		if ev, ok := parseEventLine(line, i); ok {
			events = append(events, ev)
		}
	}
	return events
}
