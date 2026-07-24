package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
)

// Project represents a project tracked by the Baton daemon.
type Project struct {
	Name        string `json:"name"`
	BuildSystem string `json:"build_system"`
	Path        string `json:"path"`
}

// ProjectNavigator holds state for the project selection screen.
type ProjectNavigator struct {
	projects  []Project
	cursor    int
	textInput textinput.Model
	loading   bool
	err       error
}

// NewProjectNavigator creates a ProjectNavigator with a ready text input.
func NewProjectNavigator() ProjectNavigator {
	ti := textinput.New()
	ti.Placeholder = "/path/to/project"
	ti.Focus()
	ti.Width = 60
	return ProjectNavigator{
		textInput: ti,
	}
}
