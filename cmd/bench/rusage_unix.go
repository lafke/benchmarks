//go:build !windows

package main

import (
	"os"
	"runtime"
	"syscall"
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
