package hub

import (
	"path/filepath"
	"testing"

	"lotse.sh/internal/store"
)

// A backup is a database that opens, and is not written over.
func TestBackup(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{DataDir: dir, DatabaseURL: filepath.Join(dir, "lotse.db")}
	db, err := store.Open(t.Context(), cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	db.SaveHost(t.Context(), store.Host{MachineID: "m1", Hostname: "web1", LastSeen: 1})
	defer db.Close()

	path := filepath.Join(t.TempDir(), "backup.db")
	if err := Backup(t.Context(), cfg, path); err != nil {
		t.Fatal(err)
	}
	copy, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	if hosts, err := copy.Hosts(t.Context()); err != nil || len(hosts) != 1 {
		t.Errorf("hosts in the backup: %v, %v", hosts, err)
	}
	if err := Backup(t.Context(), cfg, path); err == nil {
		t.Error("a backup wrote over a file")
	}
}
