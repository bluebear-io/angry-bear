// registry.go provides adapter lookup by name and auto-detection from JSON input.
// It maintains a map of registered adapters and can detect which adapter to use
// based on the raw JSON payload from an agent's hook invocation.
package adapter

import (
	"encoding/json"
	"fmt"
	"sort"
)

// AdapterRegistry holds all registered adapters indexed by name.
type AdapterRegistry struct {
	adapters map[string]HookAdapter
}

// NewRegistry creates a registry pre-populated with all built-in adapters.
// Currently registers: "claude" -> ClaudeAdapter, "cursor" -> CursorAdapter (stub).
func NewRegistry() *AdapterRegistry {
	return &AdapterRegistry{
		adapters: map[string]HookAdapter{
			"claude": &ClaudeAdapter{},
			"cursor": &CursorAdapter{},
		},
	}
}

// Get returns the adapter for the given name, or an error if not found.
func (r *AdapterRegistry) Get(name string) (HookAdapter, error) {
	a, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("unknown adapter: %s", name)
	}
	return a, nil
}

// AutoDetect examines raw JSON bytes and returns the matching adapter.
// Detection heuristic (order matters):
//  1. If JSON contains "cursor_version" key -> cursor adapter
//     (Cursor hooks also have hook_event_name since Cursor inherits Claude hooks)
//  2. If JSON contains "hook_event_name" key -> claude adapter
//  3. Otherwise -> error
func (r *AdapterRegistry) AutoDetect(rawJSON []byte) (HookAdapter, error) {
	var parsed map[string]any
	if err := json.Unmarshal(rawJSON, &parsed); err != nil {
		return nil, fmt.Errorf("parsing JSON for auto-detect: %w", err)
	}

	// Check for cursor_version first -- Cursor hooks inherit Claude format
	// but include cursor_version as an additional field
	if _, ok := parsed["cursor_version"]; ok {
		return r.Get("cursor")
	}

	// Check for hook_event_name -- indicates Claude Code hook format
	if _, ok := parsed["hook_event_name"]; ok {
		return r.Get("claude")
	}

	return nil, fmt.Errorf("unable to auto-detect agent: JSON does not contain 'hook_event_name' or 'cursor_version'")
}

// Names returns all registered adapter names sorted alphabetically (for help text, init detection).
func (r *AdapterRegistry) Names() []string {
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// MergedProject is a project path with all agents that use it.
type MergedProject struct {
	Name   string   // Human-readable name (last path component)
	Path   string   // Absolute path
	Agents []string // Which agents use this project
}

// ScanAllProjects discovers projects from ALL registered adapters, merges
// duplicates (same path used by multiple agents), and returns sorted results.
// This is the single entry point for project discovery — all agent-specific
// logic stays inside each adapter's ScanProjects().
func (r *AdapterRegistry) ScanAllProjects() ([]MergedProject, error) {
	merged := make(map[string]*MergedProject)

	for _, a := range r.adapters {
		projects, err := a.ScanProjects()
		if err != nil {
			continue // Skip adapters that fail
		}
		for _, p := range projects {
			if existing, ok := merged[p.Path]; ok {
				// Add agent if not already present
				found := false
				for _, ag := range existing.Agents {
					if ag == p.Agent {
						found = true
						break
					}
				}
				if !found {
					existing.Agents = append(existing.Agents, p.Agent)
				}
			} else {
				merged[p.Path] = &MergedProject{
					Name:   p.Name,
					Path:   p.Path,
					Agents: []string{p.Agent},
				}
			}
		}
	}

	var result []MergedProject
	for _, p := range merged {
		sort.Strings(p.Agents)
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})
	return result, nil
}
