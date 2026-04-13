// loaded.go provides utilities for reading loaded skill state from session files.
// It consolidates the logic for scanning state directories and building a map
// of which skills are loaded by which agents.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// SkillStatus tracks which agents have loaded a skill.
type SkillStatus struct {
	Agents []string // e.g. ["claude", "cursor"]
}

// CollectLoadedSkills reads all session state files in the given directory and
// returns a map of skill names to their load status, including which agents
// have loaded each skill.
func CollectLoadedSkills(stateDir string) map[string]*SkillStatus {
	loaded := make(map[string]*SkillStatus)

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return loaded
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(stateDir, e.Name()))
		if err != nil {
			continue
		}
		var ss SessionState
		if err := json.Unmarshal(data, &ss); err != nil {
			continue
		}
		agent := ss.Agent
		if agent == "" {
			agent = "unknown"
		}
		for _, skill := range ss.InvokedSkills {
			if loaded[skill] == nil {
				loaded[skill] = &SkillStatus{}
			}
			// Add agent if not already in list.
			found := false
			for _, a := range loaded[skill].Agents {
				if a == agent {
					found = true
					break
				}
			}
			if !found {
				loaded[skill].Agents = append(loaded[skill].Agents, agent)
			}
		}
	}

	return loaded
}
