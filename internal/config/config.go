package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// DefaultConfigPath is the default location for the Baton configuration file.
const DefaultConfigPath = "~/.config/baton/config.toml"

// Config represents the Baton daemon configuration.
// Parsed from ~/.config/baton/config.toml.
// JSON struct tags are placeholders for TOML tags (pending TOML library import).
type Config struct {
	DaemonPort       int               `json:"daemon_port"`
	WorktreeBasePath string            `json:"worktree_base_path"`
	Token            string            `json:"token"`
	Models           map[string]string `json:"models"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DaemonPort: 8080,
		Models: map[string]string{
			"brain":      "sonnet",
			"diagnostic": "sonnet",
			"fixer":      "sonnet",
			"reviewer":   "sonnet",
		},
	}
}

// Parse reads and parses a TOML config file at path.
func Parse(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg := DefaultConfig()
	var currentSection string

	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}

		before, after, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key := strings.TrimSpace(before)
		val := strings.TrimSpace(after)
		val = strings.Trim(val, `"`)

		switch currentSection {
		case "":
			switch key {
			case "daemon_port":
				n, err := strconv.Atoi(val)
				if err != nil {
					return nil, fmt.Errorf("invalid daemon_port %q: %w", val, err)
				}
				cfg.DaemonPort = n
			case "worktree_base_path":
				cfg.WorktreeBasePath = val
			case "token":
				cfg.Token = val
			}
		case "models":
			if cfg.Models == nil {
				cfg.Models = make(map[string]string)
			}
			cfg.Models[key] = val
		}
	}

	return cfg, nil
}
