package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// julietSuite scores the Java test cases of the NIST Juliet Test Suite. A file holds a bad part
// (methods named bad*) and a good part (methods named good*). A finding with the CWE of the
// file's directory inside a bad method makes the vulnerable case a hit; inside a good method it
// makes the safe case a false positive. Only directories whose CWE is in a CWE group are scored.
type julietSuite struct{}

var (
	julietDir   = regexp.MustCompile(`(?:^|/)testcases/CWE(\d+)_[^/]*/`)
	julietShard = regexp.MustCompile(`^s\d+$`)
	// julietCase strips what tells the files of one test case apart: 54a, 54b, 81_bad, 81_goodB2G.
	julietCase   = regexp.MustCompile(`^(.*_\d\d)(?:[a-z]|_(?:base|bad|good\w*))?\.java$`)
	julietMethod = regexp.MustCompile(`^\s*(?:public|private|protected)\s+(?:static\s+)?(?:final\s+)?[\w<>\[\],.\s]*?\b(bad\w*|good\w*)\s*\(`)
)

type span struct {
	start, end int
	bad        bool
}

// prepare makes a unit of every directory of a scored CWE, or of each of its sNN subdirectories
// where it has them, so that the units can be analyzed in parallel. The bad and good parts of a
// case are in the same directory, and the helpers they use (testcasesupport) are not needed to
// follow the data flow.
func (julietSuite) prepare(e *env, c *corpus) ([]unit, error) {
	dest := filepath.Join(e.cache, c.Name)
	if err := e.fetchRepo(dest, c.Repo, c.Rev, c.Sparse); err != nil {
		return nil, err
	}
	root := filepath.Join(dest, c.Path)
	cwes, err := os.ReadDir(filepath.Join(root, "testcases"))
	if err != nil {
		return nil, err
	}
	var units []unit
	add := func(rel string) {
		units = append(units, unit{Name: rel, Dir: filepath.Join(root, rel), Prefix: rel})
	}
	for _, d := range cwes {
		m := julietDir.FindStringSubmatch("testcases/" + d.Name() + "/")
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if !e.groups.inScope(n) {
			continue
		}
		rel := "testcases/" + d.Name()
		subs, err := os.ReadDir(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		sharded := false
		for _, s := range subs {
			if s.IsDir() && julietShard.MatchString(s.Name()) {
				add(rel + "/" + s.Name())
				sharded = true
			}
		}
		if !sharded {
			add(rel)
		}
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("no scored CWE directory under %s/testcases", c.Path)
	}
	return units, nil
}

// methodSpans returns the line ranges of the bad* and good* methods of a Java source file.
func methodSpans(src string) []span {
	lines := strings.Split(src, "\n")
	var spans []span
	for i := 0; i < len(lines); i++ {
		m := julietMethod.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		depth, opened := 0, false
		for j := i; j < len(lines); j++ {
			line := lines[j]
			if k := strings.Index(line, "//"); k >= 0 {
				line = line[:k]
			}
			for _, ch := range line {
				switch ch {
				case '{':
					depth++
					opened = true
				case '}':
					depth--
				}
			}
			if opened && depth <= 0 {
				spans = append(spans, span{i + 1, j + 1, strings.HasPrefix(m[1], "bad")})
				i = j
				break
			}
		}
	}
	return spans
}

func (julietSuite) score(e *env, c *corpus, units []unit, res map[string]*unitResult) (*accuracy, error) {
	byFile := map[string][]finding{}
	incomplete := 0
	for _, r := range res {
		incomplete += r.incomplete
		for _, f := range r.findings {
			byFile[f.File] = append(byFile[f.File], f)
		}
	}
	srcRoot := filepath.Join(e.cache, c.Name, c.Path)
	// A test case can be spread over several files: a flow variant 54a to 54e, or the base,
	// bad and good classes of variant 81. They are one case, so it is scored as one.
	type testCase struct {
		cwe                              int
		hasBad, hasGood, hitBad, hitGood bool
	}
	cases := map[string]*testCase{}
	t := newTally(true)
	t.acc.AnalysisIncomplete = incomplete
	root := filepath.Join(srcRoot, "testcases")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".java") {
			return err
		}
		rel, _ := filepath.Rel(srcRoot, path)
		rel = filepath.ToSlash(rel)
		m := julietDir.FindStringSubmatch(rel)
		if m == nil {
			return nil
		}
		cwe, _ := strconv.Atoi(m[1])
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		spans := methodSpans(string(src))
		var hasBad, hasGood bool
		for _, s := range spans {
			hasBad = hasBad || s.bad
			hasGood = hasGood || !s.bad
		}
		if !hasBad && !hasGood {
			return nil
		}
		if !e.groups.inScope(cwe) {
			t.acc.Excluded++
			return nil
		}
		key := julietCase.ReplaceAllString(rel, "$1")
		c := cases[key]
		if c == nil {
			c = &testCase{cwe: cwe}
			cases[key] = c
		}
		c.hasBad, c.hasGood = c.hasBad || hasBad, c.hasGood || hasGood
		for _, f := range byFile[rel] {
			if !e.groups.matchAny(cwe, f.CWEs) {
				continue
			}
			for _, s := range spans {
				if f.Line >= s.start && f.Line <= s.end {
					c.hitBad = c.hitBad || s.bad
					c.hitGood = c.hitGood || !s.bad
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, c := range cases {
		cat := "CWE" + strconv.Itoa(c.cwe)
		if c.hasBad {
			t.add(cat, true, c.hitBad)
		}
		if c.hasGood {
			t.add(cat, false, c.hitGood)
		}
	}
	return t.result(), nil
}
