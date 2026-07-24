package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultConfigPath is the default location for the Baton configuration file.
const DefaultConfigPath = "~/.config/baton/config.toml"

// Config represents the Baton daemon configuration.
type Config struct {
	DaemonPort       int               `json:"daemon_port"`
	WorktreeBasePath string            `json:"worktree_base_path"`
	Token            string            `json:"token"`
	Models           map[string]string `json:"models"`
	ProviderKeys     map[string]string `json:"provider_keys"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DaemonPort:   8080,
		ProviderKeys: make(map[string]string),
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
		case "providers":
			if cfg.ProviderKeys == nil {
				cfg.ProviderKeys = make(map[string]string)
			}
			cfg.ProviderKeys[key] = val
		}
	}

	return cfg, nil
}

// ConfigPath returns the standard config file path, respecting XDG_CONFIG_HOME.
func ConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "baton", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.config/baton/config.toml"
	}
	return filepath.Join(home, ".config", "baton", "config.toml")
}

// Save writes the config to path in TOML format.
func (c *Config) Save(path string) error {
	path = expandPath(path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	var buf strings.Builder
	buf.WriteString("# Baton configuration\n")
	buf.WriteString(fmt.Sprintf("daemon_port = %d\n", c.DaemonPort))
	if c.WorktreeBasePath != "" {
		buf.WriteString(fmt.Sprintf("worktree_base_path = %q\n", c.WorktreeBasePath))
	}
	if c.Token != "" {
		buf.WriteString(fmt.Sprintf("token = %q\n", c.Token))
	}
	if len(c.Models) > 0 {
		buf.WriteString("\n[models]\n")
		for k, v := range c.Models {
			buf.WriteString(fmt.Sprintf("%s = %q\n", k, v))
		}
	}
	if len(c.ProviderKeys) > 0 {
		buf.WriteString("\n[providers]\n")
		for k, v := range c.ProviderKeys {
			buf.WriteString(fmt.Sprintf("%s = %q\n", k, v))
		}
	}
	return os.WriteFile(path, []byte(buf.String()), 0644)
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
