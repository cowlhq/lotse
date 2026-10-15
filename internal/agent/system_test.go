package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRebootRequired(t *testing.T) {
	root := t.TempDir()
	if req, _ := rebootRequired(root); req {
		t.Error("nothing there, and a restart is required")
	}
	// Debian and Ubuntu.
	os.MkdirAll(filepath.Join(root, "run"), 0o755)
	os.WriteFile(filepath.Join(root, "run/reboot-required"), []byte("*** System restart required ***\n"), 0o644)
	os.WriteFile(filepath.Join(root, "run/reboot-required.pkgs"), []byte("linux-image-6.8\nlibc6\nlinux-image-6.8\n"), 0o644)
	if req, why := rebootRequired(root); !req || why != "updated: libc6, linux-image-6.8" {
		t.Errorf("debian: %v %q", req, why)
	}

	// NixOS: another kernel switched to than booted.
	nix := t.TempDir()
	for _, sys := range []string{"booted-system", "current-system"} {
		os.MkdirAll(filepath.Join(nix, "run", sys), 0o755)
	}
	os.Symlink("/nix/store/aaa-linux-6.6/bzImage", filepath.Join(nix, "run/booted-system/kernel"))
	os.Symlink("/nix/store/aaa-linux-6.6/bzImage", filepath.Join(nix, "run/current-system/kernel"))
	if req, _ := rebootRequired(nix); req {
		t.Error("nixos, same kernel")
	}
	os.Remove(filepath.Join(nix, "run/current-system/kernel"))
	os.Symlink("/nix/store/bbb-linux-6.12/bzImage", filepath.Join(nix, "run/current-system/kernel"))
	if req, why := rebootRequired(nix); !req || why != "a new kernel is installed" {
		t.Errorf("nixos: %v %q", req, why)
	}
}

// Without a system bus, there is no systemd to ask; the rest still comes.
func TestReadSystem(t *testing.T) {
	s := readSystem(t.Context(), t.TempDir(), time.Unix(100, 0))
	if s.Systemd || s.TS != 100 {
		t.Errorf("%+v", s)
	}
}

// On a machine with systemd, its units.
func TestSystemdUnits(t *testing.T) {
	if _, err := os.Stat("/run/dbus/system_bus_socket"); err != nil {
		t.Skip("no system bus here")
	}
	units, err := systemdUnits(t.Context(), "/")
	if err != nil {
		t.Skipf("systemd does not answer here: %v", err)
	}
	if len(units) == 0 {
		t.Error("no units")
	}
}
