// Package updatetest serves fake lotse releases, signed with a key of its
// own, for tests of updates.
package updatetest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"aead.dev/minisign"

	"lotse.sh/internal/update"
)

// Server serves releases at URL as /v<version>/<file>, and GitHub's
// "latest release" at URL/latest.
type Server struct {
	URL    string
	Latest string
	files  map[string][]byte
}

// New serves the given versions, each a lotse that is just its version,
// and makes the binary trust their key for the test.
func New(t *testing.T, versions ...string) *Server {
	t.Helper()
	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(update.TrustKeyForTesting(pub.String()))
	s := &Server{files: map[string][]byte{}}
	for _, v := range versions {
		s.add(v, priv)
		s.Latest = v
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			fmt.Fprintf(w, `{"tag_name":"v%s","body":"notes of %s","html_url":"https://example.com/v%s","published_at":%q}`,
				s.Latest, s.Latest, s.Latest, time.Unix(1_700_000_000, 0).UTC().Format(time.RFC3339))
			return
		}
		b, ok := s.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	s.URL = srv.URL
	return s
}

func (s *Server) add(version string, key minisign.PrivateKey) {
	var tgz bytes.Buffer
	gz := gzip.NewWriter(&tgz)
	tw := tar.NewWriter(gz)
	body := []byte("lotse " + version)
	tw.WriteHeader(&tar.Header{Name: "lotse", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
	tw.Write(body)
	tw.Close()
	gz.Close()
	archive := fmt.Sprintf("lotse_%s_linux_%s.tar.gz", version, runtime.GOARCH)
	sum := sha256.Sum256(tgz.Bytes())
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + archive + "\n")
	dir := "/v" + strings.TrimPrefix(version, "v") + "/"
	s.files[dir+archive] = tgz.Bytes()
	s.files[dir+"checksums.txt"] = sums
	s.files[dir+"checksums.txt.minisig"] = minisign.Sign(key, sums)
}
