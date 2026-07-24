package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/Ducstii/Baton/internal/opencode"
	"github.com/Ducstii/Baton/internal/worktree"
)

func main() {
	repoDir := flag.String("repo", "", "path to the git repository")
	providerID := flag.String("provider", "deepseek", "OpenCode provider ID")
	modelID := flag.String("model", "deepseek-v4-flash", "model ID")
	prompt := flag.String("prompt", "", "fix prompt to send")
	baseURL := flag.String("url", "http://127.0.0.1:4096", "OpenCode serve URL")
	flag.Parse()

	if *repoDir == "" || *prompt == "" {
		fmt.Fprintf(os.Stderr, "usage: baton -repo <path> -prompt <text> [-provider <id>] [-model <id>]\n")
		os.Exit(1)
	}

	client := opencode.NewClient(*baseURL)

	health, err := client.Health()
	if err != nil {
		fmt.Fprintf(os.Stderr, "OpenCode not reachable at %s: %v\n", *baseURL, err)
		os.Exit(1)
	}
	fmt.Printf("OpenCode %s reachable\n", health.Version)

	origDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
		os.Exit(1)
	}
	if err := os.Chdir(*repoDir); err != nil {
		fmt.Fprintf(os.Stderr, "chdir %s: %v\n", *repoDir, err)
		os.Exit(1)
	}
	defer os.Chdir(origDir)

	wtBase := *repoDir + "-baton-worktrees"
	runID := fmt.Sprintf("m2-%d", time.Now().Unix())
	wt, err := worktree.Create(wtBase, runID, "fix-1")
	if err != nil {
		fmt.Fprintf(os.Stderr, "worktree create: %v\n", err)
		os.Exit(1)
	}
	defer wt.Remove()
	fmt.Printf("Worktree: %s (branch: %s)\n", wt.Path(), wt.Branch)

	session, err := client.CreateSession(wt.Path(), *modelID, *providerID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create session: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Session: %s\n", session.ID)

	fullPrompt := fmt.Sprintf(
		"You are a bug fixer working in a git worktree. Fix the following issue. "+
			"Make the minimal change needed. Do not refactor unrelated code. "+
			"After making the fix, run the build command if one exists.\n\nIssue: %s",
		*prompt,
	)
	if err := client.PromptAsync(session.ID, *providerID, *modelID, fullPrompt); err != nil {
		fmt.Fprintf(os.Stderr, "prompt_async: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Prompt dispatched, waiting for completion...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	events, errs := client.SubscribeEvents(ctx, session.ID)

	completed := false
	for !completed {
		select {
		case evt, ok := <-events:
			if !ok {
				completed = true
				break
			}
			dataStr := "<nil>"
			if evt.Data != nil {
				dataStr = string(*evt.Data)
			}
			fmt.Printf("[%s] %s", evt.Type, dataStr)
			if len(dataStr) > 0 && dataStr[len(dataStr)-1] != '\n' {
				fmt.Println()
			}
		case err, ok := <-errs:
			if ok && err != nil {
				fmt.Fprintf(os.Stderr, "event error: %v\n", err)
			}
			if !ok {
				completed = true
			}
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "timeout waiting for completion\n")
			completed = true
		}
	}

	msgs, err := client.GetMessages(session.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get messages: %v\n", err)
		os.Exit(1)
	}

	for _, msg := range msgs {
		if msg.Info.Role == "assistant" {
			for _, part := range msg.Parts {
				if part.Type == "text" {
					fmt.Printf("\n--- Response ---\n%s\n", part.Text)
				}
			}
		}
	}

	fmt.Println("\n--- Git Diff ---")
	diffCmd := exec.Command("git", "-C", wt.Path(), "diff")
	diffOut, _ := diffCmd.CombinedOutput()
	fmt.Println(string(diffOut))

	if len(diffOut) > 0 {
		fmt.Println("\nRESULT: PASS — worktree contains changes")
	} else {
		fmt.Println("\nRESULT: FAIL — no changes in worktree")
	}
}
