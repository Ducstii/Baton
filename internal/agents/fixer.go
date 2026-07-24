package agents

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Ducstii/Baton/internal/opencode"
)

// FixerResult is the output from a fixer session.
type FixerResult struct {
	Success     bool     `json:"success"`
	Summary     string   `json:"summary"`
	ChangedFiles []string `json:"changed_files"`
	BuildPassed bool     `json:"build_passed"`
	TestsPassed bool     `json:"tests_passed"`
}

// parseFixerResult unmarshals raw JSON into a FixerResult.
func parseFixerResult(data []byte) (*FixerResult, error) {
	var res FixerResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("parse fixer result: %w", err)
	}
	return &res, nil
}

// RunFixer creates an OpenCode session in the given worktree, sends a prompt
// describing the issues to fix, waits for completion, and returns the result.
func RunFixer(client *opencode.Client, worktreePath string, workUnit WorkUnitGroup, providerID, modelID string) (*FixerResult, error) {
	session, err := client.CreateSession(worktreePath, modelID, providerID)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	// Build issue detail lines.
	var issueLines []string
	for _, issueID := range workUnit.Issues {
		issueLines = append(issueLines, "- "+issueID)
	}

	prompt := fmt.Sprintf(`You are a code fixer. Fix the following issues in the codebase with minimal, surgical changes.

Work Unit: %s
Description: %s
Issues:
%s

For each issue:
- Make minimal, surgical changes — touch only what is necessary.
- Run the build command after making changes if your environment supports it.
- Run tests after making changes if your environment supports it.

After fixing, report the results as valid JSON with this exact structure and nothing else:
{"success": true, "summary": "Fixed null pointer in UserService.login()", "changed_files": ["src/user/service.py"], "build_passed": true, "tests_passed": true}

Respond ONLY with the JSON, no other text.`, workUnit.ID, workUnit.Description, strings.Join(issueLines, "\n"))

	if err := client.PromptAsync(session.ID, providerID, modelID, prompt); err != nil {
		return nil, fmt.Errorf("send prompt: %w", err)
	}

	text, err := pollComplete(client, session.ID, 10*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("wait for completion: %w", err)
	}

	raw := extractJSON(text)
	if raw == "" {
		return nil, fmt.Errorf("no JSON found in assistant response")
	}

	return parseFixerResult([]byte(raw))
}
