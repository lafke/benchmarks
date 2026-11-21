// Command bench measures lafke on pinned public corpora: wall time and peak memory, and
// detection accuracy against the labels each corpus comes with.
//
// Corpora are fetched at the exact revision in corpora.json, so every run analyzes the same
// source. The units of a corpus are fetched and analyzed in parallel; timing runs are sequential
// so that they do not disturb each other. Results are written as JSON together with the
// environment they were measured in.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

type environment struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	CPUs   int    `json:"cpus"`
	Go     string `json:"go"`
	Lafke  string `json:"lafke"`
	Engine string `json:"engine"`
}

type corpusResult struct {
	Name     string               `json:"name"`
	Kind     string               `json:"kind"`
	Repo     string               `json:"repo"`
	Rev      string               `json:"rev"`
	Command  []string             `json:"command"`
	Units    int                  `json:"units"`
	Wall     *stats               `json:"wall_seconds,omitempty"`
	RSS      *stats               `json:"peak_rss_mib,omitempty"`
	Accuracy *accuracy            `json:"accuracy,omitempty"`
	Samples  map[string][]float64 `json:"samples,omitempty"`
}

type result struct {
	Date    string         `json:"date"`
	Env     environment    `json:"environment"`
	Runs    int            `json:"runs_per_corpus"`
	Jobs    int            `json:"jobs"`
	Corpora []corpusResult `json:"corpora"`
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(1)
	}
}

func logf(format string, a ...any) { fmt.Fprintf(os.Stderr, format+"\n", a...) }

