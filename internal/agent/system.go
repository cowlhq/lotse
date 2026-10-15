package agent

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"lotse.sh/internal/protocol"
)

// The machine's services and restarts. The agent asks systemd over D-Bus,
// as systemctl would, and reads what the package manager leaves behind; it
// runs nothing.

const systemEvery = time.Minute

// sampleSystem reads services and restarts every minute, connected or not.
// Only the newest reading waits to be sent.
func (a *agent) sampleSystem(ctx context.Context) {
	for {
		s := readSystem(ctx, a.cfg.Root, time.Now())
		a.out.addLatest(protocol.Message{Type: protocol.TypeSystem, System: &s})
		select {
		case <-ctx.Done():
			return
		case <-time.After(systemEvery):
		}
	}
}

func readSystem(ctx context.Context, root string, now time.Time) protocol.System {
	s := protocol.System{TS: now.Unix()}
	if units, err := systemdUnits(ctx, root); err == nil {
		s.Systemd, s.Units = true, units
	}
	s.RebootRequired, s.RebootReason = rebootRequired(root)
	return s
}

// systemdUnits asks systemd for its units: every service, and units of
// other kinds only when they failed.
func systemdUnits(ctx context.Context, root string) ([]protocol.Unit, error) {
	conn, err := dbus.Dial("unix:path="+filepath.Join(root, "run/dbus/system_bus_socket"), dbus.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.Auth(nil); err != nil {
		return nil, err
	}
	if err := conn.Hello(); err != nil {
		return nil, err
	}
	var list []struct {
		Name, Description, Load, Active, Sub, Following string
		Path                                            dbus.ObjectPath
		JobID                                           uint32
		JobType                                         string
		JobPath                                         dbus.ObjectPath
	}
	call := conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1").
		CallWithContext(ctx, "org.freedesktop.systemd1.Manager.ListUnits", 0)
	if err := call.Store(&list); err != nil {
		return nil, err
	}
	var units []protocol.Unit
	for _, u := range list {
		if strings.HasSuffix(u.Name, ".service") && u.Load == "loaded" || u.Active == "failed" {
			units = append(units, protocol.Unit{Name: u.Name, Active: u.Active, Sub: u.Sub})
		}
	}
	slices.SortFunc(units, func(a, b protocol.Unit) int { return strings.Compare(a.Name, b.Name) })
	return units, nil
}

// rebootRequired says whether updates wait for a restart: Debian and
// Ubuntu leave a file for it, with the packages that asked; on NixOS, the
// system switched to has another kernel than the one booted.
func rebootRequired(root string) (bool, string) {
	for _, dir := range []string{"run", "var/run"} {
		if _, err := os.Stat(filepath.Join(root, dir, "reboot-required")); err == nil {
			pkgs, _ := os.ReadFile(filepath.Join(root, dir, "reboot-required.pkgs"))
			list := slices.Compact(slices.Sorted(slices.Values(strings.Fields(string(pkgs)))))
			if len(list) > 0 {
				return true, "updated: " + strings.Join(list, ", ")
			}
			return true, ""
		}
	}
	for _, part := range []string{"kernel", "initrd", "kernel-modules"} {
		booted, err1 := os.Readlink(filepath.Join(root, "run/booted-system", part))
		current, err2 := os.Readlink(filepath.Join(root, "run/current-system", part))
		if err1 == nil && err2 == nil && booted != current {
			return true, "a new " + part + " is installed"
		}
	}
	return false, ""
}
