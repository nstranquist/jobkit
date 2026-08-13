// Command claimguard-bridge dogfoods the optional external ClaimGuard binary
// with one passing and one fail-closed fixture.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	checker, err := findChecker()
	if errors.Is(err, exec.ErrNotFound) {
		fmt.Println("claimguard-bridge: agent-ops and legacy claimguard are not installed; skip")
		return
	}
	if err != nil {
		fatal(err)
	}
	root, err := os.Getwd()
	if err != nil {
		fatal(err)
	}
	claims := filepath.Join(root, "tools", "claimguard-bridge", "testdata", "claims.yaml")
	good := filepath.Join(root, "tools", "claimguard-bridge", "testdata", "good.md")
	bad := filepath.Join(root, "tools", "claimguard-bridge", "testdata", "bad.md")
	if err := run(checker, claims, good, true); err != nil {
		fatal(fmt.Errorf("passing fixture: %w", err))
	}
	if err := run(checker, claims, bad, false); err != nil {
		fatal(fmt.Errorf("fail-closed fixture: %w", err))
	}
	if candidate := strings.TrimSpace(os.Getenv("JOBKIT_CLAIMGUARD_FILE")); candidate != "" {
		stateRoot := strings.TrimSpace(os.Getenv("JOBKIT_HOME"))
		if stateRoot == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				fatal(err)
			}
			stateRoot = filepath.Join(home, ".jobkit")
		}
		if err := run(checker, filepath.Join(stateRoot, "claims.yaml"), candidate, true); err != nil {
			fatal(fmt.Errorf("requested draft: %w", err))
		}
	}
	fmt.Printf("claimguard-bridge: OK with %s (passing and fail-closed fixtures)\n", checker.name)
}

type claimChecker struct {
	name   string
	binary string
	prefix []string
}

func findChecker() (claimChecker, error) {
	if binary, err := exec.LookPath("agent-ops"); err == nil {
		return claimChecker{name: "agent-ops", binary: binary, prefix: []string{"claims", "check"}}, nil
	}
	binary, err := exec.LookPath("claimguard")
	if err != nil {
		return claimChecker{}, err
	}
	return claimChecker{name: "legacy claimguard", binary: binary, prefix: []string{"check"}}, nil
}

func run(checker claimChecker, claims, file string, wantSuccess bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := append(append([]string(nil), checker.prefix...), "--claims", claims, "--file", file)
	command := exec.CommandContext(ctx, checker.binary, args...)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("timed out after 30s")
	}
	if wantSuccess && err != nil {
		return fmt.Errorf("unexpected rejection: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if !wantSuccess && err == nil {
		return fmt.Errorf("unsupported quantified claim was accepted")
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "claimguard-bridge:", err)
	os.Exit(1)
}
