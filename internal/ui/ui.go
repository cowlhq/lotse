// Package ui serves the web interface that is built from web/ into dist/.
//
// dist/ is not committed (decision 0002). A binary built without it, with
// go install for example, serves a page that says where to get one with it.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Files returns the built web interface, or nil if this binary has none.
func Files() fs.FS {
	files, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(files, "index.html"); err != nil {
		return nil
	}
	return files
}

// Handler serves files from files. A path that names no file gets
// index.html, so that the app's own routes work on reload; a missing file
// under /assets/ is a 404. With no files at all it serves notBuilt.
func Handler(files fs.FS) http.Handler {
	if files == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(notBuilt))
		})
	}
	server := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if strings.HasPrefix(name, "assets/") {
			// File names under assets/ carry a hash of their content.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			server.ServeHTTP(w, r)
			return
		}
		if name == "" || name == "index.html" || isDir(files, name) || !exists(files, name) {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFileFS(w, r, files, "index.html")
			return
		}
		server.ServeHTTP(w, r)
	})
}

func exists(files fs.FS, name string) bool {
	_, err := fs.Stat(files, name)
	return err == nil
}

func isDir(files fs.FS, name string) bool {
	info, err := fs.Stat(files, name)
	return err == nil && info.IsDir()
}

const notBuilt = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>lotse</title></head>
<body style="font-family: system-ui, sans-serif; max-width: 40rem; margin: 4rem auto; line-height: 1.5">
<h1>lotse</h1>
<p>This lotse binary was built without its web interface, as happens with
<code>go install</code>. To run the hub, use a release binary from
<a href="https://github.com/cowlhq/lotse/releases">GitHub</a> or the image
<code>ghcr.io/cowlhq/lotse</code>, or build it with <code>mise run build</code>.</p>
</body></html>
`
