package hub

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/protocol"
)

// A machine with a failed unit, a service it must run that is not there,
// and updates that wait for a restart.
func TestHostSystem(t *testing.T) {
	h, server := newTestHub(t)
	conn := rawAgent(t, h, server)
	hosts, _ := h.db.Hosts(t.Context())
	id := hosts[0].ID
	for _, set := range []string{
		fmt.Sprintf(`{"subject":"host:%d","key":"host.services","value":["nginx.service","ssh.service"]}`, id),
		fmt.Sprintf(`{"subject":"host:%d","key":"host.ignore_units","value":["wait-online.service"]}`, id),
		// At once, not after minutes and a day.
		`{"subject":"global:1","key":"rules.host.service.for","value":"0s"}`,
		`{"subject":"global:1","key":"rules.host.reboot.for","value":"0s"}`,
	} {
		if status, body := do(t, server, "PUT", "/api/settings", set); status != http.StatusNoContent {
			t.Fatalf("%s: %d %s", set, status, body)
		}
	}
	send := func(units []protocol.Unit, reboot bool) {
		cpu := 1.0
		now := time.Now().Unix()
		wsjson.Write(t.Context(), conn, protocol.Message{Type: protocol.TypeHost, Host: &protocol.HostSample{TS: now, CPU: &cpu}})
		wsjson.Write(t.Context(), conn, protocol.Message{Type: protocol.TypeSystem, System: &protocol.System{
			TS: now, Systemd: true, Units: units, RebootRequired: reboot, RebootReason: "updated: libc6"}})
	}
	send([]protocol.Unit{{Name: "ssh.service", Active: "active", Sub: "running"}, {Name: "backup.service", Active: "failed", Sub: "failed"}, {Name: "wait-online.service", Active: "failed"}}, true)
	waitFor(t, "the system to be kept", func() bool {
		x, _ := h.db.Host(t.Context(), id)
		return x.System != ""
	})
	h.writer.flush(t.Context())

	step(t, h, time.Now())
	firing := map[string]string{}
	for key, st := range h.alerts.states {
		firing[key.Rule+" "+key.Instance] = st.State
	}
	for want, state := range map[string]string{"host.unit backup.service": "warn", "host.service nginx.service": "crit", "host.reboot ": "warn"} {
		if firing[want] != state {
			t.Errorf("%s: %q, want %q (all: %v)", want, firing[want], state, firing)
		}
	}
	if firing["host.service ssh.service"] == "crit" || firing["host.unit wait-online.service"] != "" {
		t.Errorf("fires what it should not: %v", firing)
	}

	// The unit runs again, nginx is there, the machine restarted.
	send([]protocol.Unit{{Name: "ssh.service", Active: "active"}, {Name: "nginx.service", Active: "active"}, {Name: "backup.service", Active: "inactive", Sub: "dead"}}, false)
	waitFor(t, "the new system", func() bool {
		x, _ := h.db.Host(t.Context(), id)
		s, _ := systemOf(x)
		return !s.RebootRequired
	})
	h.writer.flush(t.Context())
	step(t, h, time.Now())
	for key, st := range h.alerts.states {
		if st.State != "ok" && (key.Rule == "host.unit" || key.Rule == "host.service" || key.Rule == "host.reboot") {
			t.Errorf("%s %s still %s", key.Rule, key.Instance, st.State)
		}
	}

	var d struct {
		Host struct {
			RebootRequired *bool `json:"reboot_required"`
		}
		System struct{ Units []protocol.Unit }
	}
	getJSON(t, server, fmt.Sprintf("/api/hosts/%d", id), &d)
	if d.Host.RebootRequired != nil || len(d.System.Units) != 3 {
		t.Errorf("after the restart: %v, %+v", d.Host.RebootRequired, d.System.Units)
	}
}
