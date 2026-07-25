package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Default* functions
// ---------------------------------------------------------------------------

func TestDefaultFixer(t *testing.T) {
	d := DefaultFixer()
	checkRequired(t, d, "fixer")
	if !d.Tools.Write {
		t.Error("DefaultFixer should have Write=true")
	}
	if !d.Tools.Edit {
		t.Error("DefaultFixer should have Edit=true")
	}
	if d.Tools.Bash != "scoped" {
		t.Errorf("DefaultFixer Bash = %q, want %q", d.Tools.Bash, "scoped")
	}
	if !d.Worktree.Required {
		t.Error("DefaultFixer should have Worktree.Required=true")
	}
	if d.Timeout.MaxSeconds <= 0 {
		t.Error("DefaultFixer should have positive Timeout.MaxSeconds")
	}
	if d.Prompt.System == "" {
		t.Error("DefaultFixer should have a non-empty Prompt.System")
	}
	if d.Prompt.Template == "" {
		t.Error("DefaultFixer should have a non-empty Prompt.Template")
	}
}

func TestDefaultReviewer(t *testing.T) {
	d := DefaultReviewer()
	checkRequired(t, d, "reviewer")
	if d.Tools.Write {
		t.Error("DefaultReviewer should have Write=false")
	}
	if d.Tools.Edit {
		t.Error("DefaultReviewer should have Edit=false")
	}
	if d.Tools.Bash != "full" {
		t.Errorf("DefaultReviewer Bash = %q, want %q", d.Tools.Bash, "full")
	}
	if d.Worktree.Required {
		t.Error("DefaultReviewer should have Worktree.Required=false")
	}
}

func TestDefaultDiagnostic(t *testing.T) {
	d := DefaultDiagnostic()
	checkRequired(t, d, "diagnostic")
	if d.Tools.Write {
		t.Error("DefaultDiagnostic should have Write=false")
	}
	if d.Tools.Edit {
		t.Error("DefaultDiagnostic should have Edit=false")
	}
	if d.Worktree.Required {
		t.Error("DefaultDiagnostic should have Worktree.Required=false")
	}
}

func TestDefaultResearcher(t *testing.T) {
	d := DefaultResearcher()
	checkRequired(t, d, "researcher")
	if d.Tools.Write {
		t.Error("DefaultResearcher should have Write=false")
	}
	if d.Tools.Edit {
		t.Error("DefaultResearcher should have Edit=false")
	}
	if d.Tools.Bash != "full" {
		t.Errorf("DefaultResearcher Bash = %q, want %q", d.Tools.Bash, "full")
	}
}

func TestDefaultTestWriter(t *testing.T) {
	d := DefaultTestWriter()
	checkRequired(t, d, "test_writer")
	if !d.Tools.Write {
		t.Error("DefaultTestWriter should have Write=true")
	}
	if !d.Tools.Edit {
		t.Error("DefaultTestWriter should have Edit=true")
	}
	if !d.Worktree.Required {
		t.Error("DefaultTestWriter should have Worktree.Required=true")
	}
}

func checkRequired(t *testing.T, d AgentDefinition, name string) {
	t.Helper()
	if d.Name == "" {
		t.Errorf("%s: Name is required", name)
	}
	if d.Description == "" {
		t.Errorf("%s: Description is required", name)
	}
	if d.Model == "" {
		t.Errorf("%s: Model is required", name)
	}
}

// ---------------------------------------------------------------------------
// ParseAgentFile
// ---------------------------------------------------------------------------

