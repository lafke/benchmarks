package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// labels points at the expected results of an OWASP Benchmark suite and maps each of its
// categories to the lafke rule that should report it.
type labels struct {
	File       string            `json:"file"`
	Categories map[string]string `json:"categories"`
}

type counts struct {
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
	TN        int     `json:"tn"`
	Recall    float64 `json:"recall"`
	Precision float64 `json:"precision"`
	F1        float64 `json:"f1"`
}

type accuracy struct {
	Labeled            int `json:"labeled"`
	AnalysisIncomplete int `json:"analysis_incomplete"`
	counts
	Categories map[string]*counts `json:"categories"`
}

var testName = regexp.MustCompile(`BenchmarkTest\d+`)

func (c *counts) add(real, hit bool) {
	switch {
	case real && hit:
		c.TP++
	case real:
		c.FN++
	case hit:
		c.FP++
	default:
		c.TN++
	}
}

func (c *counts) finish() {
	if c.TP+c.FN > 0 {
		c.Recall = float64(c.TP) / float64(c.TP+c.FN)
	}
	if c.TP+c.FP > 0 {
		c.Precision = float64(c.TP) / float64(c.TP+c.FP)
	}
	if c.Recall+c.Precision > 0 {
		c.F1 = 2 * c.Recall * c.Precision / (c.Recall + c.Precision)
	}
}

// score compares the findings of a json report with the labeled test cases. A test case is a
// hit when a finding of the expected rule is reported in its file. Every label counts, so a
// case lafke cannot analyze is a false negative (vulnerable) or a true negative (safe). Findings
// outside the labeled files are not part of the score.
func score(root string, l *labels, report []byte) (*accuracy, error) {
	var rep struct {
		Findings []struct {
			ID       string `json:"id"`
			Location struct {
				File string `json:"file"`
			} `json:"location"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(report, &rep); err != nil {
		return nil, fmt.Errorf("parse the lafke report: %w", err)
	}
	acc := &accuracy{Categories: map[string]*counts{}}
	hits := map[string]map[string]bool{}
	for _, f := range rep.Findings {
		if f.ID == "sast:analysis-incomplete" {
			acc.AnalysisIncomplete++
		}
		if name := testName.FindString(f.Location.File); name != "" {
			if hits[name] == nil {
				hits[name] = map[string]bool{}
			}
			hits[name][f.ID] = true
		}
	}

	file, err := os.Open(filepath.Join(root, l.File))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	r := csv.NewReader(file)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", l.File, err)
		}
		if len(rec) < 3 {
			return nil, fmt.Errorf("%s: unexpected record %q", l.File, rec)
		}
		name, category, real := rec[0], rec[1], rec[2] == "true"
		rule, ok := l.Categories[category]
		if !ok {
			return nil, fmt.Errorf("%s: category %q has no rule in corpora.json", l.File, category)
		}
		hit := hits[name][rule]
		if acc.Categories[category] == nil {
			acc.Categories[category] = &counts{}
		}
		acc.Categories[category].add(real, hit)
		acc.counts.add(real, hit)
		acc.Labeled++
	}
	for _, c := range acc.Categories {
		c.finish()
	}
	acc.counts.finish()
	return acc, nil
}