func run() error {
	lafke := flag.String("lafke", "lafke", "lafke executable")
	runs := flag.Int("runs", 5, "measured runs per corpus; 0 skips timing and only scores accuracy")
	jobs := flag.Int("jobs", max(2, runtime.NumCPU()/2), "parallel git operations and lafke runs for accuracy")
	corporaFile := flag.String("corpora", "corpora.json", "corpora definition")
	cache := flag.String("cache", ".cache", "where corpora are fetched")
	unitTimeout := flag.Duration("unit-timeout", 10*time.Minute, "give up on one lafke run after this long; the unit then has no findings")
	out := flag.String("out", "results", "directory for the JSON result")
	merge := flag.String("merge", "", "merge the JSON results in this directory into one job summary and exit")
	summary := flag.Bool("summary", true, "append the results to the GitHub Actions job summary (GITHUB_STEP_SUMMARY)")
	list := flag.Bool("list", false, "print the corpus names as a JSON array and exit")
	var only multiFlag
	flag.Var(&only, "only", "run only this corpus (repeatable)")
	flag.Parse()

	if *merge != "" {
		return mergeResults(*merge)
	}
	cfg, err := loadCorpora(*corporaFile)
	if err != nil {
		return err
	}
	var names []string
	for _, c := range cfg.Corpora {
		names = append(names, c.Name)
	}
	if *list {
		b, _ := json.Marshal(names)
		fmt.Println(string(b))
		return nil
	}
	for _, o := range only {
		if !slices.Contains(names, o) {
			return fmt.Errorf("unknown corpus %q (have %s)", o, strings.Join(names, ", "))
		}
	}
	if *runs < 0 || *jobs < 1 {
		return fmt.Errorf("-runs must be 0 or more and -jobs at least 1")
	}
	absCache, err := filepath.Abs(*cache)
	if err != nil {
		return err
	}
	var selected []*corpus
	for i := range cfg.Corpora {
		if len(only) == 0 || slices.Contains(only, cfg.Corpora[i].Name) {
			selected = append(selected, &cfg.Corpora[i])
		}
	}

	version, err := output(*lafke, "version")
	if err != nil {
		return err
	}
	engine, err := output(*lafke, "engine", "status")
	if err != nil {
		return err
	}
	e := &env{cache: absCache, groups: newCWEGroups(cfg.CWEGroups), sem: make(chan struct{}, *jobs)}
	res := result{
		Date: time.Now().UTC().Format(time.RFC3339),
		Env: environment{OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(),
			Go: runtime.Version(), Lafke: version, Engine: engine},
		Runs: *runs, Jobs: *jobs,
	}

	// Fetch every corpus at the same time; the semaphore bounds the git operations.
	units := make([][]unit, len(selected))
	if err := parallel(len(selected), len(selected), func(i int) error {
		c := selected[i]
		start := time.Now()
		u, err := suites[c.Kind].prepare(e, c)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Name, err)
		}
		units[i] = u
		logf("%s: fetched %d unit(s) in %.0fs", c.Name, len(u), time.Since(start).Seconds())
		return nil
	}); err != nil {
		return err
	}

	// Analyze all units of all corpora on one pool, for the findings the scores come from.
	type job struct{ corpus, unit int }
	var queue []job
	for i := range selected {
		for j := range units[i] {
			queue = append(queue, job{i, j})
		}
	}
	results := make([]map[string]*unitResult, len(selected))
	for i := range results {
		results[i] = map[string]*unitResult{}
	}
	var mu sync.Mutex
	var done int
	start := time.Now()
	if err := parallel(len(queue), *jobs, func(k int) error {
		jb := queue[k]
		c, u := selected[jb.corpus], units[jb.corpus][jb.unit]
		_, _, report, err := measure(*lafke, lafkeArgs(c, u.Dir), true, true, *unitTimeout)
		if errors.Is(err, errTimeout) {
			logf("%s/%s: timed out after %s, scored without findings", c.Name, u.Name, *unitTimeout)
			mu.Lock()
			results[jb.corpus][u.Name] = &unitResult{timedOut: true}
			mu.Unlock()
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s/%s: %w", c.Name, u.Name, err)
		}
		fs, inc, err := parseFindings(report, u.Prefix)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", c.Name, u.Name, err)
		}
		mu.Lock()
		results[jb.corpus][u.Name] = &unitResult{findings: fs, incomplete: inc}
		done++
		if done%25 == 0 || done == len(queue) {
			logf("analyzed %d/%d units (%.0fs)", done, len(queue), time.Since(start).Seconds())
		}
		mu.Unlock()
		return nil
	}); err != nil {
		return err
	}

	for i, c := range selected {
		cr := corpusResult{Name: c.Name, Kind: c.Kind, Repo: c.Repo, Rev: c.Rev, Command: append([]string{*lafke}, c.Command...), Units: len(units[i])}
		acc, err := suites[c.Kind].score(e, c, units[i], results[i])
		if err != nil {
			return fmt.Errorf("%s: %w", c.Name, err)
		}
		for _, r := range results[i] {
			if r.timedOut {
				acc.TimedOut++
			}
		}
		cr.Accuracy = acc
		logf("%s: TP %d FP %d FN %d TN %d, recall %.2f%%, precision %.2f%%, F1 %.2f%%",
			c.Name, acc.TP, acc.FP, acc.FN, acc.TN, 100*acc.Recall, 100*acc.Precision, 100*acc.F1)

		timed := len(units[i]) == 1
		if c.Timing != nil {
			timed = *c.Timing
		}
		if timed && *runs > 0 {
			args := lafkeArgs(c, units[i][0].Dir)
			logf("%s: warm-up", c.Name)
			if _, _, _, err := measure(*lafke, args, false, cr.Accuracy.AnalysisIncomplete > 0, 4**unitTimeout); err != nil {
				return fmt.Errorf("%s: %w", c.Name, err)
			}
			var walls, rsss []float64
			for k := 1; k <= *runs; k++ {
				wall, rss, _, err := measure(*lafke, args, false, cr.Accuracy.AnalysisIncomplete > 0, 4**unitTimeout)
				if err != nil {
					return fmt.Errorf("%s: %w", c.Name, err)
				}
				walls, rsss = append(walls, wall), append(rsss, rss)
				logf("%s: run %d/%d %.2fs %.0f MiB", c.Name, k, *runs, wall, rss)
			}
			w, r := summarize(walls), summarize(rsss)
			cr.Wall, cr.RSS = &w, &r
			cr.Samples = map[string][]float64{"wall_seconds": walls, "peak_rss_mib": rsss}
		}
		res.Corpora = append(res.Corpora, cr)
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	name := time.Now().UTC().Format("20060102T150405Z")
	if len(only) == 1 {
		name = only[0] + "-" + name
	}
	path := filepath.Join(*out, name+".json")
	enc, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(enc, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Println(path)
	if !*summary {
		return nil
	}
	return writeSummary([]result{res})
}