func TestParseAgentFile(t *testing.T) {
	content := `name = "custom-fixer"
description = "My custom fixer agent"
model = "opus"

[prompt]
system = "You are a fixer."
template = "Fix this: {description}"

[tools]
write = true
edit = true
bash = "scoped"
search = true
read = true

[worktree]
required = true

[timeout]
max_seconds = 3600

[reporting]
format = "json"
`
	path := tempAgentFile(t, content)

	def, err := ParseAgentFile(path)
	if err != nil {
		t.Fatalf("ParseAgentFile: %v", err)
	}

	if def.Name != "custom-fixer" {
		t.Errorf("Name = %q, want %q", def.Name, "custom-fixer")
	}
	if def.Description != "My custom fixer agent" {
		t.Errorf("Description = %q, want %q", def.Description, "My custom fixer agent")
	}
	if def.Model != "opus" {
		t.Errorf("Model = %q, want %q", def.Model, "opus")
	}

	// Prompt section.
	if def.Prompt.System != "You are a fixer." {
		t.Errorf("Prompt.System = %q, want %q", def.Prompt.System, "You are a fixer.")
	}
	if def.Prompt.Template != "Fix this: {description}" {
		t.Errorf("Prompt.Template = %q, want %q", def.Prompt.Template, "Fix this: {description}")
	}

	// Tools section.
	if !def.Tools.Write {
		t.Error("Tools.Write should be true")
	}
	if !def.Tools.Edit {
		t.Error("Tools.Edit should be true")
	}
	if def.Tools.Bash != "scoped" {
		t.Errorf("Tools.Bash = %q, want %q", def.Tools.Bash, "scoped")
	}
	if !def.Tools.Search {
		t.Error("Tools.Search should be true")
	}
	if !def.Tools.Read {
		t.Error("Tools.Read should be true")
	}

	// Worktree section.
	if !def.Worktree.Required {
		t.Error("Worktree.Required should be true")
	}

	// Timeout section.
	if def.Timeout.MaxSeconds != 3600 {
		t.Errorf("Timeout.MaxSeconds = %d, want %d", def.Timeout.MaxSeconds, 3600)
	}

	// Reporting section.
	if def.Reporting.Format != "json" {
		t.Errorf("Reporting.Format = %q, want %q", def.Reporting.Format, "json")
	}
}

func TestParseAgentFile_Minimal(t *testing.T) {
	content := `name = "minimal"
description = "Minimal agent"
model = "sonnet"
`
	path := tempAgentFile(t, content)

	def, err := ParseAgentFile(path)
	if err != nil {
		t.Fatalf("ParseAgentFile: %v", err)
	}

	if def.Name != "minimal" {
		t.Errorf("Name = %q, want %q", def.Name, "minimal")
	}
	if def.Tools.Bash != "" {
		t.Errorf("Tools.Bash = %q, want empty", def.Tools.Bash)
	}
}

