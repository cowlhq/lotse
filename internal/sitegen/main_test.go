package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var href = regexp.MustCompile(`(?:href|src|srcset)="([^"]*)"`)

// The site builds from the repository; every page can serve go get, no
// link is left pointing at a Markdown file, and every link within the site
// leads somewhere.
func TestBuild(t *testing.T) {
	out := t.TempDir()
	if err := build("../..", out); err != nil {
		t.Fatal(err)
	}
	script, _ := os.ReadFile("../../site/install")
	if got, _ := os.ReadFile(filepath.Join(out, "install")); !bytes.Equal(got, script) {
		t.Error("the install script is not copied as it is")
	}
	for _, p := range []string{"docs/install/index.html", "docs/decisions/index.html", "docs/decisions/0012-lotse-sh/index.html"} {
		if _, err := os.Stat(filepath.Join(out, p)); err != nil {
			t.Error(err)
		}
	}
	pages := 0
	err := filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		pages++
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(out, path)
		if !bytes.Contains(b, []byte(`<meta name="go-import" content="lotse.sh git https://github.com/cowlhq/lotse">`)) {
			t.Errorf("%s has no go-import tag", rel)
		}
		for _, m := range href.FindAllSubmatch(b, -1) {
			link := string(m[1])
			if strings.Contains(link, ".md") && !strings.HasPrefix(link, "https://github.com/") {
				t.Errorf("%s links to %s", rel, link)
			}
			if !strings.HasPrefix(link, "/") {
				continue
			}
			target := filepath.Join(out, filepath.FromSlash(strings.SplitN(link, "#", 2)[0]))
			if strings.HasSuffix(link, "/") || !strings.Contains(filepath.Base(target), ".") {
				target = filepath.Join(target, "index.html")
			}
			if _, err := os.Stat(target); err != nil {
				t.Errorf("%s links to %s, which is not there", rel, link)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if pages < 30 {
		t.Errorf("only %d pages", pages)
	}
}

func TestLink(t *testing.T) {
	bySrc := map[string]page{
		"docs/install.md":          {URL: "/docs/install/"},
		"docs/decisions/README.md": {URL: "/docs/decisions/"},
		"docs/decisions/0010-x.md": {URL: "/docs/decisions/0010-x/"},
	}
	for _, c := range []struct{ from, dest, want string }{
		{"docs/alerts.md", "install.md#agents", "/docs/install/#agents"},
		{"docs/install.md", "decisions/0010-x.md", "/docs/decisions/0010-x/"},
		{"docs/decisions/0010-x.md", "README.md", "/docs/decisions/"},
		{"docs/install.md", "decisions/", "/docs/decisions/"},
		{"docs/install.md", "../deploy/compose.yml", "https://github.com/cowlhq/lotse/blob/main/deploy/compose.yml"},
		{"docs/api.md", "../client/", "https://github.com/cowlhq/lotse/tree/main/client"},
		{"docs/install.md", "https://healthchecks.io", "https://healthchecks.io"},
		{"docs/install.md", "#the-hub", "#the-hub"},
	} {
		if got := link("../..", c.from, c.dest, bySrc); got != c.want {
			t.Errorf("%s in %s: %s, want %s", c.dest, c.from, got, c.want)
		}
	}
}
