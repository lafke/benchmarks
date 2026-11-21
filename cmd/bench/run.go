package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// measure runs lafke once and returns its wall time in seconds, peak memory in MiB and output.
// Exit code 1 means findings and counts as success. Exit code 2 is an operational error, but lafke
// also uses it for a complete report that says some files could not be analyzed; that is accepted
// when incomplete is set, and the report is checked when it is parsed.
func measure(lafke string, args []string, capture, incomplete bool, timeout time.Duration) (wall, rss float64, out []byte, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, lafke, args...)
	// lafke starts the engine, so on a timeout the whole process group is killed.
	killGroupOnCancel(cmd)
	var buf bytes.Buffer
	if capture {
		cmd.Stdout = &buf
	}
	start := time.Now()
	err = cmd.Run()
	wall = time.Since(start).Seconds()
	if ctx.Err() != nil {
		return wall, 0, nil, errTimeout
	}
	if ee, ok := err.(*exec.ExitError); ok && (ee.ExitCode() == 1 || (incomplete && ee.ExitCode() == 2)) {
		err = nil
	}
	if err != nil {
		return 0, 0, nil, fmt.Errorf("lafke %s: %w", strings.Join(args, " "), err)
	}
	return wall, peakRSSMiB(cmd.ProcessState), buf.Bytes(), nil
}

// errTimeout is returned by measure when lafke did not finish in time.
var errTimeout = errors.New("timed out")

func output(name string, args ...string) (string, error) {
	b, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(b)), nil
}

func lafkeArgs(c *corpus, dir string) []string {
	return append(append([]string{}, c.Command...), dir, "--format", "json", "--fail-on", "critical")
}
