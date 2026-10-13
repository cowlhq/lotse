package secret

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBox(t *testing.T) {
	b, err := NewBox("a key of some length")
	if err != nil {
		t.Fatal(err)
	}
	sealed := b.Seal("lotse_enr_xyz")
	if sealed == b.Seal("lotse_enr_xyz") {
		t.Error("sealing twice gives the same: no nonce")
	}
	if plain, err := b.Open(sealed); err != nil || plain != "lotse_enr_xyz" {
		t.Errorf("open: %q, %v", plain, err)
	}
	other, _ := NewBox("another key, also long")
	if _, err := other.Open(sealed); err == nil {
		t.Error("another key opened it")
	}
	if _, err := NewBox("short"); err == nil {
		t.Error("a short key was taken")
	}
}

func TestLoadKey(t *testing.T) {
	dir := t.TempDir()
	if k, _ := LoadKey("from the environment", dir); k != "from the environment" {
		t.Errorf("env: %q", k)
	}
	k1, err := LoadKey("", dir)
	if err != nil || len(k1) < 16 {
		t.Fatalf("made: %q, %v", k1, err)
	}
	info, _ := os.Stat(filepath.Join(dir, "secret.key"))
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info.Mode())
	}
	if k2, _ := LoadKey("", dir); k2 != k1 {
		t.Error("the key changed")
	}
}
