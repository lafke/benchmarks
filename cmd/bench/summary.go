package main

import (
	"fmt"
	"os"
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
	fmt.Fprintf(&b, "\nShared runners vary between runs and are not comparable with other machines. Memory is the median peak resident set size. The JSON with every sample is attached as an artifact.\n")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(b.String())
	return err
}
