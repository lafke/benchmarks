package main

import "sort"

type counts struct {
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
	TN        int     `json:"tn"`
	Recall    float64 `json:"recall"`
	Precision float64 `json:"precision"`
	FPR       float64 `json:"fpr"`
	F1        float64 `json:"f1"`
	Youden    float64 `json:"youden_j"`
}

type accuracy struct {
	Labeled            int `json:"labeled"`
	Excluded           int `json:"excluded"`
	AnalysisIncomplete int `json:"analysis_incomplete"`
	// TimedOut is the number of units where lafke did not finish in time. They have no findings,
	// so their vulnerable cases are false negatives.
	TimedOut int `json:"timed_out_units"`
	// FPDefined is false when the labels only name vulnerabilities, so a finding that matches
	// none is not known to be false. FP, TN, precision, F1 and J are then not meaningful.
	FPDefined bool `json:"fp_defined"`
	counts
	Categories map[string]*counts `json:"categories"`
}

// tally collects the outcome of every labeled case.
type tally struct {
	acc  accuracy
	cats map[string]*counts
}

func newTally(fpDefined bool) *tally {
	t := &tally{cats: map[string]*counts{}}
	t.acc.FPDefined = fpDefined
	return t
}

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

func (t *tally) category(name string) *counts {
	if t.cats[name] == nil {
		t.cats[name] = &counts{}
	}
	return t.cats[name]
}

// add records one labeled case.
func (t *tally) add(category string, real, hit bool) {
	t.category(category).add(real, hit)
	t.acc.counts.add(real, hit)
	t.acc.Labeled++
}

func (c *counts) finish() {
	div := func(a, b int) float64 {
		if b == 0 {
			return 0
		}
		return float64(a) / float64(b)
	}
	c.Recall = div(c.TP, c.TP+c.FN)
	c.Precision = div(c.TP, c.TP+c.FP)
	c.FPR = div(c.FP, c.FP+c.TN)
	c.Youden = c.Recall - c.FPR
	if den := c.Precision + c.Recall; den > 0 {
		c.F1 = 2 * c.Precision * c.Recall / den
	}
}

func (t *tally) result() *accuracy {
	a := t.acc
	a.Categories = t.cats
	a.counts.finish()
	for _, c := range t.cats {
		c.finish()
	}
	return &a
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
