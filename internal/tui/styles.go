package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Status badge styles.
	StyleBadgeDiagnosing         = lipgloss.NewStyle().Background(lipgloss.Color("#FFB800")).Foreground(lipgloss.Color("#000")).Padding(0, 1)
	StyleBadgeAwaitingCheckpoint = lipgloss.NewStyle().Background(lipgloss.Color("#0055FF")).Foreground(lipgloss.Color("#FFF")).Padding(0, 1)
	StyleBadgeInProgress         = lipgloss.NewStyle().Background(lipgloss.Color("#00CC66")).Foreground(lipgloss.Color("#000")).Padding(0, 1)
	StyleBadgeCompleted          = lipgloss.NewStyle().Background(lipgloss.Color("#666666")).Foreground(lipgloss.Color("#FFF")).Padding(0, 1)
	StyleBadgeFailed             = lipgloss.NewStyle().Background(lipgloss.Color("#FF0000")).Foreground(lipgloss.Color("#FFF")).Padding(0, 1)

	// Role badge styles.
	StyleRoleBrain      = lipgloss.NewStyle().Background(lipgloss.Color("#7C3AED")).Foreground(lipgloss.Color("#FFF")).Padding(0, 1)
	StyleRoleDiagnostic = lipgloss.NewStyle().Background(lipgloss.Color("#F59E0B")).Foreground(lipgloss.Color("#000")).Padding(0, 1)
	StyleRoleFixer      = lipgloss.NewStyle().Background(lipgloss.Color("#EF4444")).Foreground(lipgloss.Color("#FFF")).Padding(0, 1)
	StyleRoleReviewer   = lipgloss.NewStyle().Background(lipgloss.Color("#10B981")).Foreground(lipgloss.Color("#000")).Padding(0, 1)

	// Layout styles.
	StyleTitle    = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("#7C3AED")).Foreground(lipgloss.Color("#FFF")).Padding(0, 2)
	StyleNormal   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF"))
	StyleDimmed   = lipgloss.NewStyle().Foreground(lipgloss.Color("#666"))
	StyleSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true)
	StyleHelp     = lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
	StyleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
)

// StatusBadge renders a colored badge for the given run/session status.
func StatusBadge(status string) string {
	switch status {
	case "diagnosing":
		return StyleBadgeDiagnosing.Render("DIAGNOSING")
	case "awaiting_checkpoint":
		return StyleBadgeAwaitingCheckpoint.Render("AWAITING CHKPT")
	case "in_progress":
		return StyleBadgeInProgress.Render("IN PROGRESS")
	case "completed":
		return StyleBadgeCompleted.Render("COMPLETED")
	case "failed":
		return StyleBadgeFailed.Render("FAILED")
	default:
		return StyleDimmed.Render(status)
	}
}

// RoleBadge renders a colored badge for the given session role.
func RoleBadge(role string) string {
	switch role {
	case "brain":
		return StyleRoleBrain.Render("BRAIN")
	case "diagnostic":
		return StyleRoleDiagnostic.Render("DIAG")
	case "fixer":
		return StyleRoleFixer.Render("FIXER")
	case "reviewer":
		return StyleRoleReviewer.Render("REVIEWER")
	default:
		return StyleDimmed.Render(role)
	}
}
