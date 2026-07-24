package tui

// Session represents a single agent session within a run.
type Session struct {
	ID       string `json:"id"`
	Role     string `json:"role"`
	Status   string `json:"status"`
	Duration string `json:"duration"`
}

// SessionList holds state for the session detail screen.
type SessionList struct {
	run      Run
	sessions []Session
	cursor   int
	loading  bool
	err      error
}

// NewSessionList creates a SessionList for the given run.
func NewSessionList(r Run) SessionList {
	return SessionList{run: r}
}
