package main

import (
	"math"
	"slices"
)

type stats struct {
	Min    float64 `json:"min"`
	Median float64 `json:"median"`
	Mean   float64 `json:"mean"`
	Stdev  float64 `json:"stdev"`
}

func summarize(v []float64) stats {
	s := slices.Sorted(slices.Values(v))
	n := len(s)
	var sum float64
	for _, x := range s {
		sum += x
	}
	mean := sum / float64(n)
	median := s[n/2]
	if n%2 == 0 {
		median = (s[n/2-1] + s[n/2]) / 2
	}
	var stdev float64
	if n > 1 {
		var sq float64
		for _, x := range s {
			sq += (x - mean) * (x - mean)
		}
		stdev = math.Sqrt(sq / float64(n-1))
	}
	return stats{Min: s[0], Median: median, Mean: mean, Stdev: stdev}
}
