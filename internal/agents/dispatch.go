package agents

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DiagnosticIssue is one issue found by the diagnostic agent.
type DiagnosticIssue struct {
	ID              string   `json:"id"`
	Description     string   `json:"description"`
	Confidence      string   `json:"confidence"`
	Group           string   `json:"group"`
	SuspectedFiles  []string `json:"suspected_files"`
	DependencyEdges []string `json:"dependency_edges"`
}

// WorkUnitGroup groups related issues into a single fixer task.
type WorkUnitGroup struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Issues      []string `json:"issues"`
}

// DiagnosticResult is the full output from a diagnostic run.
type DiagnosticResult struct {
	Issues    []DiagnosticIssue `json:"issues"`
	WorkUnits []WorkUnitGroup   `json:"work_units"`
}

// FixerResult is the output from a fixer session.
type FixerResult struct {
	Success      bool     `json:"success"`
	Summary      string   `json:"summary"`
	ChangedFiles []string `json:"changed_files"`
	BuildPassed  bool     `json:"build_passed"`
	TestsPassed  bool     `json:"tests_passed"`
}

// extractJSON strips markdown fences and leading/trailing non-JSON content
// from s, returning the first JSON object or array it finds.
func extractJSON(s string) string {
	// Strip markdown code fences.
	s = strings.TrimSpace(s)

	// Remove ```json ... ``` block.
	if idx := strings.Index(s, "```json"); idx >= 0 {
		end := strings.LastIndex(s, "```")
		if end > idx {
			s = s[idx+7 : end]
		}
	} else if idx := strings.Index(s, "```"); idx >= 0 {
		end := strings.LastIndex(s, "```")
		if end > idx {
			s = s[idx+3 : end]
		}
	}

	s = strings.TrimSpace(s)

	// Find the outermost { or [.
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return ""
	}
	s = s[start:]

	// Find matching closing bracket -- crude but sufficient for one level.
	var depth int
	var inStr bool
	for i, r := range s {
		if inStr {
			if r == '\\' {
				continue // skip escaped char
			}
			if r == '"' {
				inStr = false
			}
			continue
		}
		switch r {
		case '"':
			inStr = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}

	return s
}

// ParseDiagnosticResult unmarshals raw JSON into a DiagnosticResult.
func ParseDiagnosticResult(data []byte) (*DiagnosticResult, error) {
	var res DiagnosticResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("parse diagnostic result: %w", err)
	}
	// Basic validation.
	if len(res.Issues) == 0 {
		return nil, fmt.Errorf("diagnostic result contains no issues")
	}
	for _, wu := range res.WorkUnits {
		if len(wu.Issues) == 0 {
			return nil, fmt.Errorf("work unit %q contains no issues", wu.ID)
		}
	}
	return &res, nil
}

// ParseFixerResult unmarshals raw JSON into a FixerResult.
func ParseFixerResult(data []byte) (*FixerResult, error) {
	var res FixerResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("parse fixer result: %w", err)
	}
	return &res, nil
}
