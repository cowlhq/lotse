package hub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/protocol"
	"lotse.sh/internal/store"
)

func TestClockEstimate(t *testing.T) {
	for _, c := range []struct {
		name    string
		delays  []int64
		offset  int64
		settled bool
	}{
		{"right", []int64{0, 1, 0, 2, 1}, 0, true},
		{"too few", []int64{600, 600}, 0, false},
		{"behind", []int64{601, 600, 602, 600, 601}, 600, true},
		{"ahead", []int64{-300, -299, -300, -298, -299}, -298, true},
		{"slightly off", []int64{20, 21, 20, 22, 20}, 0, true},
		// Sent from the buffer after a reconnect: late, not off.
		{"catching up", []int64{900, 885, 870, 855, 840}, 0, false},
	} {
		var e clockEstimate
		var offset int64
		var settled bool
		for _, d := range c.delays {
			offset, settled = e.observe(1000-d, 1000)
		}
		if offset != c.offset || settled != c.settled {
			t.Errorf("%s: %d, %v; want %d, %v", c.name, offset, settled, c.offset, c.settled)
		}
	}
}

// An agent whose clock is ten minutes behind: its readings land at the
// hub's time, and its host says how far off it is.
func TestClockBehind(t *testing.T) {
	h, server := newTestHub(t)
	token := enrollmentToken(t, h)
	status, body := do(t, server, "POST", protocol.EnrollPath, `{"token":"`+token+`","hostname":"web1","machine_id":"m1"}`)
	var e protocol.EnrollResponse
	json.Unmarshal([]byte(body), &e)
	if status != http.StatusOK {
		t.Fatalf("enroll: %d %s", status, body)
	}
	conn, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http")+protocol.ConnectPath, &websocket.DialOptions{
		Subprotocols: protocol.Subprotocols(),
		HTTPHeader:   http.Header{"Authorization": {"Bearer " + e.Credential}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	wsjson.Write(t.Context(), conn, protocol.Message{Type: protocol.TypeHello, Hello: &protocol.Hello{Hostname: "web1", MachineID: "m1"}})
	var welcome protocol.Message
	if err := wsjson.Read(t.Context(), conn, &welcome); err != nil {
		t.Fatal(err)
	}
	cpu := 10.0
	for i := range 6 {
		ts := time.Now().Unix() - 600 + int64(i)
		wsjson.Write(t.Context(), conn, protocol.Message{Type: protocol.TypeHost, Host: &protocol.HostSample{TS: ts, CPU: &cpu}})
	}
	hosts, _ := h.db.Hosts(t.Context())
	id := hosts[0].ID
	waitFor(t, "the clock to be known", func() bool { return h.clocks.get(id) >= 595 })
	waitFor(t, "a reading at the hub's time", func() bool {
		s, err := h.db.Series(t.Context(), "host", store.Raw, id, time.Now().Unix()-60, time.Now().Unix()+60, 1)
		return err == nil && len(s.Time) > 0
	})
	var host struct {
		Host struct {
			ClockOffset *int64 `json:"clock_offset"`
		}
	}
	getJSON(t, server, fmt.Sprintf("/api/hosts/%d", id), &host)
	if host.Host.ClockOffset == nil || *host.Host.ClockOffset < 595 {
		t.Errorf("clock offset: %v", host.Host.ClockOffset)
	}
}
