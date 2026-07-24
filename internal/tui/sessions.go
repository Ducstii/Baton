package tui

import (
	"fmt"
	"strings"
	"time"
)

// Session is a display-level session card derived from daemon data.
type Session struct {
	ID        string
	Name      string
	Status    string
	Provider  string
	Model     string
	Directory string
	RunID     string
	UpdatedAt time.Time
}

// SessionGroup is a named group of sessions.
type SessionGroup struct {
	Title    string
	Status   string
	Sessions []Session
}

// mapStatus converts a daemon status to an AMUX display status.
func mapStatus(s string) string {
	switch s {
	case "diagnosing", "in_progress", "running":
		return "working"
	case "awaiting_checkpoint", "pending":
		return "waiting"
	case "completed", "idle", "":
		return "idle"
	case "failed", "stopped", "error", "cancelled":
		return "stopped"
	default:
		return "idle"
	}
}

// extractSessions flattens daemon runs into display sessions.
func extractSessions(runs []Run) []Session {
	var sessions []Session
	for _, r := range runs {
		if len(r.Sessions) == 0 {
			name := r.InputSource
			if name == "" {
				name = r.ID
				if len(name) > 8 {
					name = name[:8]
				}
			}
			sessions = append(sessions, Session{
				ID:        r.ID,
				Name:      name,
				Status:    mapStatus(r.Status),
				Directory: r.ProjectPath,
				RunID:     r.ID,
				UpdatedAt: r.UpdatedAt,
			})
			continue
		}
		for _, s := range r.Sessions {
			name := r.InputSource
			if name == "" {
				name = s.ID
				if len(name) > 8 {
					name = name[:8]
				}
			}
			sessions = append(sessions, Session{
				ID:        s.ID,
				Name:      name,
				Status:    mapStatus(s.Status),
				Provider:  s.AgentID,
				Model:     s.Model,
				Directory: r.ProjectPath,
				RunID:     r.ID,
				UpdatedAt: r.UpdatedAt,
			})
		}
	}
	return sessions
}

// groupSessions groups sessions by status in display order.
// Order: working, waiting, idle, stopped.
func groupSessions(sessions []Session) []SessionGroup {
	order := []string{"working", "waiting", "idle", "stopped"}
	titles := map[string]string{
		"working": "WORKING",
		"waiting": "NEEDS INPUT",
		"idle":    "IDLE",
		"stopped": "STOPPED",
	}
	groups := make(map[string][]Session)
	for _, s := range sessions {
		groups[s.Status] = append(groups[s.Status], s)
	}
	var result []SessionGroup
	for _, key := range order {
		if ss, ok := groups[key]; ok && len(ss) > 0 {
			result = append(result, SessionGroup{
				Title:    titles[key],
				Status:   key,
				Sessions: ss,
			})
		}
	}
	return result
}

// timeAgo returns a human-readable relative time string.
func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		return t.Format("Jan 2")
	}
}

// shortPath shortens an absolute path for display.
func shortPath(p string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "/Users/") {
		parts := strings.SplitN(p, "/", 4)
		if len(parts) >= 4 {
			return "~/" + parts[3]
		}
	}
	if strings.HasPrefix(p, "/home/") {
		parts := strings.SplitN(p, "/", 4)
		if len(parts) >= 4 {
			return "~/" + parts[3]
		}
	}
	if len(p) > 55 {
		return "..." + p[len(p)-52:]
	}
	return p
}
