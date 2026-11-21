package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// vulnbenchSuite scores the fixtures of Snyk VulnBench that have an attacker-reachable answer key.
// The key only names vulnerabilities, so only recall is meaningful. This is lafke's own reading
// of the key, not the VulnBench scorer: a vulnerability is found when a finding of its type (the
// rule name equals the type or one of its aliases) lies within vulnbenchTolerance lines of one of
// the locations of its source-to-sink flow, in the same file.
type vulnbenchSuite struct{}

const vulnbenchTolerance = 3

type vbMeta struct{ key string }

type vbVuln struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	TypeAliases  []string `json:"typeAliases"`
	FilesRelated []struct {
		File string `json:"file"`
		Line int    `json:"line"`
	} `json:"filesRelated"`
}

func (vulnbenchSuite) prepare(e *env, c *corpus) ([]unit, error) {
	dest := filepath.Join(e.cache, c.Name)
	if err := e.fetchRepo(dest, c.Repo, c.Rev, c.Sparse); err != nil {
		return nil, err
	}
	fixtures, err := filepath.Glob(filepath.Join(dest, "fixtures", "*", "findings-attacker-reachable.json"))
	if err != nil {
		return nil, err
	}
	var units []unit
	for _, key := range fixtures {
		dir := filepath.Dir(key)
		units = append(units, unit{Name: filepath.Base(dir), Dir: filepath.Join(dir, "project"), Meta: vbMeta{key}})
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("no fixture with an attacker-reachable answer key")
	}
	return units, nil
}

func normType(s string) string {
	return strings.NewReplacer(" ", "-", "_", "-").Replace(strings.ToLower(strings.TrimSpace(s)))
}

func (vulnbenchSuite) score(e *env, c *corpus, units []unit, res map[string]*unitResult) (*accuracy, error) {
	t := newTally(false)
	for _, u := range units {
		data, err := os.ReadFile(u.Meta.(vbMeta).key)
		if err != nil {
			return nil, err
		}
		var key struct {
			Vulnerabilities []vbVuln `json:"vulnerabilities"`
		}
		if err := json.Unmarshal(stripJSONC(data), &key); err != nil {
			return nil, fmt.Errorf("%s: %w", u.Name, err)
		}
		r := res[u.Name]
		t.acc.AnalysisIncomplete += r.incomplete
		for _, v := range key.Vulnerabilities {
			types := []string{normType(v.Type)}
			for _, a := range v.TypeAliases {
				types = append(types, normType(a))
			}
			hit := false
			for _, f := range r.findings {
				if !slices.Contains(types, normType(ruleName(f.ID))) {
					continue
				}
				at := append([]loc{{f.File, f.Line}}, f.Trace...)
				for _, p := range at {
					for _, rel := range v.FilesRelated {
						if p.File == filepath.ToSlash(rel.File) && p.Line >= rel.Line-vulnbenchTolerance && p.Line <= rel.Line+vulnbenchTolerance {
							hit = true
						}
					}
				}
			}
			t.add(normType(v.Type), true, hit)
		}
	}
	return t.result(), nil
}

// stripJSONC removes // and /* */ comments, which some answer keys contain, outside of strings.
func stripJSONC(b []byte) []byte {
	out := make([]byte, 0, len(b))
	inString, escaped := false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case inString:
			out = append(out, c)
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(b) && b[i+1] == '/':
			for i < len(b) && b[i] != '\n' {
				i++
			}
			out = append(out, '\n')
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			i += 2
			for i+1 < len(b) && !(b[i] == '*' && b[i+1] == '/') {
				i++
			}
			i++
		default:
			out = append(out, c)
		}
	}
	return out
}
