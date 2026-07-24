package tui

// Run represents a Baton run within a project.
type Run struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	WorkUnits int    `json:"work_units"`
}

// RunSwitcher holds state for the run selection screen.
type RunSwitcher struct {
	project     Project
	runs        []Run
	cursor      int
	loading     bool
	err         error
	newRunMode  bool
	newRunInput string
}

// NewRunSwitcher creates a RunSwitcher for the given project.
func NewRunSwitcher(p Project) RunSwitcher {
	return RunSwitcher{project: p}
}
