// command.go implements command-string matching for enforcement rules. It lets
// a rule require a skill based on the shell command an agent is about to run
// (e.g. "git", "aws s3 cp", "terraform*") rather than only on file paths.
//
// Matching is deliberately simple and documented:
//   - The command is split into segments on shell separators (&&, ||, |, ;,
//     newline) so a rule fires on any segment of a compound command
//     (e.g. `git add . && git commit` matches the pattern "git").
//   - Each segment is stripped of leading environment assignments (FOO=bar) and
//     a leading `sudo` so `sudo FOO=bar git push` still matches "git".
//   - A pattern containing glob metacharacters (*, ?, [) is glob-matched against
//     both the leading token and the whole segment.
//   - A plain pattern matches when the segment's leading tokens equal the
//     pattern's tokens: "git" matches `git commit`, "git push" matches
//     `git push -f` but not `git commit`.
package engine

import (
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// commandSeparators are the shell operators that split one command line into
// independently-run segments. Two-character operators are listed before the
// single-character "|" so "||" is consumed before "|".
var commandSeparators = []string{"&&", "||", "|", ";", "\n"}

// MatchCommand reports whether a shell command string matches a rule's command
// pattern. Empty, "*", and "**" patterns match any command. It returns an error
// only when a glob pattern is malformed (mirroring MatchPath).
func MatchCommand(pattern, command string) (bool, error) {
	if pattern == "" || pattern == "*" || pattern == "**" {
		return true, nil
	}
	if strings.TrimSpace(command) == "" {
		return false, nil
	}

	for _, segment := range splitCommandSegments(command) {
		tokens := commandTokens(segment)
		if len(tokens) == 0 {
			continue
		}
		matched, err := matchCommandSegment(pattern, tokens)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

// splitCommandSegments breaks a command line into segments on shell separators.
func splitCommandSegments(command string) []string {
	normalized := command
	for _, sep := range commandSeparators {
		normalized = strings.ReplaceAll(normalized, sep, "\x00")
	}
	return strings.Split(normalized, "\x00")
}

// commandTokens splits a segment into whitespace-separated tokens and strips
// leading environment-variable assignments (FOO=bar) and a leading `sudo`, so
// matching sees the actual program being invoked.
func commandTokens(segment string) []string {
	tokens := strings.Fields(segment)
	for len(tokens) > 0 {
		switch {
		case isEnvAssignment(tokens[0]):
			tokens = tokens[1:]
		case tokens[0] == "sudo":
			tokens = tokens[1:]
		default:
			return tokens
		}
	}
	return tokens
}

// isEnvAssignment reports whether a token looks like a leading shell env
// assignment such as "FOO=bar" (an identifier, then '=').
func isEnvAssignment(token string) bool {
	eq := strings.IndexByte(token, '=')
	if eq <= 0 {
		return false
	}
	for i := 0; i < eq; i++ {
		c := token[i]
		isLetter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		isDigit := c >= '0' && c <= '9'
		if !isLetter && !isDigit && c != '_' {
			return false
		}
	}
	// First character must not be a digit for a valid identifier.
	first := token[0]
	if first >= '0' && first <= '9' {
		return false
	}
	return true
}

// matchCommandSegment matches a single command segment's tokens against a
// pattern. Glob patterns match the leading token or the whole segment; plain
// patterns match when the leading tokens equal the pattern's tokens.
func matchCommandSegment(pattern string, tokens []string) (bool, error) {
	if strings.ContainsAny(pattern, "*?[") {
		leadMatch, err := doublestar.Match(pattern, tokens[0])
		if err != nil {
			return false, err
		}
		if leadMatch {
			return true, nil
		}
		return doublestar.Match(pattern, strings.Join(tokens, " "))
	}

	patternTokens := strings.Fields(pattern)
	if len(patternTokens) == 0 || len(patternTokens) > len(tokens) {
		return false, nil
	}
	for i, pt := range patternTokens {
		if tokens[i] != pt {
			return false, nil
		}
	}
	return true, nil
}
