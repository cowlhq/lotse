// Command sitegen builds lotse.sh (decision 0022): it copies site/ as it is
// and renders the documentation in docs/ into pages beside it, so that the
// Markdown stays the one source and reads the same on GitHub.
//
//	go run ./internal/sitegen -out _site
package main

import (
	"bytes"
	"embed"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

const repo = "https://github.com/cowlhq/lotse"

//go:embed page.html
var files embed.FS

var layout = template.Must(template.ParseFS(files, "page.html"))

// A page is one Markdown file of docs/ and where it is published.
type page struct {
	Src   string // in the repository, such as docs/install.md
	URL   string // on the site, such as /docs/install/
	Nav   string // its name in the navigation; empty for pages not listed
	Group string
}

// pages are what the site publishes, in the order of the navigation. Every
// decision is published as well, under /docs/decisions/.
var pages = []page{
	{Src: "docs/install.md", URL: "/docs/install/", Nav: "Installing lotse", Group: "Guides"},
	{Src: "docs/alerts.md", URL: "/docs/alerts/", Nav: "Checks and alerts", Group: "Guides"},
	{Src: "docs/configuration.md", URL: "/docs/configuration/", Nav: "Configuration as code", Group: "Guides"},
	{Src: "docs/api.md", URL: "/docs/api/", Nav: "The read API", Group: "Guides"},
	{Src: "docs/design.md", URL: "/docs/design/", Nav: "Design", Group: "How lotse works"},
	{Src: "docs/decisions/README.md", URL: "/docs/decisions/", Nav: "Decisions", Group: "How lotse works"},
}

func main() {
	root := flag.String("root", ".", "the repository")
	out := flag.String("out", "_site", "where the site goes; it is made anew")
	flag.Parse()
	if err := build(*root, *out); err != nil {
		log.Fatal(err)
	}
}

func build(root, out string) error {
	all, err := withDecisions(root)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	if err := os.CopyFS(out, os.DirFS(filepath.Join(root, "site"))); err != nil {
		return err
	}
	bySrc := map[string]page{}
	for _, p := range all {
		bySrc[p.Src] = p
	}
	for _, p := range all {
		src, err := os.ReadFile(filepath.Join(root, p.Src))
		if err != nil {
			return err
		}
		html, err := render(root, p, src, bySrc)
		if err != nil {
			return fmt.Errorf("%s: %w", p.Src, err)
		}
		dst := filepath.Join(out, filepath.FromSlash(strings.TrimPrefix(p.URL, "/")), "index.html")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, html, 0o644); err != nil {
			return err
		}
	}
	// /docs/ itself leads to the first guide.
	return os.WriteFile(filepath.Join(out, "docs", "index.html"), redirect(pages[0].URL), 0o644)
}

// withDecisions adds a page for every decision record.
func withDecisions(root string) ([]page, error) {
	all := append([]page(nil), pages...)
	list, err := fs.Glob(os.DirFS(root), "docs/decisions/0*.md")
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, errors.New("no decisions in docs/decisions")
	}
	for _, src := range list {
		name := strings.TrimSuffix(path.Base(src), ".md")
		all = append(all, page{Src: src, URL: "/docs/decisions/" + name + "/", Group: "How lotse works"})
	}
	return all, nil
}

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

type heading struct{ ID, Text string }

func render(root string, p page, src []byte, bySrc map[string]page) ([]byte, error) {
	doc := md.Parser().Parse(text.NewReader(src))
	var title string
	var toc []heading
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading:
			t := plain(n, src)
			if n.Level == 1 && title == "" {
				title = t
			}
			if v, ok := n.AttributeString("id"); ok && n.Level == 2 {
				if id, ok := v.([]byte); ok {
					toc = append(toc, heading{ID: string(id), Text: t})
				}
			}
		case *ast.Link:
			n.Destination = []byte(link(root, p.Src, string(n.Destination), bySrc))
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	if title == "" {
		return nil, errors.New("no title (# heading)")
	}
	var body bytes.Buffer
	if err := md.Renderer().Render(&body, src, doc); err != nil {
		return nil, err
	}
	if len(toc) < 3 {
		toc = nil
	}
	var buf bytes.Buffer
	err = layout.Execute(&buf, map[string]any{
		"Title":   title,
		"Body":    template.HTML(body.String()),
		"Page":    p,
		"Nav":     navigation(p),
		"TOC":     toc,
		"EditURL": repo + "/blob/main/" + p.Src,
	})
	return buf.Bytes(), err
}

// plain is a heading's text without its markup.
func plain(n ast.Node, src []byte) string {
	var b strings.Builder
	ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := n.(*ast.Text); ok && entering {
			b.Write(t.Segment.Value(src))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

type navGroup struct {
	Name  string
	Pages []navItem
}

type navItem struct {
	Name, URL string
	Current   bool
}

func navigation(cur page) []navGroup {
	var groups []navGroup
	for _, p := range pages {
		if len(groups) == 0 || groups[len(groups)-1].Name != p.Group {
			groups = append(groups, navGroup{Name: p.Group})
		}
		g := &groups[len(groups)-1]
		// A decision counts as being under Decisions.
		current := p.URL == cur.URL || (p.URL == "/docs/decisions/" && strings.HasPrefix(cur.URL, p.URL))
		g.Pages = append(g.Pages, navItem{Name: p.Nav, URL: p.URL, Current: current})
	}
	return groups
}

// link makes a link of a Markdown file work on the site: one to another
// published file leads to its page, one to anything else in the repository
// to GitHub.
func link(root, from, dest string, bySrc map[string]page) string {
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "/") {
		return dest
	}
	target := path.Join(path.Dir(from), u.Path)
	frag := ""
	if u.Fragment != "" {
		frag = "#" + u.Fragment
	}
	if p, ok := bySrc[target]; ok {
		return p.URL + frag
	}
	if p, ok := bySrc[path.Join(target, "README.md")]; ok {
		return p.URL + frag
	}
	kind := "blob"
	if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(target))); err == nil && fi.IsDir() {
		kind = "tree"
	}
	return repo + "/" + kind + "/main/" + target + frag
}

func redirect(to string) []byte {
	return fmt.Appendf(nil, `<!doctype html>
<meta charset="utf-8">
<meta name="go-import" content="lotse.sh git %s">
<meta http-equiv="refresh" content="0; url=%s">
<link rel="canonical" href="%s">
<title>lotse docs</title>
<a href="%s">lotse docs</a>
`, repo, to, to, to)
}
