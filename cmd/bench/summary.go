package main

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
)

// writeSummary appends a Markdown table of the result to the GitHub Actions job summary,
// when GITHUB_STEP_SUMMARY is set.
func writeSummary(res result) error {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Lafke benchmark\n\n")
	fmt.Fprintf(&b, "%s/%s, %d CPUs, %s, %d measured runs per corpus.\n\n", res.Env.OS, res.Env.Arch, res.Env.CPUs, res.Env.Go, res.Runs)
	fmt.Fprintf(&b, "lafke `%s`\n\n```\n%s\n```\n\n", res.Env.Lafke, res.Env.Engine)
	fmt.Fprintf(&b, "| Corpus | Revision | Median (s) | Min (s) | Stdev (s) | Peak memory (MiB) |\n")
	fmt.Fprintf(&b, "| --- | --- | ---: | ---: | ---: | ---: |\n")
	for _, c := range res.Corpora {
		fmt.Fprintf(&b, "| `%s` | `%.12s` | %.2f | %.2f | %.2f | %.0f |\n", c.Name, c.Rev, c.Wall.Median, c.Wall.Min, c.Wall.Stdev, c.RSS.Median)
	}
	var scored bool
	for _, c := range res.Corpora {
		if c.Accuracy != nil {
			scored = true
		}
	}
	if scored {
		fmt.Fprintf(&b, "\n### Detection accuracy\n\n")
		fmt.Fprintf(&b, "| Corpus | TP | FP | FN | TN | Recall | Precision | F1 |\n")
		fmt.Fprintf(&b, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for _, c := range res.Corpora {
			if a := c.Accuracy; a != nil {
				fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d | %.2f%% | %.2f%% | %.2f%% |\n", c.Name, a.TP, a.FP, a.FN, a.TN, 100*a.Recall, 100*a.Precision, 100*a.F1)
			}
		}
		fmt.Fprintf(&b, "\nTP: vulnerable cases found. FP: safe cases flagged. FN: vulnerable cases missed. TN: safe cases not flagged. Recall = TP / (TP + FN), precision = TP / (TP + FP), F1 is their harmonic mean.\n")
		for _, c := range res.Corpora {
			a := c.Accuracy
			if a == nil {
				continue
			}
			fmt.Fprintf(&b, "\n<details><summary>%s by category</summary>\n\n", c.Name)
			fmt.Fprintf(&b, "| Category | TP | FP | FN | TN | Recall | Precision | F1 |\n")
			fmt.Fprintf(&b, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
			for _, name := range slices.Sorted(maps.Keys(a.Categories)) {
				k := a.Categories[name]
				fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d | %.2f%% | %.2f%% | %.2f%% |\n", name, k.TP, k.FP, k.FN, k.TN, 100*k.Recall, 100*k.Precision, 100*k.F1)
			}
			fmt.Fprintf(&b, "\n%d labeled cases, %d analysis-incomplete findings.\n\n</details>\n", a.Labeled, a.AnalysisIncomplete)
		}
		fmt.Fprintf(&b, "\nThe suites are synthetic. These scores do not predict accuracy on an arbitrary application, and they say nothing about a tool that was not run on the same cases.\n")
	}
	fmt.Fprintf(&b, "\nShared runners vary between runs and are not comparable with other machines. Memory is the median peak resident set size. The JSON with every sample is attached as an artifact.\n")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(b.String())
	return err
}
