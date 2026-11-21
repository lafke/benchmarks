package main

import "os"

// peakRSSMiB is not measured on Windows and reports 0.
func peakRSSMiB(*os.ProcessState) float64 { return 0 }
