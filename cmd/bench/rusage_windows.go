package main

import (
	"os"
	"os/exec"
)

// peakRSSMiB is not measured on Windows and reports 0.
func peakRSSMiB(*os.ProcessState) float64 { return 0 }

// killGroupOnCancel kills the process when its context ends; lafke's child is left to exit.
func killGroupOnCancel(cmd *exec.Cmd) {}
