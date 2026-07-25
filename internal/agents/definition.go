package agents

// AgentDefinition describes an agent that the brain can dispatch.
type AgentDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Model       string          `json:"model"`
	Prompt      PromptConfig    `json:"prompt"`
	Tools       ToolsConfig     `json:"tools"`
	Worktree    WorktreeConfig  `json:"worktree"`
	Timeout     TimeoutConfig   `json:"timeout"`
	Reporting   ReportingConfig `json:"reporting"`
}

// PromptConfig holds the system instruction and template for an agent.
type PromptConfig struct {
	System   string `json:"system"`
	Template string `json:"template"`
}

// ToolsConfig describes which tools an agent is allowed to use.
type ToolsConfig struct {
	Write  bool   `json:"write"`
	Edit   bool   `json:"edit"`
	Bash   string `json:"bash"` // "scoped", "full", or "" / "false"
	Search bool   `json:"search"`
	Read   bool   `json:"read"`
}

// WorktreeConfig controls whether a worktree is required.
type WorktreeConfig struct {
	Required bool `json:"required"`
}

// TimeoutConfig sets the maximum execution time for an agent.
type TimeoutConfig struct {
	MaxSeconds int `json:"max_seconds"`
}

// ReportingConfig controls how the agent reports results.
type ReportingConfig struct {
	Format string `json:"format"` // "json" or "text"
}

// DefaultFixer returns the built-in fixer agent definition.
func DefaultFixer() AgentDefinition {
	return AgentDefinition{
		Name:        "fixer",
		Description: "Fixes bugs with minimal surgical changes",
		Model:       "deepseek-v4-flash",
		Prompt: PromptConfig{
			System:   "You are a code fixer. Make minimal, surgical changes. Use Edit or Write tools to modify files. Do not just describe changes — make them.",
			Template: "Fix the following issue:\n\nWork Unit: {work_unit_id}\nDescription: {description}\nIssues:\n{issues}\n\nInstructions:\n1. Read the relevant files first.\n2. Use Edit or Write to make minimal changes. Touch only what is necessary.\n3. After all changes, run the build command if available.\n4. If tests exist, run them.\n\nAfter completing ALL changes, respond with this JSON:\n{\"success\": true, \"summary\": \"what you changed\", \"changed_files\": [\"file1.go\"], \"build_passed\": true, \"tests_passed\": true}\n\nSet success to false if you could not complete the changes. Respond ONLY with the JSON.",
		},
		Tools: ToolsConfig{
			Write:  true,
			Edit:   true,
			Bash:   "scoped",
			Search: true,
			Read:   true,
		},
		Worktree:  WorktreeConfig{Required: true},
		Timeout:   TimeoutConfig{MaxSeconds: 1800},
		Reporting: ReportingConfig{Format: "json"},
	}
}

// DefaultReviewer returns the built-in reviewer agent definition.
func DefaultReviewer() AgentDefinition {
	return AgentDefinition{
		Name:        "reviewer",
		Description: "Reviews code for correctness and quality",
		Model:       "deepseek-v4-flash",
		Prompt: PromptConfig{
			System:   "You are a code reviewer. Read the code and find issues, but do not modify any files. Provide a thorough review.",
			Template: "Review the following changes:\n\nFiles: {files}\nDescription: {description}\n\nIdentify any bugs, logic errors, or quality issues. Do not edit any files.",
		},
		Tools: ToolsConfig{
			Write:  false,
			Edit:   false,
			Bash:   "full",
			Search: true,
			Read:   true,
		},
		Worktree:  WorktreeConfig{Required: false},
		Timeout:   TimeoutConfig{MaxSeconds: 600},
		Reporting: ReportingConfig{Format: "json"},
	}
}

