package agents

import (
	"testing"
)

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
			res, err := parseDiagnosticResult([]byte(tt.input))
			if tt.wantOK && err != nil {
				t.Fatalf("parseDiagnosticResult: %v", err)
			}
			if !tt.wantOK && err == nil {
				t.Fatal("parseDiagnosticResult: expected error, got nil")
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
			res, err := parseFixerResult([]byte(tt.input))
			if tt.wantOK && err != nil {
				t.Fatalf("parseFixerResult: %v", err)
			}
			if !tt.wantOK && err == nil {
				t.Fatal("parseFixerResult: expected error, got nil")
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