func TestParseAgentFile_MissingFile(t *testing.T) {
	_, err := ParseAgentFile("/nonexistent/path.toml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseAgentFile_Comments(t *testing.T) {
	content := `# This is a comment
name = "commented"
description = "Has comments" # inline comment
model = "haiku"
`
	path := tempAgentFile(t, content)

	def, err := ParseAgentFile(path)
	if err != nil {
		t.Fatalf("ParseAgentFile: %v", err)
	}
	if def.Name != "commented" {
		t.Errorf("Name = %q, want %q", def.Name, "commented")
	}
}

// ---------------------------------------------------------------------------
// ExpandTemplate
// ---------------------------------------------------------------------------

func TestExpandTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		params   map[string]string
		want     string
	}{
		{
			name:     "single variable",
			template: "Hello, {name}!",
			params:   map[string]string{"name": "World"},
			want:     "Hello, World!",
		},
		{
			name:     "multiple variables",
			template: "{greeting}, {name}!",
			params:   map[string]string{"greeting": "Hi", "name": "Alice"},
			want:     "Hi, Alice!",
		},
		{
			name:     "no variables",
			template: "Hello, World!",
			params:   map[string]string{"name": "Bob"},
			want:     "Hello, World!",
		},
		{
			name:     "missing variable left as-is",
			template: "Hello, {name}!",
			params:   map[string]string{},
			want:     "Hello, {name}!",
		},
		{
			name:     "multi-line template",
			template: "Line 1: {a}\nLine 2: {b}",
			params:   map[string]string{"a": "foo", "b": "bar"},
			want:     "Line 1: foo\nLine 2: bar",
		},
		{
			name:     "empty template",
			template: "",
			params:   map[string]string{"x": "y"},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandTemplate(tt.template, tt.params)
			if got != tt.want {
				t.Errorf("ExpandTemplate =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

func TestRegistry_Get_Builtin(t *testing.T) {
	r := NewRegistry(t.TempDir())

	d, ok := r.Get("fixer")
	if !ok {
		t.Fatal("Get(fixer) should find built-in agent")
	}
	if d.Name != "fixer" {
		t.Errorf("Name = %q, want %q", d.Name, "fixer")
	}
	if d.Prompt.System == "" {
		t.Error("built-in fixer should have non-empty system prompt")
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry(t.TempDir())

	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("Get(nonexistent) should return false")
	}
}

func TestRegistry_Get_ProjectOverridesBuiltin(t *testing.T) {
	projectDir := t.TempDir()
	agentsDir := filepath.Join(projectDir, ".baton", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `name = "fixer"
description = "Overridden fixer"
model = "opus"

[prompt]
system = "Overridden system"
template = "Overridden template"

[tools]
write = false
edit = false
bash = "full"
search = false
read = true

[worktree]
required = false

[timeout]
max_seconds = 60

[reporting]
format = "text"
`
	path := filepath.Join(agentsDir, "fixer.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(projectDir)

	d, ok := r.Get("fixer")
	if !ok {
		t.Fatal("Get(fixer) should find the agent")
	}

	// Verify the project-level override took effect.
	if d.Description != "Overridden fixer" {
		t.Errorf("Description = %q, want %q", d.Description, "Overridden fixer")
	}
	if d.Model != "opus" {
		t.Errorf("Model = %q, want %q", d.Model, "opus")
	}
	if d.Prompt.System != "Overridden system" {
		t.Errorf("Prompt.System = %q, want %q", d.Prompt.System, "Overridden system")
	}
	if d.Tools.Write {
		t.Error("Tools.Write should be false from override")
	}
	if d.Tools.Bash != "full" {
		t.Errorf("Tools.Bash = %q, want %q", d.Tools.Bash, "full")
	}
}

func TestRegistry_List(t *testing.T) {
	projectDir := t.TempDir()
	r := NewRegistry(projectDir)

	list := r.List()
	if len(list) == 0 {
		t.Fatal("List() should return at least built-in agents")
	}

	// Verify all built-in agents are present.
	names := make(map[string]bool)
	for _, d := range list {
		names[d.Name] = true
	}
	for _, want := range []string{"fixer", "reviewer", "diagnostic", "researcher", "test_writer"} {
		if !names[want] {
			t.Errorf("List() missing built-in agent %q", want)
		}
	}
}

func TestRegistry_List_ProjectOverrides(t *testing.T) {
	projectDir := t.TempDir()
	agentsDir := filepath.Join(projectDir, ".baton", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write an override for fixer.
	content := `name = "fixer"
description = "Overridden fixer"
model = "opus"

[prompt]
system = "Override"
template = "Override template"

[tools]
read = true

[timeout]
max_seconds = 60

[reporting]
format = "text"
`
	path := filepath.Join(agentsDir, "fixer.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a new project-only agent.
	content2 := `name = "project-helper"
description = "Project-specific helper"
model = "haiku"

[prompt]
system = "Helper"
template = "Help with: {task}"

[tools]
read = true

[timeout]
max_seconds = 120

[reporting]
format = "text"
`
	path2 := filepath.Join(agentsDir, "project_helper.toml")
	if err := os.WriteFile(path2, []byte(content2), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry(projectDir)
	list := r.List()

	names := make(map[string]string)
	for _, d := range list {
		names[d.Name] = d.Description
	}

	// Project override should be active.
	if names["fixer"] != "Overridden fixer" {
		t.Errorf("fixer description = %q, want %q", names["fixer"], "Overridden fixer")
	}

	// New project-only agent should appear.
	if names["project-helper"] != "Project-specific helper" {
		t.Errorf("project-helper description = %q, want %q", names["project-helper"], "Project-specific helper")
	}

	// Built-in agents should still be present.
	if names["reviewer"] == "" {
		t.Error("reviewer should still be in list")
	}
}

func TestRegistry_Reload(t *testing.T) {
	projectDir := t.TempDir()
	r := NewRegistry(projectDir)

	// Initially no project override.
	d, ok := r.Get("fixer")
	if !ok {
		t.Fatal("Get(fixer) should find built-in")
	}
	origDesc := d.Description

	// Create project override after registry is built.
	agentsDir := filepath.Join(projectDir, ".baton", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `name = "fixer"
description = "Reloaded fixer"
model = "sonnet"

[prompt]
system = "Reloaded"
template = "Reloaded template"

[tools]
read = true

[timeout]
max_seconds = 30

[reporting]
format = "text"
`
	path := filepath.Join(agentsDir, "fixer.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := r.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	d, ok = r.Get("fixer")
	if !ok {
		t.Fatal("Get(fixer) after reload should succeed")
	}
	if d.Description == origDesc {
		t.Error("Reload should have picked up the project override")
	}
	if d.Description != "Reloaded fixer" {
		t.Errorf("Description = %q, want %q", d.Description, "Reloaded fixer")
	}
}

// ---------------------------------------------------------------------------
// extractJSON (preserved from old system)
// ---------------------------------------------------------------------------

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     string
		wantFail bool
	}{
		{
			name:  "raw json object",
			input: `{"issues": [], "work_units": []}`,
			want:  `{"issues": [], "work_units": []}`,
		},
		{
			name:  "fenced with json marker",
			input: "```json\n{\"issues\": [{\"id\": \"i1\"}], \"work_units\": []}\n```",
			want:  `{"issues": [{"id": "i1"}], "work_units": []}`,
		},
		{
			name:  "fenced without language",
			input: "```\n{\"success\": true}\n```",
			want:  `{"success": true}`,
		},
		{
			name:  "text before and after",
			input: "Here is the result:\n\n```json\n{\"issues\": []}\n```\nDone.",
			want:  `{"issues": []}`,
		},
		{
			name:  "json in text without fences",
			input: `The result is {"success": false, "summary": "nope"} and that's it.`,
			want:  `{"success": false, "summary": "nope"}`,
		},
		{
			name:  "nested braces",
			input: `{"issues": [{"id": "i1", "suspected_files": ["a.go", "b.go"]}], "work_units": [{"id": "wu_01", "description": "fix", "issues": ["i1"]}]}`,
			want:  `{"issues": [{"id": "i1", "suspected_files": ["a.go", "b.go"]}], "work_units": [{"id": "wu_01", "description": "fix", "issues": ["i1"]}]}`,
		},
		{
			name:     "empty string",
			input:    "",
			wantFail: true,
		},
		{
			name:     "no json content",
			input:    "just some text without brackets",
			wantFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if tt.wantFail && got != "" {
				t.Errorf("expected failure, got %q", got)
				return
			}
			if !tt.wantFail && got == "" {
				t.Fatalf("extractJSON returned empty, want %q", tt.want)
			}
			if !tt.wantFail && got != tt.want {
				t.Errorf("extractJSON =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}

func TestParseDiagnosticResult(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantOK        bool
		wantIssues    int
		wantWorkUnits int
	}{
		{
			name: "full valid result",
			input: `{
				"issues": [
					{
						"id": "i1",
						"description": "nil pointer dereference in login",
						"confidence": "high",
						"group": "null_reference",
						"suspected_files": ["src/auth/login.go"],
						"dependency_edges": []
					},
					{
						"id": "i2",
						"description": "unclosed file handle",
						"confidence": "medium",
						"group": "resource_leak",
						"suspected_files": ["src/util/files.go"],
						"dependency_edges": []
					}
				],
				"work_units": [
					{
						"id": "wu_01",
						"description": "fix auth issues",
						"issues": ["i1"]
					}
				]
			}`,
			wantOK:        true,
			wantIssues:    2,
			wantWorkUnits: 1,
		},
		{
			name: "single issue with dependency edge",
			input: `{
				"issues": [
					{
						"id": "i1",
						"description": "crash on empty input",
						"confidence": "high",
						"group": "logic_error",
						"suspected_files": ["src/main.go"],
						"dependency_edges": []
					}
				],
				"work_units": [
					{
						"id": "wu_01",
						"description": "fix input handling",
						"issues": ["i1"]
					}
				]
			}`,
			wantOK:        true,
			wantIssues:    1,
			wantWorkUnits: 1,
		},
		{
			name: "empty issues list",
			input: `{
				"issues": [],
				"work_units": []
			}`,
			wantOK: false,
		},
		{
			name: "malformed JSON",
			input: `{
				"issues": [bad data here]
			}`,
			wantOK: false,
		},
		{
			name: "work unit with no issues",
			input: `{
				"issues": [
					{
						"id": "i1",
						"description": "test",
						"confidence": "low",
						"group": "other",
						"suspected_files": [],
						"dependency_edges": []
					}
				],
				"work_units": [
					{
						"id": "wu_01",
						"description": "empty work unit",
						"issues": []
					}
				]
			}`,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ParseDiagnosticResult([]byte(tt.input))
			if tt.wantOK && err != nil {
				t.Fatalf("ParseDiagnosticResult: %v", err)
			}
			if !tt.wantOK && err == nil {
				t.Fatal("ParseDiagnosticResult: expected error, got nil")
			}
			if !tt.wantOK {
				return
			}
			if len(res.Issues) != tt.wantIssues {
				t.Errorf("len(Issues) = %d, want %d", len(res.Issues), tt.wantIssues)
			}
			if len(res.WorkUnits) != tt.wantWorkUnits {
				t.Errorf("len(WorkUnits) = %d, want %d", len(res.WorkUnits), tt.wantWorkUnits)
			}
		})
	}
}

func TestParseFixerResult(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantOK bool
		want   *FixerResult
	}{
		{
			name: "successful fix",
			input: `{
				"success": true,
				"summary": "Added nil check in login handler",
				"changed_files": ["src/auth/login.go"],
				"build_passed": true,
				"tests_passed": true
			}`,
			wantOK: true,
			want: &FixerResult{
				Success:      true,
				Summary:      "Added nil check in login handler",
				ChangedFiles: []string{"src/auth/login.go"},
				BuildPassed:  true,
				TestsPassed:  true,
			},
		},
		{
			name: "failed fix",
			input: `{
				"success": false,
				"summary": "Could not reproduce the issue",
				"changed_files": [],
				"build_passed": true,
				"tests_passed": true
			}`,
			wantOK: true,
			want: &FixerResult{
				Success:      false,
				Summary:      "Could not reproduce the issue",
				ChangedFiles: []string{},
				BuildPassed:  true,
				TestsPassed:  true,
			},
		},
		{
			name: "multiple changed files",
			input: `{
				"success": true,
				"summary": "Fixed race condition in worker pool",
				"changed_files": ["src/worker/pool.go", "src/worker/scheduler.go"],
				"build_passed": true,
				"tests_passed": false
			}`,
			wantOK: true,
			want: &FixerResult{
				Success:      true,
				Summary:      "Fixed race condition in worker pool",
				ChangedFiles: []string{"src/worker/pool.go", "src/worker/scheduler.go"},
				BuildPassed:  true,
				TestsPassed:  false,
			},
		},
		{
			name:   "malformed JSON",
			input:  `{bad data`,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ParseFixerResult([]byte(tt.input))
			if tt.wantOK && err != nil {
				t.Fatalf("ParseFixerResult: %v", err)
			}
			if !tt.wantOK && err == nil {
				t.Fatal("ParseFixerResult: expected error, got nil")
			}
			if !tt.wantOK || tt.want == nil {
				return
			}
			if res.Success != tt.want.Success {
				t.Errorf("Success = %v, want %v", res.Success, tt.want.Success)
			}
			if res.Summary != tt.want.Summary {
				t.Errorf("Summary = %q, want %q", res.Summary, tt.want.Summary)
			}
			if len(res.ChangedFiles) != len(tt.want.ChangedFiles) {
				t.Errorf("len(ChangedFiles) = %d, want %d", len(res.ChangedFiles), len(tt.want.ChangedFiles))
			} else {
				for i := range res.ChangedFiles {
					if res.ChangedFiles[i] != tt.want.ChangedFiles[i] {
						t.Errorf("ChangedFiles[%d] = %q, want %q", i, res.ChangedFiles[i], tt.want.ChangedFiles[i])
					}
				}
			}
			if res.BuildPassed != tt.want.BuildPassed {
				t.Errorf("BuildPassed = %v, want %v", res.BuildPassed, tt.want.BuildPassed)
			}
			if res.TestsPassed != tt.want.TestsPassed {
				t.Errorf("TestsPassed = %v, want %v", res.TestsPassed, tt.want.TestsPassed)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func tempAgentFile(t *testing.T, content string) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "baton-agent-*.toml")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	t.Cleanup(func() { f.Close() })

	if _, err := f.WriteString(strings.TrimSpace(content)); err != nil {
		t.Fatalf("writing agent file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing agent file: %v", err)
	}

	return f.Name()
}