// DefaultDiagnostic returns the built-in diagnostic agent definition.
func DefaultDiagnostic() AgentDefinition {
	return AgentDefinition{
		Name:        "diagnostic",
		Description: "Analyzes code for bugs and issues",
		Model:       "deepseek-v4-flash",
		Prompt: PromptConfig{
			System: "You are a code diagnostician. Analyze the following input and produce a structured JSON issue list.",
			Template: `Input type: {input_type}
Input: {input}

For each issue, provide:
- id: unique short identifier
- description: what is wrong
- confidence: "high", "medium", or "low"
- group: semantic category (e.g., "null_reference", "logic_error", "performance")
- suspected_files: list of file paths likely containing the bug
- dependency_edges: list of other issue IDs this issue depends on (empty if none)

Group related issues into work units. A work unit is a set of related issues that should be fixed together.
Each work unit needs:
- id: "wu_01" style
- description: what this work unit addresses
- issues: list of issue IDs

Respond ONLY with valid JSON, no other text. The JSON must have this exact structure:
{"issues": [{"id": "...", "description": "...", "confidence": "...", "group": "...", "suspected_files": ["..."], "dependency_edges": ["..."]}], "work_units": [{"id": "...", "description": "...", "issues": ["..."]}]}`,
		},
		Tools: ToolsConfig{
			Write:  false,
			Edit:   false,
			Bash:   "scoped",
			Search: true,
			Read:   true,
		},
		Worktree:  WorktreeConfig{Required: false},
		Timeout:   TimeoutConfig{MaxSeconds: 300},
		Reporting: ReportingConfig{Format: "json"},
	}
}

// DefaultResearcher returns the built-in researcher agent definition.
func DefaultResearcher() AgentDefinition {
	return AgentDefinition{
		Name:        "researcher",
		Description: "Researches codebases and finds information",
		Model:       "deepseek-v4-flash",
		Prompt: PromptConfig{
			System:   "You are a code researcher. Explore the codebase to answer questions. Read files and search for patterns. Do not modify any files.",
			Template: "Research the following:\n\nQuestion: {question}\nContext: {context}",
		},
		Tools: ToolsConfig{
			Write:  false,
			Edit:   false,
			Bash:   "full",
			Search: true,
			Read:   true,
		},
		Worktree:  WorktreeConfig{Required: false},
		Timeout:   TimeoutConfig{MaxSeconds: 600},
		Reporting: ReportingConfig{Format: "json"},
	}
}

// DefaultTestWriter returns the built-in test writer agent definition.
func DefaultTestWriter() AgentDefinition {
	return AgentDefinition{
		Name:        "test_writer",
		Description: "Writes tests for code",
		Model:       "deepseek-v4-flash",
		Prompt: PromptConfig{
			System:   "You are a test writer. Write comprehensive tests for the given code. Use the project's existing test framework and patterns.",
			Template: "Write tests for:\n\nFiles: {files}\nDescription: {description}\n\nMake sure tests compile and pass. Use Edit or Write to create test files.",
		},
		Tools: ToolsConfig{
			Write:  true,
			Edit:   true,
			Bash:   "scoped",
			Search: true,
			Read:   true,
		},
		Worktree:  WorktreeConfig{Required: true},
		Timeout:   TimeoutConfig{MaxSeconds: 1800},
		Reporting: ReportingConfig{Format: "json"},
	}
}

// DefaultBrain returns the built-in brain orchestrator agent definition.
// The brain is not dispatched via AgentRuntime -- it is the persistent
// orchestrator session. Its tools are custom (tool-call JSON blocks),
// not OpenCode built-in tools.
func DefaultBrain() AgentDefinition {
	return AgentDefinition{
		Name:        "brain",
		Description: "Orchestrator agent that dispatches and coordinates subagents",
		Model:       "deepseek-v4-pro",
		Prompt: PromptConfig{
			System:   "",
			Template: "",
		},
		Tools: ToolsConfig{
			Write:  false,
			Edit:   false,
			Bash:   "",
			Search: false,
			Read:   false,
		},
		Worktree:  WorktreeConfig{Required: false},
		Timeout:   TimeoutConfig{MaxSeconds: 300},
		Reporting: ReportingConfig{Format: "text"},
	}
}

// builtinAgents returns all built-in agent definitions indexed by name.
func builtinAgents() map[string]AgentDefinition {
	defs := []AgentDefinition{
		DefaultFixer(),
		DefaultReviewer(),
		DefaultDiagnostic(),
		DefaultResearcher(),
		DefaultTestWriter(),
	}
	m := make(map[string]AgentDefinition, len(defs))
	for _, d := range defs {
		m[d.Name] = d
	}
	return m
}
