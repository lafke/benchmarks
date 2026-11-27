package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const latestRef = "latest"

// stored is a result file in the output directory. Numbers start at 1 with the oldest result, so
// a number keeps naming the same result when newer ones are added.
type stored struct {
	Num   int
	Label string // how the comparison names it: "#12", or the directory of a run
	Path  string
	result
}

// loadStored reads the results in dir, oldest first by their recorded date, then file name.
func loadStored(dir string) ([]stored, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var all []stored
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		s := stored{Path: f}
		if err := json.Unmarshal(data, &s.result); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		all = append(all, s)
	}
	slices.SortFunc(all, func(a, b stored) int {
		if c := strings.Compare(a.Date, b.Date); c != 0 {
			return c
		}
		return strings.Compare(a.Path, b.Path)
	})
	for i := range all {
		all[i].Num = i + 1
		all[i].Label = fmt.Sprintf("#%d", i+1)
	}
	return all, nil
}

func (s stored) names() string {
	var n []string
	for _, c := range s.Corpora {
		n = append(n, c.Name)
	}
	return strings.Join(n, ", ")
}

// historyText lists the stored results with the numbers -base and -head take.
func historyText(all []stored) string {
	var b strings.Builder
	for _, s := range all {
		fmt.Fprintf(&b, "%3d  %s  %s  %s\n", s.Num, s.Date, filepath.Base(s.Path), s.names())
	}
	return b.String()
}

// pick resolves a reference, a number or "latest", to a stored result.
func pick(all []stored, ref string) (stored, error) {
	if ref == latestRef {
		return all[len(all)-1], nil
	}
	n, err := strconv.Atoi(ref)
	if err != nil {
		return stored{}, fmt.Errorf("%q is not a result number or %q", ref, latestRef)
	}
	if n < 1 || n > len(all) {
		return stored{}, fmt.Errorf("no result %d (have 1 to %d, see -history)", n, len(all))
	}
	return all[n-1], nil
}

// selectPair picks the two results to compare. At least one side must be given; the other one
// defaults to the latest result. Both sides being the latest result is an error.
func selectPair(all []stored, base, head string) (stored, stored, error) {
	if base == "" && head == "" {
		return stored{}, stored{}, fmt.Errorf("give -base or -head (a result number from -history); the other side defaults to the latest")
	}
	if base == latestRef && head == latestRef {
		return stored{}, stored{}, fmt.Errorf("-base and -head are both %q: name a result number for one side", latestRef)
	}
	if len(all) < 2 {
		return stored{}, stored{}, fmt.Errorf("need at least 2 results to compare, have %d", len(all))
	}
	if base == "" {
		base = latestRef
	}
	if head == "" {
		head = latestRef
	}
	a, err := pick(all, base)
	if err != nil {
		return stored{}, stored{}, fmt.Errorf("-base: %w", err)
	}
	b, err := pick(all, head)
	if err != nil {
		return stored{}, stored{}, fmt.Errorf("-head: %w", err)
	}
	if a.Num == b.Num {
		return stored{}, stored{}, fmt.Errorf("-base and -head are the same result (%d)", a.Num)
	}
	return a, b, nil
}

func signed(v float64, f string) string {
	s := fmt.Sprintf(f, v)
	if v > 0 {
		return "+" + s
	}
	return s
}

func pctDelta(a, b float64) string {
	return fmt.Sprintf("%s → %s (%s pp)", pct(a), pct(b), signed(100*(b-a), "%.2f"))
}

