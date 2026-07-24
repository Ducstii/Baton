package config

import (
	"os"
	"testing"
)

func TestParse(t *testing.T) {
	content := `# Baton configuration
daemon_port = 9090
worktree_base_path = "/tmp/baton-worktrees"

[models]
brain = "opus"
diagnostic = "sonnet"
fixer = "haiku"
reviewer = "sonnet"
`
	path := tempConfig(t, content)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if cfg.DaemonPort != 9090 {
		t.Errorf("DaemonPort = %d, want 9090", cfg.DaemonPort)
	}
	if cfg.WorktreeBasePath != "/tmp/baton-worktrees" {
		t.Errorf("WorktreeBasePath = %q, want %q", cfg.WorktreeBasePath, "/tmp/baton-worktrees")
	}
	if cfg.Token != "" {
		t.Errorf("Token = %q, want empty", cfg.Token)
	}
	if cfg.Models["brain"] != "opus" {
		t.Errorf("Models[brain] = %q, want opus", cfg.Models["brain"])
	}
	if cfg.Models["fixer"] != "haiku" {
		t.Errorf("Models[fixer] = %q, want haiku", cfg.Models["fixer"])
	}
	if cfg.Models["reviewer"] != "sonnet" {
		t.Errorf("Models[reviewer] = %q, want sonnet", cfg.Models["reviewer"])
	}
}

func TestParse_Token(t *testing.T) {
	content := `token = "auto-generated-abc123"
`
	path := tempConfig(t, content)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if cfg.Token != "auto-generated-abc123" {
		t.Errorf("Token = %q, want %q", cfg.Token, "auto-generated-abc123")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DaemonPort != 8080 {
		t.Errorf("DaemonPort = %d, want 8080", cfg.DaemonPort)
	}
	if cfg.Token != "" {
		t.Errorf("Token = %q, want empty", cfg.Token)
	}
	if len(cfg.Models) != 4 {
		t.Errorf("len(Models) = %d, want 4", len(cfg.Models))
	}
	for _, role := range []string{"brain", "diagnostic", "fixer", "reviewer"} {
		if _, ok := cfg.Models[role]; !ok {
			t.Errorf("Models missing key %q", role)
		}
	}
}

func tempConfig(t *testing.T, content string) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "baton-config-*.toml")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	t.Cleanup(func() { f.Close() })

	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing config: %v", err)
	}

	return f.Name()
}
