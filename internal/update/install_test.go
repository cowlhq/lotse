package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aead.dev/minisign"
)

// release is a fake release server: each version's archive holds a lotse
// that is just its version, signed with key.
type release struct {
	key      minisign.PrivateKey
	files    map[string][]byte // path → content
	tamper   func(path string, b []byte) []byte
	requests int
}

func newRelease(t *testing.T, versions ...string) (*release, *httptest.Server) {
	t.Helper()
	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	text, _ := pub.MarshalText()
	old := extraKey
	extraKey = strings.TrimSpace(string(text[strings.IndexByte(string(text), '\n')+1:]))
	t.Cleanup(func() { extraKey = old })

	r := &release{key: priv, files: map[string][]byte{}}
	for _, v := range versions {
		r.add(t, v, priv)
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.requests++
		b, ok := r.files[req.URL.Path]
		if !ok {
			http.NotFound(w, req)
			return
		}
		if r.tamper != nil {
			b = r.tamper(req.URL.Path, b)
		}
		w.Write(b)
	}))
	t.Cleanup(s.Close)
	return r, s
}

func (r *release) add(t *testing.T, version string, key minisign.PrivateKey) {
	var tgz bytes.Buffer
	gz := gzip.NewWriter(&tgz)
	tw := tar.NewWriter(gz)
	body := []byte("lotse " + version)
	tw.WriteHeader(&tar.Header{Name: "LICENSE", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg})
	tw.Write([]byte("ok"))
	tw.WriteHeader(&tar.Header{Name: "lotse", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
	tw.Write(body)
	tw.Close()
	gz.Close()
	archive := fmt.Sprintf("lotse_%s_linux_%s.tar.gz", version, runtime.GOARCH)
	sum := sha256.Sum256(tgz.Bytes())
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + archive + "\n")
	dir := "/v" + version + "/"
	r.files[dir+archive] = tgz.Bytes()
	r.files[dir+"checksums.txt"] = sums
	r.files[dir+"checksums.txt.minisig"] = minisign.Sign(key, sums)
}

func installer(t *testing.T, url, own string) *Installer {
	return &Installer{Dir: t.TempDir(), ReleaseURL: url, Own: own}
}

func current(t *testing.T, in *Installer, link string) string {
	t.Helper()
	v, err := os.Readlink(filepath.Join(in.Dir, link))
	if err != nil {
		return ""
	}
	b, _ := os.ReadFile(filepath.Join(in.Dir, v, "lotse"))
	return v + ":" + string(b)
}

func TestInstall(t *testing.T) {
	_, srv := newRelease(t, "1.2.0", "1.3.0")
	in := installer(t, srv.URL, "1.1.0")
	if err := in.Install(t.Context(), "1.2.0"); err != nil {
		t.Fatal(err)
	}
	if got := current(t, in, "current"); got != "1.2.0:lotse 1.2.0" {
		t.Errorf("current: %q", got)
	}
	info, _ := os.Stat(filepath.Join(in.Dir, "1.2.0", "lotse"))
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode %v", info.Mode())
	}
	in.Own = "1.2.0"
	if err := in.Install(t.Context(), "v1.3.0"); err != nil {
		t.Fatal(err)
	}
	if got, prev := current(t, in, "current"), current(t, in, "previous"); got != "1.3.0:lotse 1.3.0" || prev != "1.2.0:lotse 1.2.0" {
		t.Errorf("current %q, previous %q", got, prev)
	}
}

func TestInstallRefuses(t *testing.T) {
	r, srv := newRelease(t, "1.2.0", "1.0.0")
	for _, c := range []struct {
		name, own, version, want string
	}{
		{"older", "1.1.0", "1.0.0", "not newer"},
		{"the same", "1.2.0", "1.2.0", "not newer"},
		{"development build", "dev", "1.2.0", "development build"},
		{"not a version", "1.1.0", "latest", "not newer"},
		{"missing", "1.1.0", "1.9.0", "404"},
	} {
		in := installer(t, srv.URL, c.own)
		if err := in.Install(t.Context(), c.version); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}

	// Signed with another key.
	_, other, _ := minisign.GenerateKey(rand.Reader)
	r.files["/v1.2.0/checksums.txt.minisig"] = minisign.Sign(other, r.files["/v1.2.0/checksums.txt"])
	in := installer(t, srv.URL, "1.1.0")
	if err := in.Install(t.Context(), "1.2.0"); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Errorf("another key: %v", err)
	}
	r.files["/v1.2.0/checksums.txt.minisig"] = minisign.Sign(r.key, r.files["/v1.2.0/checksums.txt"])

	// A changed archive: its checksum, which the signature covers, differs.
	r.tamper = func(path string, b []byte) []byte {
		if strings.HasSuffix(path, ".tar.gz") {
			return append(b, 0)
		}
		return b
	}
	if err := in.Install(t.Context(), "1.2.0"); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("a changed archive: %v", err)
	}
	r.tamper = nil
	if current(t, in, "current") != "" {
		t.Error("something became current after refusals")
	}

	// A version that failed to start before is not installed again.
	os.MkdirAll(filepath.Join(in.Dir, "1.2.0"), 0o755)
	os.WriteFile(filepath.Join(in.Dir, "1.2.0", "bad"), nil, 0o644)
	if err := in.Install(t.Context(), "1.2.0"); err == nil || !strings.Contains(err.Error(), "failed to start") {
		t.Errorf("a bad version: %v", err)
	}
}

func TestRolledBack(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "1.2.0"), 0o755)
	s := &Starter{Dir: dir, Log: nil}
	s.Log = discard()
	s.markBad("1.2.0")
	if got := TakeRolledBack(dir); got != "1.2.0" {
		t.Errorf("rolled back from %q", got)
	}
	if got := TakeRolledBack(dir); got != "" {
		t.Errorf("told twice: %q", got)
	}
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }
