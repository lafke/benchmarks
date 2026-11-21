package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}

// retry runs f up to three times, for network operations.
func retry(f func() error) error {
	var err error
	for i := 0; i < 3; i++ {
		if err = f(); err == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 2 * time.Second)
	}
	return err
}

// fetchRepo checks repo out at rev in dest and verifies the result. With sparse set, only those
// directories (and the files at the root) are checked out, and file contents are fetched on
// demand. A checkout that is already at rev is reused without touching the network.
func (e *env) fetchRepo(dest, repo, rev string, sparse []string) error {
	e.sem <- struct{}{}
	defer func() { <-e.sem }()
	if head, err := git(dest, "rev-parse", "HEAD"); err == nil && head == rev {
		if len(sparse) > 0 {
			_, err = git(dest, append([]string{"sparse-checkout", "set"}, sparse...)...)
		}
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	if _, err := git(dest, "init", "-q"); err != nil {
		return err
	}
	git(dest, "remote", "remove", "origin")
	if _, err := git(dest, "remote", "add", "origin", repo); err != nil {
		return err
	}
	fetch := []string{"fetch", "-q", "--depth", "1"}
	if len(sparse) > 0 {
		git(dest, "config", "remote.origin.promisor", "true")
		git(dest, "config", "remote.origin.partialclonefilter", "blob:none")
		fetch = append(fetch, "--filter=blob:none")
		if _, err := git(dest, append([]string{"sparse-checkout", "set"}, sparse...)...); err != nil {
			return err
		}
	}
	fetch = append(fetch, "origin", rev)
	if err := retry(func() error { _, err := git(dest, fetch...); return err }); err != nil {
		return err
	}
	if _, err := git(dest, "checkout", "-q", "--detach", "FETCH_HEAD"); err != nil {
		return err
	}
	head, err := git(dest, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if head != rev {
		return fmt.Errorf("expected %s, got %s", rev, head)
	}
	return nil
}
