// Command bench measures the wall time and peak memory of lafke on pinned public corpora.
//
// Corpora are cloned at the exact revision in corpora.json, so every run analyzes the same
// source. Each measurement runs lafke in a fresh process. Results are written as JSON
// together with the environment they were measured in.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

type corpus struct {
	Name    string   `json:"name"`
	Repo    string   `json:"repo"`
	Rev     string   `json:"rev"`
	Path    string   `json:"path"`
	Command []string `json:"command"`
}

type stats struct {
	Min    float64 `json:"min"`
	Median float64 `json:"median"`
	Mean   float64 `json:"mean"`
	Stdev  float64 `json:"stdev"`
}

type corpusResult struct {
	Name    string               `json:"name"`
	Repo    string               `json:"repo"`
	Rev     string               `json:"rev"`
	Command []string             `json:"command"`
	Wall    stats                `json:"wall_seconds"`
	RSS     stats                `json:"peak_rss_mib"`
	Samples map[string][]float64 `json:"samples"`
}

type environment struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	CPUs   int    `json:"cpus"`
	Go     string `json:"go"`
	Lafke  string `json:"lafke"`
	Engine string `json:"engine"`
}

type result struct {
	Date    string         `json:"date"`
	Env     environment    `json:"environment"`
	Runs    int            `json:"runs_per_corpus"`
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

func run() error {
	lafke := flag.String("lafke", "lafke", "lafke executable")
	runs := flag.Int("runs", 5, "measured runs per corpus")
	corporaFile := flag.String("corpora", "corpora.json", "corpora definition")
	cache := flag.String("cache", ".cache", "where corpora are cloned")
	out := flag.String("out", "results", "directory for the JSON result")
	var only multiFlag
	flag.Var(&only, "only", "run only this corpus (repeatable)")
	flag.Parse()
	if *runs < 1 {
		return fmt.Errorf("-runs must be at least 1")
	}

	data, err := os.ReadFile(*corporaFile)
	if err != nil {
		return err
	}
	var def struct {
		Corpora []corpus `json:"corpora"`
	}
	if err := json.Unmarshal(data, &def); err != nil {
		return fmt.Errorf("%s: %w", *corporaFile, err)
	}

	version, err := output(*lafke, "version")
	if err != nil {
		return err
	}
	engine, err := output(*lafke, "engine", "status")
	if err != nil {
		return err
	}
	res := result{
		Date: time.Now().UTC().Format(time.RFC3339),
		Env: environment{
			OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(),
			Go: runtime.Version(), Lafke: version, Engine: engine,
		},
		Runs: *runs,
	}

	for _, c := range def.Corpora {
		if len(only) > 0 && !slices.Contains(only, c.Name) {
			continue
		}
		src, err := fetch(c, *cache)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Name, err)
		}
		args := append(slices.Clone(c.Command), filepath.Join(src, c.Path), "--format", "json", "--fail-on", "critical")
		fmt.Fprintf(os.Stderr, "%s: warm-up\n", c.Name)
		if _, _, err := measure(*lafke, args); err != nil {
			return fmt.Errorf("%s: %w", c.Name, err)
		}
		var walls, rsss []float64
		for i := 1; i <= *runs; i++ {
			wall, rss, err := measure(*lafke, args)
			if err != nil {
				return fmt.Errorf("%s: %w", c.Name, err)
			}
			walls, rsss = append(walls, wall), append(rsss, rss)
			fmt.Fprintf(os.Stderr, "%s: run %d/%d %.2fs %.0f MiB\n", c.Name, i, *runs, wall, rss)
		}
		res.Corpora = append(res.Corpora, corpusResult{
			Name: c.Name, Repo: c.Repo, Rev: c.Rev,
			Command: append([]string{*lafke}, c.Command...),
			Wall:    summarize(walls), RSS: summarize(rsss),
			Samples: map[string][]float64{"wall_seconds": walls, "peak_rss_mib": rsss},
		})
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	path := filepath.Join(*out, time.Now().UTC().Format("20060102T150405Z")+".json")
	enc, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(enc, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Println(path)
	return writeSummary(res)
}

func output(name string, args ...string) (string, error) {
	b, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(b)), nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}

// fetch checks the corpus out at its pinned revision and verifies the result.
func fetch(c corpus, cache string) (string, error) {
	dest, err := filepath.Abs(filepath.Join(cache, c.Name))
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(dest); err != nil {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return "", err
		}
		if _, err := git(dest, "init", "-q"); err != nil {
			return "", err
		}
		if _, err := git(dest, "remote", "add", "origin", c.Repo); err != nil {
			return "", err
		}
	}
	if _, err := git(dest, "fetch", "-q", "--depth", "1", "origin", c.Rev); err != nil {
		return "", err
	}
	if _, err := git(dest, "checkout", "-q", "--detach", "FETCH_HEAD"); err != nil {
		return "", err
	}
	head, err := git(dest, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if head != c.Rev {
		return "", fmt.Errorf("expected %s, got %s", c.Rev, head)
	}
	return dest, nil
}

// measure runs lafke once and returns its wall time in seconds and peak memory in MiB.
// Exit code 1 means findings and counts as success; any other failure is an error.
func measure(lafke string, args []string) (float64, float64, error) {
	cmd := exec.Command(lafke, args...)
	start := time.Now()
	err := cmd.Run()
	wall := time.Since(start).Seconds()
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		err = nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("lafke %s: %w", strings.Join(args, " "), err)
	}
	return wall, peakRSSMiB(cmd.ProcessState), nil
}
