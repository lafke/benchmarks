//go:build !windows

package main

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

// peakRSSMiB returns the peak resident set size of the process and the descendants it waited
// for. Maxrss is in bytes on macOS and in kibibytes elsewhere.
func peakRSSMiB(ps *os.ProcessState) float64 {
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0
	}
	if runtime.GOOS == "darwin" {
		return float64(ru.Maxrss) / (1024 * 1024)
	}
	return float64(ru.Maxrss) / 1024
}

// killGroupOnCancel runs cmd in its own process group and kills the group when its context ends.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
}
