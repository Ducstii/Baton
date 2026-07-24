package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ducstii/Baton/internal/config"
	"github.com/Ducstii/Baton/internal/daemon"
	"github.com/Ducstii/Baton/internal/tui"
)

func main() {
	harnessMode := flag.Bool("harness", false, "run dev harness mode")
	daemonOnly := flag.Bool("daemon", false, "run daemon only (no TUI)")
	repoDir := flag.String("repo", "", "repository path (harness mode)")
	prompt := flag.String("prompt", "", "fix prompt (harness mode)")
	providerID := flag.String("provider", "deepseek", "provider ID")
	modelID := flag.String("model", "deepseek-v4-flash", "model ID")
	flag.Parse()

	if *harnessMode {
		runHarness(*repoDir, *prompt, *providerID, *modelID)
		return
	}

	cfgPath := config.ConfigPath()
	cfg, err := config.Parse(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	if cfg == nil {
		cfg = config.DefaultConfig()
		cfg.Token = generateToken()
		if err := cfg.Save(cfgPath); err != nil {
			fmt.Fprintf(os.Stderr, "save config: %v\n", err)
			os.Exit(1)
		}
	}

	if cfg.Token == "" {
		cfg.Token = generateToken()
		if err := cfg.Save(cfgPath); err != nil {
			fmt.Fprintf(os.Stderr, "save config: %v\n", err)
			os.Exit(1)
		}
	}

	daemonURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.DaemonPort)

	var daemonProc *daemon.Daemon
	if !isDaemonRunning(daemonURL) {
		daemonProc = daemon.New(cfg)
		go func() {
			if err := daemonProc.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "daemon: %v\n", err)
				os.Exit(1)
			}
		}()
		waitForDaemon(daemonURL, 3*time.Second)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if *daemonOnly {
		fmt.Printf("Daemon running at %s\n", daemonURL)
		<-sigCh
		if daemonProc != nil {
			daemonProc.Stop()
		}
		return
	}

	go func() {
		<-sigCh
		if daemonProc != nil {
			daemonProc.Stop()
		}
		os.Exit(0)
	}()

	if err := tui.Start(daemonURL, cfg.Token); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}
}

func waitForDaemon(url string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func isDaemonRunning(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	return "baton-" + hex.EncodeToString(b)
}
