package agents

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseAgentFile reads a .toml file and returns an AgentDefinition.
func ParseAgentFile(path string) (AgentDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentDefinition{}, fmt.Errorf("reading agent file: %w", err)
	}

	return parseAgentTOML(string(data))
}

// parseAgentTOML parses TOML-formatted string into an AgentDefinition.
func parseAgentTOML(data string) (AgentDefinition, error) {
	var def AgentDefinition
	var currentSection string

	for _, raw := range strings.Split(data, "\n") {
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

		switch currentSection {
		case "":
			if err := setTopLevel(&def, key, val); err != nil {
				return AgentDefinition{}, err
			}
		case "prompt":
			if err := setPrompt(&def, key, val); err != nil {
				return AgentDefinition{}, err
			}
		case "tools":
			if err := setTools(&def, key, val); err != nil {
				return AgentDefinition{}, err
			}
		case "worktree":
			if err := setWorktree(&def, key, val); err != nil {
				return AgentDefinition{}, err
			}
		case "timeout":
			if err := setTimeout(&def, key, val); err != nil {
				return AgentDefinition{}, err
			}
		case "reporting":
			if err := setReporting(&def, key, val); err != nil {
				return AgentDefinition{}, err
			}
		default:
			return AgentDefinition{}, fmt.Errorf("unknown section: %q", currentSection)
		}
	}

	return def, nil
}

// setTopLevel sets a top-level field on def from a parsed key=value pair.
func setTopLevel(def *AgentDefinition, key, val string) error {
	val = strings.Trim(val, `"`)
	switch key {
	case "name":
		def.Name = val
	case "description":
		def.Description = val
	case "model":
		def.Model = val
	default:
		return fmt.Errorf("unknown top-level key: %q", key)
	}
	return nil
}

// setPrompt sets a field on def.Prompt from a parsed key=value pair.
func setPrompt(def *AgentDefinition, key, val string) error {
	val = strings.Trim(val, `"`)
	switch key {
	case "system":
		def.Prompt.System = val
	case "template":
		def.Prompt.Template = val
	default:
		return fmt.Errorf("unknown prompt key: %q", key)
	}
	return nil
}

// setTools sets a field on def.Tools from a parsed key=value pair.
func setTools(def *AgentDefinition, key, val string) error {
	switch key {
	case "write":
		b, err := parseBool(val)
		if err != nil {
			return fmt.Errorf("invalid tools.write: %w", err)
		}
		def.Tools.Write = b
	case "edit":
		b, err := parseBool(val)
		if err != nil {
			return fmt.Errorf("invalid tools.edit: %w", err)
		}
		def.Tools.Edit = b
	case "bash":
		def.Tools.Bash = strings.Trim(val, `"`)
	case "search":
		b, err := parseBool(val)
		if err != nil {
			return fmt.Errorf("invalid tools.search: %w", err)
		}
		def.Tools.Search = b
	case "read":
		b, err := parseBool(val)
		if err != nil {
			return fmt.Errorf("invalid tools.read: %w", err)
		}
		def.Tools.Read = b
	default:
		return fmt.Errorf("unknown tools key: %q", key)
	}
	return nil
}

// setWorktree sets a field on def.Worktree from a parsed key=value pair.
func setWorktree(def *AgentDefinition, key, val string) error {
	switch key {
	case "required":
		b, err := parseBool(val)
		if err != nil {
			return fmt.Errorf("invalid worktree.required: %w", err)
		}
		def.Worktree.Required = b
	default:
		return fmt.Errorf("unknown worktree key: %q", key)
	}
	return nil
}

// setTimeout sets a field on def.Timeout from a parsed key=value pair.
func setTimeout(def *AgentDefinition, key, val string) error {
	switch key {
	case "max_seconds":
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return fmt.Errorf("invalid timeout.max_seconds %q: %w", val, err)
		}
		def.Timeout.MaxSeconds = n
	default:
		return fmt.Errorf("unknown timeout key: %q", key)
	}
	return nil
}

// setReporting sets a field on def.Reporting from a parsed key=value pair.
func setReporting(def *AgentDefinition, key, val string) error {
	val = strings.Trim(val, `"`)
	switch key {
	case "format":
		def.Reporting.Format = val
	default:
		return fmt.Errorf("unknown reporting key: %q", key)
	}
	return nil
}

// parseBool parses "true" or "false" into a bool. Unlike strconv.ParseBool it
// only accepts the exact lowercase strings used in TOML.
func parseBool(s string) (bool, error) {
	s = strings.TrimSpace(s)
	switch s {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool: %q", s)
	}
}