// compareMarkdown compares the corpora that both results hold.
func compareMarkdown(a, b stored) string {
	var s strings.Builder
	fmt.Fprintf(&s, "## Lafke benchmark comparison\n\n")
	for _, x := range []struct {
		label string
		r     stored
	}{{"base", a}, {"head", b}} {
		fmt.Fprintf(&s, "- %s: %s, %s, lafke `%s`, engine `%s`, %d runs, %s/%s with %d CPUs\n",
			x.label, x.r.Label, x.r.Date, x.r.Env.Lafke, engineName(x.r.Env.Engine), x.r.Runs, x.r.Env.OS, x.r.Env.Arch, x.r.Env.CPUs)
	}
	index := func(r stored) map[string]corpusResult {
		m := map[string]corpusResult{}
		for _, c := range r.Corpora {
			m[c.Name] = c
		}
		return m
	}
	ia, ib := index(a), index(b)
	var common, skipped []string
	for _, n := range sortedKeys(ia) {
		if _, ok := ib[n]; ok {
			common = append(common, n)
		} else {
			skipped = append(skipped, n)
		}
	}
	for _, n := range sortedKeys(ib) {
		if _, ok := ia[n]; !ok {
			skipped = append(skipped, n)
		}
	}
	if len(common) == 0 {
		fmt.Fprintf(&s, "\nNo corpus is in both results.\n")
		return s.String()
	}

	fmt.Fprintf(&s, "\n### Detection accuracy\n\n| Corpus | TP | FP | FN | TN | Recall | Precision | F1 | J |\n| --- | ---: | ---: | ---: | ---: | --- | --- | --- | --- |\n")
	var notes []string
	for _, n := range common {
		ca, cb := ia[n], ib[n]
		if ca.Rev != cb.Rev {
			notes = append(notes, fmt.Sprintf("`%s` is at a different revision (%.7s and %.7s), so its scores compare other cases", n, ca.Rev, cb.Rev))
		}
		xa, xb := ca.Accuracy, cb.Accuracy
		if xa == nil || xb == nil {
			continue
		}
		d := func(p, q int) string { return fmt.Sprintf("%d → %d (%+d)", p, q, q-p) }
		if !xa.FPDefined || !xb.FPDefined {
			fmt.Fprintf(&s, "| `%s` | %s | – | %s | – | %s | – | – | – |\n", n, d(xa.TP, xb.TP), d(xa.FN, xb.FN), pctDelta(xa.Recall, xb.Recall))
			continue
		}
		fmt.Fprintf(&s, "| `%s` | %s | %s | %s | %s | %s | %s | %s | %s |\n", n,
			d(xa.TP, xb.TP), d(xa.FP, xb.FP), d(xa.FN, xb.FN), d(xa.TN, xb.TN),
			pctDelta(xa.Recall, xb.Recall), pctDelta(xa.Precision, xb.Precision), pctDelta(xa.F1, xb.F1), pctDelta(xa.Youden, xb.Youden))
		if xa.Labeled != xb.Labeled {
			notes = append(notes, fmt.Sprintf("`%s` scored %d cases, then %d", n, xa.Labeled, xb.Labeled))
		}
	}

	timed := false
	for _, n := range common {
		timed = timed || (ia[n].Wall != nil && ib[n].Wall != nil)
	}
	if timed {
		fmt.Fprintf(&s, "\n### Speed and memory\n\n| Corpus | Median (s) | Peak memory (MiB) |\n| --- | --- | --- |\n")
		for _, n := range common {
			ca, cb := ia[n], ib[n]
			if ca.Wall == nil || cb.Wall == nil {
				continue
			}
			fmt.Fprintf(&s, "| `%s` | %.2f → %.2f (%s) | %.0f → %.0f (%s) |\n", n,
				ca.Wall.Median, cb.Wall.Median, signed(cb.Wall.Median-ca.Wall.Median, "%.2f"),
				ca.RSS.Median, cb.RSS.Median, signed(cb.RSS.Median-ca.RSS.Median, "%.0f"))
		}
		fmt.Fprintf(&s, "\nTimings from different machines or runs of different noise are not comparable; check the environment lines above.\n")
		if a.Env.OS != b.Env.OS || a.Env.Arch != b.Env.Arch || a.Env.CPUs != b.Env.CPUs {
			notes = append(notes, "the two results were measured on different machines, so the timings do not compare")
		}
	}
	if len(skipped) > 0 {
		notes = append(notes, "only in one result and left out: "+strings.Join(skipped, ", "))
	}
	if len(notes) > 0 {
		fmt.Fprintf(&s, "\nWorth a look:\n\n")
		for _, n := range notes {
			fmt.Fprintf(&s, "- %s\n", n)
		}
	}
	fmt.Fprintf(&s, "\npp is percentage points. The scores are not a held-out evaluation; see the README for how each corpus is scored.\n")
	return s.String()
}

// compareResults prints the comparison of two stored results.
func compareResults(dir, base, head string) error {
	all, err := loadStored(dir)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return fmt.Errorf("no results in %s", dir)
	}
	a, b, err := selectPair(all, base, head)
	if err != nil {
		return err
	}
	fmt.Print(compareMarkdown(a, b))
	return nil
}

// printHistory lists the stored results.
func printHistory(dir string) error {
	all, err := loadStored(dir)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return fmt.Errorf("no results in %s", dir)
	}
	fmt.Print(historyText(all))
	return nil
}

// loadRun reads every result in dir as one run: the results the per-corpus jobs of a workflow
// run wrote. The environment and date come from the first one.
func loadRun(dir string) (stored, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return stored{}, err
	}
	if len(files) == 0 {
		return stored{}, fmt.Errorf("no results in %s", dir)
	}
	slices.Sort(files)
	run := stored{Label: filepath.Base(filepath.Clean(dir)), Path: dir}
	for i, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return stored{}, err
		}
		var r result
		if err := json.Unmarshal(data, &r); err != nil {
			return stored{}, fmt.Errorf("%s: %w", f, err)
		}
		if i == 0 {
			run.result = r
			continue
		}
		run.Corpora = append(run.Corpora, r.Corpora...)
	}
	return run, nil
}

// compareRuns compares two directories of results, each one run, and appends the comparison to
// the job summary when GITHUB_STEP_SUMMARY is set.
func compareRuns(baseDir, headDir string) error {
	a, err := loadRun(baseDir)
	if err != nil {
		return err
	}
	b, err := loadRun(headDir)
	if err != nil {
		return err
	}
	md := compareMarkdown(a, b)
	fmt.Print(md)
	if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString(md)
		return err
	}
	return nil
}
