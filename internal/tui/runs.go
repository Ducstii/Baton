package tui

import "time"

// Run matches the daemon's JSON for a run.
type Run struct {
	ID               string            `json:"id"`
	ProjectPath      string            `json:"project_path"`
	InputType        string            `json:"input_type"`
	InputSource      string            `json:"input_source"`
	TargetAgentCount int               `json:"target_agent_count"`
	ModelMapping     map[string]string `json:"model_mapping"`
	Status           string            `json:"status"`
	WorkUnits        WorkUnitStats     `json:"work_units"`
	Sessions         []SessionInfo     `json:"sessions"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// SessionInfo matches the daemon's session info JSON.
type SessionInfo struct {
	ID      string `json:"id"`
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
	Model   string `json:"model"`
}

// WorkUnitStats matches the daemon's work unit stats JSON.
type WorkUnitStats struct {
	Total      int `json:"total"`
	Done       int `json:"done"`
	InProgress int `json:"in_progress"`
	Blocked    int `json:"blocked"`
	Failed     int `json:"failed"`
}
