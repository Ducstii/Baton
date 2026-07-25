package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// userAgentsDir is the user-level directory for custom agent definitions.
const userAgentsDir = "~/.config/baton/agents"

// projAgentsDir is the project-level directory for custom agent definitions.
const projAgentsDir = ".baton/agents"

// Registry loads agent definitions from built-in, user, and project sources.
type Registry struct {
	builtin map[string]AgentDefinition
	userDir string
	projDir string
	// Cached loaded definitions.
	userDefs map[string]AgentDefinition
	projDefs map[string]AgentDefinition
}

// NewRegistry creates a Registry with built-in agents loaded from Default*
// functions, ready to overlay user and project definitions.
func NewRegistry(projectPath string) *Registry {
	r := &Registry{
		builtin: builtinAgents(),
		userDir: expandHome(userAgentsDir),
		projDir: filepath.Join(projectPath, projAgentsDir),
	}
	// Attempt initial load; errors are non-fatal — we fall back to built-ins.
	_ = r.Reload()
	return r
}

// Get returns the highest-precedence definition for the given agent name.
// Precedence: project > user > built-in.
func (r *Registry) Get(name string) (AgentDefinition, bool) {
	if r.projDefs != nil {
		if d, ok := r.projDefs[name]; ok {
			return d, true
		}
	}
	if r.userDefs != nil {
		if d, ok := r.userDefs[name]; ok {
			return d, true
		}
	}
	d, ok := r.builtin[name]
	return d, ok
}

// List returns all known agent definitions after merging all three tiers.
// Project definitions override user definitions, which override built-in.
func (r *Registry) List() []AgentDefinition {
	merged := make(map[string]AgentDefinition)

	// Start with built-in.
	for name, def := range r.builtin {
		merged[name] = def
	}
	// Overlay user definitions.
	if r.userDefs != nil {
		for name, def := range r.userDefs {
			merged[name] = def
		}
	}
	// Overlay project definitions.
	if r.projDefs != nil {
		for name, def := range r.projDefs {
			merged[name] = def
		}
	}

	result := make([]AgentDefinition, 0, len(merged))
	for _, def := range merged {
		result = append(result, def)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Reload re-scans the user and project directories for TOML agent files.
func (r *Registry) Reload() error {
	userDefs, err := loadDir(r.userDir)
	if err != nil {
		// User directory may not exist; that is acceptable.
		r.userDefs = nil
	} else {
		r.userDefs = userDefs
	}

	projDefs, err := loadDir(r.projDir)
	if err != nil {
		r.projDefs = nil
	} else {
		r.projDefs = projDefs
	}

	return nil
}

// loadDir reads all .toml files in dir and returns parsed agent definitions
// indexed by name. Returns an error if the directory does not exist.
func loadDir(dir string) (map[string]AgentDefinition, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading agents dir %q: %w", dir, err)
	}

	defs := make(map[string]AgentDefinition)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		def, err := ParseAgentFile(path)
		if err != nil {
			return nil, fmt.Errorf("parsing %q: %w", path, err)
		}
		if def.Name == "" {
			return nil, fmt.Errorf("agent file %q has no name field", path)
		}
		if _, exists := defs[def.Name]; exists {
			return nil, fmt.Errorf("duplicate agent name %q in directory %q", def.Name, dir)
		}
		defs[def.Name] = def
	}

	return defs, nil
}

// expandHome replaces a leading "~/" with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
