package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The OWASP Benchmark and BenchProctor share a layout: one file per test case named
// BenchmarkTest00001, and a CSV with a row (test name, category, real vulnerability, CWE) for each.

// testName finds the test case in a file name: BenchmarkTest00001.java, or benchmark_test_00001.go
// where the language's convention is snake case.
var testName = regexp.MustCompile(`(?:BenchmarkTest|benchmark_test_)(\d{5,})\.\w+$`)

type labelRow struct {
	Name     string
	Category string
	Real     bool
	CWE      int
}

func readLabelCSV(path string) ([]labelRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	var rows []labelRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if len(rec) < 3 {
			return nil, fmt.Errorf("%s: unexpected record %q", path, rec)
		}
		row := labelRow{Name: strings.TrimSpace(rec[0]), Category: strings.TrimSpace(rec[1]), Real: strings.TrimSpace(rec[2]) == "true"}
		if len(rec) > 3 {
			row.CWE, _ = strconv.Atoi(strings.TrimSpace(rec[3]))
		}
		rows = append(rows, row)
	}
}

// byTestName groups the findings by the test case their file belongs to.
func byTestName(findings []finding) map[string][]finding {
	m := map[string][]finding{}
	for _, f := range findings {
		if m2 := testName.FindStringSubmatch(f.File); m2 != nil {
			m["BenchmarkTest"+m2[1]] = append(m["BenchmarkTest"+m2[1]], f)
		}
	}
	return m
}

// owaspSuite scores the OWASP Benchmark. A test case is a hit when the rule expected for its
// category reports a finding in its file.
type owaspSuite struct{}

// labels points at the expected results of an OWASP Benchmark suite and maps each of its
// categories to the lafke rule that should report it.
type labels struct {
	File       string            `json:"file"`
	Categories map[string]string `json:"categories"`
}

func (owaspSuite) prepare(e *env, c *corpus) ([]unit, error) {
	dest := filepath.Join(e.cache, c.Name)
	if err := e.fetchRepo(dest, c.Repo, c.Rev, c.Sparse); err != nil {
		return nil, err
	}
	return []unit{{Name: c.Name, Dir: filepath.Join(dest, c.Path)}}, nil
}

func (owaspSuite) score(e *env, c *corpus, units []unit, res map[string]*unitResult) (*accuracy, error) {
	if c.Labels == nil {
		return nil, fmt.Errorf("labels are missing")
	}
	root := filepath.Join(e.cache, c.Name)
	rows, err := readLabelCSV(filepath.Join(root, c.Labels.File))
	if err != nil {
		return nil, err
	}
	r := res[units[0].Name]
	hits := map[string]map[string]bool{}
	for n, fs := range byTestName(r.findings) {
		hits[n] = map[string]bool{}
		for _, f := range fs {
			hits[n][f.ID] = true
		}
	}
	t := newTally(true)
	for _, row := range rows {
		rule, ok := c.Labels.Categories[row.Category]
		if !ok {
			return nil, fmt.Errorf("%s: category %q has no rule in corpora.json", c.Labels.File, row.Category)
		}
		t.add(row.Category, row.Real, hits[row.Name][rule])
	}
	t.acc.AnalysisIncomplete = r.incomplete
	return t.result(), nil
}

// cweCSVSuite scores BenchProctor's standalone cases of one language. Every framework folder is a
// unit with its own testcode directory and answer key. A case is a hit when a finding in its file
// has the expected CWE, or one of the same group.
type cweCSVSuite struct{}

type cweUnitMeta struct{ keyFile string }

func (cweCSVSuite) prepare(e *env, c *corpus) ([]unit, error) {
	dest := filepath.Join(e.cache, c.Name)
	if err := e.fetchRepo(dest, c.Repo, c.Rev, c.Sparse); err != nil {
		return nil, err
	}
	base := filepath.Join(dest, c.Path)
	dirs, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var units []unit
	for _, d := range dirs {
		code := filepath.Join(base, d.Name(), "testcode")
		if _, err := os.Stat(code); err != nil {
			continue
		}
		keys, _ := filepath.Glob(filepath.Join(base, d.Name(), "expectedresults-*.csv"))
		if len(keys) != 1 {
			return nil, fmt.Errorf("%s: expected one answer key, found %d", d.Name(), len(keys))
		}
		// The framework folder is analyzed, not just testcode/, so that lafke sees the dependency
		// manifest (go.mod, pom.xml, ...) that sits next to it.
		units = append(units, unit{Name: d.Name(), Dir: filepath.Join(base, d.Name()), Meta: cweUnitMeta{keys[0]}})
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("no framework with a testcode directory under %s", c.Path)
	}
	return units, nil
}

func (cweCSVSuite) score(e *env, c *corpus, units []unit, res map[string]*unitResult) (*accuracy, error) {
	t := newTally(true)
	for _, u := range units {
		rows, err := readLabelCSV(u.Meta.(cweUnitMeta).keyFile)
		if err != nil {
			return nil, err
		}
		r := res[u.Name]
		t.acc.AnalysisIncomplete += r.incomplete
		fs := byTestName(r.findings)
		for _, row := range rows {
			if slices.Contains(c.SkipCategories, row.Category) {
				t.acc.Excluded++
				continue
			}
			hit := false
			for _, f := range fs[row.Name] {
				if e.groups.matchAny(row.CWE, f.CWEs) {
					hit = true
					break
				}
			}
			t.add(row.Category, row.Real, hit)
		}
	}
	return t.result(), nil
}
