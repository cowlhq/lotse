package update

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"aead.dev/minisign"
	"golang.org/x/mod/semver"
)

// trustedKeys are the minisign keys lotse's releases are signed with
// (decision 0010). There may be two while the key is replaced.
var trustedKeys = []string{
	"RWQGkOnHGbqiZ+oFSmuH/4VRHQB0FRdFZ3SzMcDyNqYc5apeh5h4SfQ6", // 67A2BA19C7E99006
}

// extraKey adds a key at build time, for trying updates with releases one
// signed oneself: -ldflags "-X lotse.sh/internal/update.extraKey=RW…".
// Release builds leave it empty.
var extraKey string

// DefaultReleaseURL is where releases are, as <url>/v<version>/<file>. Only
// the machine itself can name another place (a mirror), never the hub.
const DefaultReleaseURL = "https://github.com/cowlhq/lotse/releases/download"

// Limits on what is downloaded, so that a broken server cannot fill the disk.
const (
	maxChecksums = 64 << 10
	maxSignature = 4 << 10
	maxArchive   = 256 << 20
)

// Installer puts a release into the directory the starter runs versions
// from (see Starter) and makes it the current one.
type Installer struct {
	// Dir is the starter's directory.
	Dir string
	// ReleaseURL is DefaultReleaseURL unless the machine says otherwise.
	ReleaseURL string
	// Own is the version installing, which only installs newer ones.
	Own    string
	Client *http.Client
}

// Install downloads version, checks it and makes it current. It refuses a
// version that is not newer than the own one or that was marked bad.
func (in *Installer) Install(ctx context.Context, version string) error {
	version = strings.TrimPrefix(version, "v")
	if !semver.IsValid("v" + in.Own) {
		return fmt.Errorf("this is a development build (%s); it does not update", in.Own)
	}
	if !newer(version, in.Own) {
		return fmt.Errorf("%s is not newer than %s", version, in.Own)
	}
	s := Starter{Dir: in.Dir}
	if s.isBad(version) {
		return fmt.Errorf("%s failed to start here before", version)
	}
	binary := filepath.Join(in.Dir, version, "lotse")
	if _, err := os.Stat(binary); err != nil {
		if err := in.download(ctx, version, binary); err != nil {
			return err
		}
	}
	return in.switchTo(version)
}

func (in *Installer) download(ctx context.Context, version, binary string) error {
	base := strings.TrimSuffix(cmpOr(in.ReleaseURL, DefaultReleaseURL), "/") + "/v" + version + "/"
	archive := fmt.Sprintf("lotse_%s_linux_%s.tar.gz", version, runtime.GOARCH)

	sums, err := in.get(ctx, base+"checksums.txt", maxChecksums)
	if err != nil {
		return err
	}
	sig, err := in.get(ctx, base+"checksums.txt.minisig", maxSignature)
	if err != nil {
		return err
	}
	if err := verify(sums, sig); err != nil {
		return err
	}
	want, err := checksum(sums, archive)
	if err != nil {
		return err
	}
	data, err := in.get(ctx, base+archive, maxArchive)
	if err != nil {
		return err
	}
	if got := sha256.Sum256(data); hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("the checksum of %s does not match", archive)
	}
	exe, err := extract(data, "lotse")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		return err
	}
	tmp := binary + ".tmp"
	if err := os.WriteFile(tmp, exe, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, binary)
}

// verify checks that sums is signed with one of lotse's keys.
func verify(sums, sig []byte) error {
	keys := trustedKeys
	if extraKey != "" {
		keys = append(append([]string{}, keys...), extraKey)
	}
	for _, k := range keys {
		var pub minisign.PublicKey
		if err := pub.UnmarshalText([]byte(k)); err != nil {
			continue
		}
		if minisign.Verify(pub, sums, sig) {
			return nil
		}
	}
	return errors.New("the signature of checksums.txt does not match lotse's keys")
}

// checksum finds the SHA-256 of file in a checksums.txt.
func checksum(sums []byte, file string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && f[1] == file {
			return f[0], nil
		}
	}
	return "", fmt.Errorf("checksums.txt does not list %s", file)
}

// extract returns the file name from a .tar.gz.
func extract(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("the archive has no %s", name)
		}
		if err != nil {
			return nil, err
		}
		if h.Name == name && h.Typeflag == tar.TypeReg {
			return io.ReadAll(io.LimitReader(tr, maxArchive))
		}
	}
}

func (in *Installer) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	client := in.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s is larger than expected", url)
	}
	return b, nil
}

// switchTo makes version current and the version that was current, if it
// is good, previous: the one the starter goes back to.
func (in *Installer) switchTo(version string) error {
	s := Starter{Dir: in.Dir}
	if cur, err := os.Readlink(filepath.Join(in.Dir, "current")); err == nil {
		cur = filepath.Base(cur)
		if cur != version && !s.isBad(cur) {
			if err := link(in.Dir, "previous", cur); err != nil {
				return err
			}
		}
	}
	return link(in.Dir, "current", version)
}

// link points dir/name at target, replacing it at once.
func link(dir, name, target string) error {
	tmp := filepath.Join(dir, "."+name+".tmp")
	os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, name))
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
