package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The comparison at /compare/ is made from compare.yaml: every tool, and
// for every feature what each tool does. One source, so that pages that
// compare lotse with one tool at a time can come from it too.

//go:embed compare.yaml
var compareData []byte

var compareLayout = template.Must(template.New("compare.html").Funcs(template.FuncMap{
	"inc":  func(n int) int { return n + 1 },
	"mark": func(level string) string { return marks[level] },
	"host": func(s string) string {
		u, err := url.Parse(s)
		if err != nil {
			return s
		}
		return strings.TrimSuffix(u.Host+u.Path, "/")
	},
}).ParseFS(files, "compare.html"))

type comparison struct {
	AsOf   string  `yaml:"as_of"`
	Tools  []tool  `yaml:"tools"`
	Groups []group `yaml:"groups"`
}

type tool struct {
	ID      string   `yaml:"id"`
	Name    string   `yaml:"name"`
	URL     string   `yaml:"url"`
	Edition string   `yaml:"edition"` // which version or edition is compared
	Sources []string `yaml:"sources"`
}

type group struct {
	Name string `yaml:"name"`
	Rows []row  `yaml:"rows"`
}

// A row is one feature. Every tool has a cell, written as a level and,
// after a colon, a note: "yes", "partial: only Linux".
type row struct {
	Feature string            `yaml:"feature"`
	Cells   map[string]string `yaml:",inline"`
}

// A cell is how one tool does at one feature.
type cell struct {
	Level string // yes, partial, extra, paid, no
	Note  string
}

var levels = map[string]string{
	"yes":     "Yes",
	"partial": "Partly",
	"extra":   "With an add-on",
	"paid":    "In a paid edition",
	"no":      "No",
}

var marks = map[string]string{"yes": "✓", "partial": "◐", "extra": "+", "paid": "$", "no": "–"}

func (c cell) Label() string { return levels[c.Level] }

// cells are a row's cells in the order of the tools.
func (c comparison) cells(r row) ([]cell, error) {
	var out []cell
	for _, t := range c.Tools {
		v, ok := r.Cells[t.ID]
		if !ok {
			return nil, fmt.Errorf("%q has nothing for %s", r.Feature, t.ID)
		}
		level, note, _ := strings.Cut(v, ":")
		if _, ok := levels[level]; !ok {
			return nil, fmt.Errorf("%q, %s: %q is no level", r.Feature, t.ID, level)
		}
		out = append(out, cell{Level: level, Note: strings.TrimSpace(note)})
	}
	if len(r.Cells) != len(c.Tools) {
		return nil, fmt.Errorf("%q has cells for tools that are not compared", r.Feature)
	}
	return out, nil
}

func readComparison() (comparison, error) {
	var c comparison
	dec := yaml.NewDecoder(bytes.NewReader(compareData))
	dec.KnownFields(true)
	err := dec.Decode(&c)
	return c, err
}

// buildCompare writes /compare/.
func buildCompare(out string) error {
	c, err := readComparison()
	if err != nil {
		return fmt.Errorf("compare.yaml: %w", err)
	}
	type shownRow struct {
		Feature string
		Cells   []cell
	}
	type shownGroup struct {
		Name string
		Rows []shownRow
	}
	var groups []shownGroup
	for _, g := range c.Groups {
		sg := shownGroup{Name: g.Name}
		for _, r := range g.Rows {
			cells, err := c.cells(r)
			if err != nil {
				return fmt.Errorf("compare.yaml: %w", err)
			}
			sg.Rows = append(sg.Rows, shownRow{Feature: r.Feature, Cells: cells})
		}
		groups = append(groups, sg)
	}
	var buf bytes.Buffer
	err = compareLayout.Execute(&buf, map[string]any{
		"AsOf":   c.AsOf,
		"Tools":  c.Tools,
		"Others": c.Tools[1:],
		"Groups": groups,
	})
	if err != nil {
		return err
	}
	dst := filepath.Join(out, "compare", "index.html")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, buf.Bytes(), 0o644)
}
