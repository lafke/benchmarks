package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func pct(v float64) string { return fmt.Sprintf("%.2f%%", 100*v) }

// accRow formats the numbers of a confusion matrix; the ones that need labeled safe cases are
// a dash when there are none.
func accRow(c counts, fpDefined bool) string {
	if !fpDefined {
		return fmt.Sprintf("%d | – | %d | – | %s | – | – | –", c.TP, c.FN, pct(c.Recall))
	}
	return fmt.Sprintf("%d | %d | %d | %d | %s | %s | %s | %s", c.TP, c.FP, c.FN, c.TN, pct(c.Recall), pct(c.Precision), pct(c.F1), pct(c.Youden))
}

// accHeader is the header and separator of a table whose first columns are the given labels.
func accHeader(first ...string) string {
	cols := append(append([]string{}, first...), "TP", "FP", "FN", "TN", "Recall", "Precision", "F1", "J")
	sep := make([]string, len(cols))
	for i := range sep {
		sep[i] = "---:"
		if i < len(first) {
			sep[i] = "---"
		}
	}
	return "| " + strings.Join(cols, " | ") + " |\n| " + strings.Join(sep, " | ") + " |\n"
}

// engineName picks the engine line out of the output of lafke engine status.
func engineName(status string) string {
	for _, l := range strings.Split(status, "\n") {
		if v, ok := strings.CutPrefix(l, "engine:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return strings.TrimSpace(status)
}

func summaryMarkdown(rs []result) string {
	var b strings.Builder
	first := rs[0]
	var corpora []corpusResult
	for _, r := range rs {
		corpora = append(corpora, r.Corpora...)
	}
	slices.SortFunc(corpora, func(a, c corpusResult) int { return strings.Compare(a.Name, c.Name) })

	fmt.Fprintf(&b, "## Lafke benchmark\n\n")
	fmt.Fprintf(&b, "lafke `%s`, engine `%s`, on %s/%s with %d CPUs.\n\n", first.Env.Lafke, engineName(first.Env.Engine), first.Env.OS, first.Env.Arch, first.Env.CPUs)

	fmt.Fprintf(&b, "### Detection accuracy\n\n%s", accHeader("Corpus", "Cases"))
	for _, c := range corpora {
		if a := c.Accuracy; a != nil {
			fmt.Fprintf(&b, "| `%s` | %d | %s |\n", c.Name, a.Labeled, accRow(a.counts, a.FPDefined))
		}
	}
	fmt.Fprintf(&b, "\nTP: vulnerable cases found. FP: safe cases flagged. FN: vulnerable cases missed. TN: safe cases not flagged. Recall = TP / (TP + FN), precision = TP / (TP + FP), F1 is their harmonic mean, J is recall minus the false-positive rate. A dash means the labels name no safe cases. Cases outside the scope of a corpus, such as categories lafke does not attempt, are left out.\n")

	var notes []string
	for _, c := range corpora {
		if a := c.Accuracy; a != nil {
			var n []string
			if a.AnalysisIncomplete > 0 {
				n = append(n, fmt.Sprintf("%d files not analyzed", a.AnalysisIncomplete))
			}
			if a.TimedOut > 0 {
				n = append(n, fmt.Sprintf("%d units timed out", a.TimedOut))
			}
			if len(n) > 0 {
				notes = append(notes, fmt.Sprintf("- `%s`: %s", c.Name, strings.Join(n, ", ")))
			}
		}
	}
	if len(notes) > 0 {
		fmt.Fprintf(&b, "\nWorth a look:\n\n%s\n", strings.Join(notes, "\n"))
	}

	timed := false
	for _, c := range corpora {
		timed = timed || c.Wall != nil
	}
	if timed {
		fmt.Fprintf(&b, "\n### Speed and memory\n\n| Corpus | Median (s) | Min (s) | Stdev (s) | Peak memory (MiB) | Runs |\n| --- | ---: | ---: | ---: | ---: | ---: |\n")
		for _, c := range corpora {
			if c.Wall != nil {
				fmt.Fprintf(&b, "| `%s` | %.2f | %.2f | %.2f | %.0f | %d |\n", c.Name, c.Wall.Median, c.Wall.Min, c.Wall.Stdev, c.RSS.Median, len(c.Samples["wall_seconds"]))
			}
		}
		fmt.Fprintf(&b, "\nOnly corpora that are one tree are timed. Shared runners are noisy: compare runs of the same workflow, not other machines. Memory is the median peak resident set size.\n")
	}

	fmt.Fprintf(&b, "\n<details><summary>Accuracy by category</summary>\n")
	for _, c := range corpora {
		if a := c.Accuracy; a != nil {
			fmt.Fprintf(&b, "\n#### %s\n\n%s", c.Name, accHeader("Category"))
			for _, name := range sortedKeys(a.Categories) {
				fmt.Fprintf(&b, "| `%s` | %s |\n", name, accRow(*a.Categories[name], a.FPDefined))
			}
		}
	}
	fmt.Fprintf(&b, "\n</details>\n")

	fmt.Fprintf(&b, "\nThe corpora are synthetic or curated, and lafke is developed against suites like them, so these scores do not predict accuracy on an arbitrary application. BenchProctor is scored with CWE groups and VulnBench with lafke's own reading, so neither matches the official scores.\n")
	return b.String()
}

// writeSummary appends the Markdown summary to the GitHub Actions job summary, when
// GITHUB_STEP_SUMMARY is set.
func writeSummary(rs []result) error {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" || len(rs) == 0 || len(rs[0].Corpora) == 0 {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(summaryMarkdown(rs))
	return err
}

// mergeResults reads the results of several runs, for example one per corpus on separate
// runners, and writes one job summary. The Markdown is printed when the summary is not set.
func mergeResults(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	var rs []result
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		var r result
		if err := json.Unmarshal(data, &r); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		rs = append(rs, r)
	}
	if len(rs) == 0 {
		return fmt.Errorf("no results in %s", dir)
	}
	if os.Getenv("GITHUB_STEP_SUMMARY") == "" {
		fmt.Print(summaryMarkdown(rs))
		return nil
	}
	return writeSummary(rs)
}
