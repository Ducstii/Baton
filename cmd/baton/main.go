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

	"github.com/Ducstii/Baton/internal/config"
	"github.com/Ducstii/Baton/internal/daemon"
	"github.com/Ducstii/Baton/internal/tui"
)

func main() {
	harnessMode := flag.Bool("harness", false, "run dev harness mode")
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
	if os.IsNotExist(err) {
		cfg = config.DefaultConfig()
		cfg.Token = generateToken()
		if err := cfg.Save(cfgPath); err != nil {
			fmt.Fprintf(os.Stderr, "save config: %v\n", err)
			os.Exit(1)
		}
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	daemonURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.DaemonPort)

	if !isDaemonRunning(daemonURL) {
		d := daemon.New(cfg)
		go func() {
			if err := d.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "daemon: %v\n", err)
			}
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		os.Exit(0)
	}()

	if err := tui.Start(daemonURL, cfg.Token); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}
}

func isDaemonRunning(url string) bool {
	resp, err := http.Get(url + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "baton-" + hex.EncodeToString(b)
}
