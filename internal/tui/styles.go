package tui

import "github.com/charmbracelet/lipgloss"

var (
	Bg     = lipgloss.Color("#0d1117")
	Card   = lipgloss.Color("#161b22")
	Border = lipgloss.Color("#30363d")
	Text   = lipgloss.Color("#e6edf3")
	Dim    = lipgloss.Color("#8b949e")
	Accent = lipgloss.Color("#58a6ff")
	Green  = lipgloss.Color("#3fb950")
	Red    = lipgloss.Color("#f85149")
	Yellow = lipgloss.Color("#d29922")
	Cyan   = lipgloss.Color("#39d2c0")
	Purple = lipgloss.Color("#8957e5")
)

var (
	statusWorkingBg = lipgloss.Color("#0f2d1a")
	statusWaitingBg = lipgloss.Color("#2d220f")
	statusIdleBg    = lipgloss.Color("#161b22")
	statusStoppedBg = lipgloss.Color("#111111")
)

// StatusWorking renders a green-tinted WORKING badge.
func StatusWorking() string {
	return lipgloss.NewStyle().
		Background(statusWorkingBg).
		Foreground(Green).
		Bold(true).
		Padding(0, 1).
		Render("WORKING")
}

// StatusWaiting renders a yellow-tinted NEEDS INPUT badge.
func StatusWaiting() string {
	return lipgloss.NewStyle().
		Background(statusWaitingBg).
		Foreground(Yellow).
		Bold(true).
		Padding(0, 1).
		Render("NEEDS INPUT")
}

// StatusIdle renders a dim IDLE badge.
func StatusIdle() string {
	return lipgloss.NewStyle().
		Background(statusIdleBg).
		Foreground(Dim).
		Padding(0, 1).
		Render("IDLE")
}

// StatusStopped renders a dim, bordered STOPPED badge.
func StatusStopped() string {
	return lipgloss.NewStyle().
		Background(statusStoppedBg).
		Foreground(Dim).
		Border(lipgloss.NormalBorder()).
		BorderForeground(Border).
		Padding(0, 1).
		Render("STOPPED")
}

// StatusBadge returns the appropriate status badge for a status string.
func StatusBadge(status string) string {
	switch status {
	case "working":
		return StatusWorking()
	case "waiting":
		return StatusWaiting()
	case "idle":
		return StatusIdle()
	case "stopped":
		return StatusStopped()
	default:
		return StatusIdle()
	}
}

// RoleBadge returns a colored role/provider badge.
func RoleBadge(role string) string {
	switch role {
	case "brain", "claude", "anthropic", "sonnet":
		return lipgloss.NewStyle().
			Background(lipgloss.Color("#2a1a4a")).
			Foreground(Purple).
			Bold(true).
			Padding(0, 1).
			Render(role)
	case "diagnostic":
		return lipgloss.NewStyle().
			Background(statusWaitingBg).
			Foreground(Yellow).
			Bold(true).
			Padding(0, 1).
			Render(role)
	case "fixer":
		return lipgloss.NewStyle().
			Background(lipgloss.Color("#3a1a1a")).
			Foreground(Red).
			Bold(true).
			Padding(0, 1).
			Render(role)
	case "reviewer":
		return lipgloss.NewStyle().
			Background(lipgloss.Color("#1a2a1a")).
			Foreground(Green).
			Bold(true).
			Padding(0, 1).
			Render(role)
	default:
		if role == "" {
			return ""
		}
		return lipgloss.NewStyle().
			Foreground(Cyan).
			Padding(0, 1).
			Render(role)
	}
}

// View styles.
var (
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Accent).
			Padding(0, 1)

	TabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(Dim)

	TabActiveStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(Text).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(Accent)

	CardStyle = lipgloss.NewStyle().
			Background(Card).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Border).
			Padding(0, 1)

	CardSelectedStyle = lipgloss.NewStyle().
			Background(Card).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Accent).
			Padding(0, 1)

	GroupHeaderStyle = lipgloss.NewStyle().
			Foreground(Dim).
			Bold(true).
			Padding(0, 1)

	MenuItemStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(Text)

	MenuItemDangerStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(Red)

	ViewportStyle = lipgloss.NewStyle().
			Padding(0, 1)

	HelpBarStyle = lipgloss.NewStyle().
			Foreground(Dim).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(Border).
			Padding(0, 1)
)
