// registry.go provides adapter lookup by name and auto-detection from JSON input.
// It maintains a map of registered adapters and can detect which adapter to use
// based on the raw JSON payload from an agent's hook invocation.
package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/Blue-Bear-Security/care-bare/internal/engine"
)

// registryDefaultHomeDir and registryDefaultBinaryPath are applied to
// all adapters created by NewRegistry. main.go sets BinaryPath at startup;
// tests may set HomeDir to redirect config file paths to temp directories.
var (
	registryDefaultHomeDir    string
	registryDefaultBinaryPath string
)

// SetRegistryDefaults sets HomeDir and BinaryPath defaults applied to all
// adapters created by NewRegistry. Pass empty strings to clear overrides.
// main.go calls this at startup with the resolved binary path;
// tests call it to inject a temporary home directory.
func SetRegistryDefaults(homeDir, binaryPath string) {
	registryDefaultHomeDir = homeDir
	registryDefaultBinaryPath = binaryPath
}

// RegistryBinaryPath returns the current registry default binary path.
// Used by the init command to display installation hints.
func RegistryBinaryPath() string {
	return registryDefaultBinaryPath
}

// AdapterRegistry holds all registered adapters indexed by name.
type AdapterRegistry struct {
	adapters map[string]HookAdapter
}

// NewRegistry creates a registry pre-populated with all built-in adapters.
// Currently registers: "claude" -> ClaudeAdapter, "cursor" -> CursorAdapter.
// Registry defaults set via SetRegistryDefaults are applied to each adapter.
func NewRegistry() *AdapterRegistry {
	return &AdapterRegistry{
		adapters: map[string]HookAdapter{
			"claude": &ClaudeAdapter{
				HomeDir:    registryDefaultHomeDir,
				BinaryPath: registryDefaultBinaryPath,
			},
			"cursor": &CursorAdapter{
				HomeDir:    registryDefaultHomeDir,
				BinaryPath: registryDefaultBinaryPath,
			},
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

// MergedProject is a repo with all agents and local paths.
type MergedProject struct {
	Name       string   // Human-readable name (repo slug or last path component)
	Path       string   // Primary absolute path (first discovered)
	RepoSlug   string   // Git repo identity (e.g., "Blue-Bear-Security/blueden")
	LocalPaths []string // All local directories that are clones of this repo
	Agents     []string // Which agents use this repo
}

// ScanAllProjects discovers projects from ALL registered adapters, merges
// duplicates (same path used by multiple agents), and returns sorted results.
// This is the single entry point for project discovery -- all agent-specific
// logic stays inside each adapter's ScanProjects().
//
// After merging, if a repo has multiple local paths, ScanAllProjects checks
// for a preferred path in ~/.care-bare/repos/{hash}-{slug}/preferences.json
// and uses it as the primary Path when valid.
func (r *AdapterRegistry) ScanAllProjects() ([]MergedProject, error) {
	// Key by repo slug (Git identity), not by local path
	byRepo := make(map[string]*MergedProject)

	for _, a := range r.adapters {
		projects, err := a.ScanProjects()
		if err != nil {
			continue
		}
		for _, p := range projects {
			// Resolve Git repo identity
			repo := engine.ResolveRepoIdentity(p.Path)
			key := p.Path // fallback: use path if no git repo
			slug := ""
			if repo != nil {
				key = repo.Slug
				slug = repo.Slug
			}

			if existing, ok := byRepo[key]; ok {
				// Add agent
				hasAgent := false
				for _, ag := range existing.Agents {
					if ag == p.Agent {
						hasAgent = true
						break
					}
				}
				if !hasAgent {
					existing.Agents = append(existing.Agents, p.Agent)
				}
				// Add local path
				hasPath := false
				for _, lp := range existing.LocalPaths {
					if lp == p.Path {
						hasPath = true
						break
					}
				}
				if !hasPath {
					existing.LocalPaths = append(existing.LocalPaths, p.Path)
				}
			} else {
				name := p.Name
				if slug != "" {
					name = slug
				}
				byRepo[key] = &MergedProject{
					Name:       name,
					Path:       p.Path,
					RepoSlug:   slug,
					LocalPaths: []string{p.Path},
					Agents:     []string{p.Agent},
				}
			}
		}
	}

	// Resolve preferred paths for multi-checkout repos.
	homeDir := registryDefaultHomeDir
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}

	var result []MergedProject
	for _, p := range byRepo {
		sort.Strings(p.Agents)
		sort.Strings(p.LocalPaths)
		// Apply preferred path for repos with multiple checkouts.
		if len(p.LocalPaths) > 1 && p.RepoSlug != "" && homeDir != "" {
			repo := &engine.RepoIdentity{
				Slug: p.RepoSlug,
				Hash: engine.ShortHash(p.RepoSlug),
			}
			repoDir := engine.RepoConfigDir(homeDir, repo)
			prefs, err := engine.LoadRepoPreferences(repoDir)
			if err == nil && prefs.PreferredPath != "" {
				// Verify the preferred path is among the discovered paths.
				for _, lp := range p.LocalPaths {
					if lp == prefs.PreferredPath {
						p.Path = prefs.PreferredPath
						break
					}
				}
			}
		}
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}
