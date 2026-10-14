package hub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/notify"
	"lotse.sh/internal/probe"
	"lotse.sh/internal/store"
)

// receiver is a webhook that keeps what it gets.
type receiver struct {
	mu     sync.Mutex
	got    []notify.WebhookBody
	failed bool
}

func (r *receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failed {
		http.Error(w, "down", http.StatusBadGateway)
		return
	}
	var b notify.WebhookBody
	json.NewDecoder(req.Body).Decode(&b)
	r.got = append(r.got, b)
}

func (r *receiver) take() []notify.WebhookBody {
	r.mu.Lock()
	defer r.mu.Unlock()
	got := r.got
	r.got = nil
	return got
}

// alertHub is a hub with a webhook channel "ops" that every alert goes to,
// and a heartbeat in acme/shop/prod. Its loops do not run; the test calls
// them, at the times it chooses.
func alertHub(t *testing.T) (*hub, *httptest.Server, *receiver, store.Check, string) {
	t.Helper()
	h, server := newTestHub(t)
	h.gather = time.Second
	h.alerts.started = time.Unix(0, 0)
	rec := &receiver{}
	hook := httptest.NewServer(rec)
	t.Cleanup(hook.Close)
	enrollmentToken(t, h)
	status, body := do(t, server, "POST", "/api/channels", fmt.Sprintf(
		`{"scope_id":1,"key":"ops","name":"Ops","type":"webhook","spec":{},"secrets":{"url":{"value":%q}}}`, hook.URL))
	if status != http.StatusCreated {
		t.Fatalf("channel: %d %s", status, body)
	}
	if status, body := do(t, server, "PUT", "/api/settings", `{"subject":"global:1","key":"notify.channels","value":["ops"]}`); status != http.StatusNoContent {
		t.Fatalf("notify.channels: %d %s", status, body)
	}
	envs, _ := h.db.Scopes(t.Context())
	var check api.Check
	status, body = do(t, server, "POST", "/api/checks", fmt.Sprintf(
		`{"environment_id":%d,"name":"Backup","type":"heartbeat","spec":{"period":"1h","grace":"10m"}}`, envs[len(envs)-1].ID))
	if status != http.StatusCreated {
		t.Fatalf("heartbeat: %d %s", status, body)
	}
	json.Unmarshal([]byte(body), &check)
	c, _ := h.db.Check(t.Context(), check.Id)
	return h, server, rec, c, strings.Replace(*check.PingUrl, h.URL(), server.URL, 1)
}

func ping(t *testing.T, url string) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
}

func step(t *testing.T, h *hub, now time.Time) {
	t.Helper()
	if err := h.evaluate(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	if err := h.deliver(t.Context(), now.Add(h.gather)); err != nil {
		t.Fatal(err)
	}
}

func TestAlertFlow(t *testing.T) {
	h, server, rec, check, pingURL := alertHub(t)
	now := time.Now()

	// The job says it failed: CRIT, sent once the second of gathering is
	// over.
	ping(t, pingURL+"/fail")
	if err := h.evaluate(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	h.deliver(t.Context(), now)
	if got := rec.take(); len(got) != 0 {
		t.Errorf("sent before gathering is over: %+v", got)
	}
	h.deliver(t.Context(), now.Add(time.Second))
	got := rec.take()
	if len(got) != 1 || len(got[0].Alerts) != 1 {
		t.Fatalf("sent: %+v", got)
	}
	a := got[0].Alerts[0]
	if a.Rule != "check.heartbeat" || a.State != "crit" || a.Subject != "acme/shop/prod/backup" || a.Reason != "the job said it failed" ||
		!strings.HasPrefix(a.URL, "http://lotse.example/alerts/") {
		t.Errorf("alert: %+v", a)
	}

	var firing []api.AlertState
	getJSON(t, server, "/api/alerts", &firing)
	if len(firing) != 1 || firing[0].State != "crit" || firing[0].SubjectPath != "acme/shop/prod/backup" || firing[0].NotifiedState != "crit" {
		t.Fatalf("firing: %+v", firing)
	}
	getJSON(t, server, fmt.Sprintf("/api/alerts?subject=check:%d", check.ID), &firing)
	if len(firing) != 1 {
		t.Errorf("firing on the check: %+v", firing)
	}
	getJSON(t, server, "/api/alerts?subject=customer:999", &firing)
	if len(firing) != 0 {
		t.Errorf("firing elsewhere: %+v", firing)
	}

	// Nothing new: nothing sent.
	step(t, h, now.Add(10*time.Second))
	if got := rec.take(); len(got) != 0 {
		t.Errorf("sent again: %+v", got)
	}

	// Why it fired, and where it went.
	var detail api.AlertDetail
	getJSON(t, server, fmt.Sprintf("/api/alerts/%d", firing0(t, server).AlertId), &detail)
	if detail.To != "crit" || len(detail.Notifications) != 1 || detail.Notifications[0].Status != "sent" || detail.Notifications[0].Channel != "ops" {
		t.Errorf("detail: %+v", detail)
	}
	channels := settingOf(detail.Settings, "notify.channels")
	if channels.From == nil || channels.From.Kind != "global" {
		t.Errorf("notify.channels: %+v", channels)
	}

	// The job ran: resolved.
	ping(t, pingURL)
	step(t, h, now.Add(20*time.Second))
	got = rec.take()
	if len(got) != 1 || got[0].Alerts[0].State != "ok" || got[0].Alerts[0].From != "crit" {
		t.Fatalf("resolved: %+v", got)
	}
	getJSON(t, server, "/api/alerts", &firing)
	if len(firing) != 0 {
		t.Errorf("firing after recovery: %+v", firing)
	}
	var history []api.Alert
	getJSON(t, server, fmt.Sprintf("/api/alerts/history?subject=check:%d&from=0&to=%d", check.ID, now.Add(time.Hour).Unix()), &history)
	if len(history) != 2 || history[0].To != "ok" || history[1].To != "crit" {
		t.Errorf("history: %+v", history)
	}
	if states, _ := h.db.AlertStates(t.Context()); len(states) != 0 {
		t.Errorf("states kept: %+v", states)
	}
}

func firing0(t *testing.T, server *httptest.Server) api.AlertState {
	t.Helper()
	var firing []api.AlertState
	getJSON(t, server, "/api/alerts", &firing)
	if len(firing) == 0 {
		t.Fatal("nothing fires")
	}
	return firing[0]
}

func settingOf(list []api.RuleSetting, key string) api.RuleSetting {
	for _, s := range list {
		if s.Key == key {
			return s
		}
	}
	return api.RuleSetting{}
}

func TestMaintenanceHoldsBack(t *testing.T) {
	h, server, rec, check, pingURL := alertHub(t)
	now := time.Now()
	status, body := do(t, server, "POST", "/api/maintenance", fmt.Sprintf(
		`{"subject":"environment:%d","starts_at":%d,"ends_at":%d,"reason":"moving the backup"}`,
		check.EnvironmentID, now.Add(-time.Minute).Unix(), now.Add(time.Hour).Unix()))
	if status != http.StatusCreated {
		t.Fatalf("maintenance: %d %s", status, body)
	}
	var m api.Maintenance
	json.Unmarshal([]byte(body), &m)
	if !m.Active || m.SubjectPath != "acme/shop/prod" {
		t.Errorf("window: %+v", m)
	}
	var windows []api.Maintenance
	getJSON(t, server, fmt.Sprintf("/api/maintenance?subject=check:%d", check.ID), &windows)
	if len(windows) != 1 {
		t.Errorf("windows over the check: %+v", windows)
	}

	ping(t, pingURL+"/fail")
	step(t, h, now)
	if got := rec.take(); len(got) != 0 {
		t.Errorf("sent during maintenance: %+v", got)
	}
	a := firing0(t, server)
	if a.State != "crit" || !a.InMaintenance || a.NotifiedState != "ok" {
		t.Errorf("firing in maintenance: %+v", a)
	}
	var detail api.AlertDetail
	getJSON(t, server, fmt.Sprintf("/api/alerts/%d", a.AlertId), &detail)
	if !detail.InMaintenance || len(detail.Notifications) != 1 || detail.Notifications[0].Status != "suppressed" {
		t.Errorf("held back: %+v", detail)
	}

	// The window ends; what still fires is sent.
	if status, _ := do(t, server, "DELETE", fmt.Sprintf("/api/maintenance/%d", m.Id), ""); status != http.StatusNoContent {
		t.Fatal("end the window")
	}
	step(t, h, now.Add(10*time.Second))
	if got := rec.take(); len(got) != 1 || got[0].Alerts[0].State != "crit" {
		t.Errorf("after maintenance: %+v", got)
	}

	// A weekly window, and a wrong one.
	for body, want := range map[string]int{
		`{"subject":"global:1","schedule":"sun 02:00-04:00"}`:                   http.StatusCreated,
		`{"subject":"global:1","schedule":"sunday at two"}`:                     http.StatusBadRequest,
		`{"subject":"global:1"}`:                                                http.StatusBadRequest,
		`{"subject":"host:999","schedule":"sun 02:00-04:00"}`:                   http.StatusNotFound,
		`{"subject":"global:1","schedule":"daily 01:00-02:00","starts_at":100}`: http.StatusBadRequest,
	} {
		if status, got := do(t, server, "POST", "/api/maintenance", body); status != want {
			t.Errorf("%s: %d %s", body, status, got)
		}
	}
}

func TestAcknowledge(t *testing.T) {
	h, server, rec, _, pingURL := alertHub(t)
	// notify.repeat is at least 5 minutes through the API; a second here.
	h.db.SetSetting(t.Context(), ada(), store.Subject{Kind: "global", ID: 1}, "notify.repeat", json.RawMessage(`"1s"`), 1)
	now := time.Now()
	ping(t, pingURL+"/fail")
	step(t, h, now)
	rec.take()
	step(t, h, now.Add(2*time.Second))
	got := rec.take()
	if len(got) != 1 || !got[0].Alerts[0].Repeat {
		t.Fatalf("repeated: %+v", got)
	}

	a := firing0(t, server)
	if status, body := do(t, server, "POST", fmt.Sprintf("/api/alerts/%d/acknowledge", a.AlertId), ""); status != http.StatusNoContent {
		t.Fatalf("acknowledge: %d %s", status, body)
	}
	if status, _ := do(t, server, "POST", fmt.Sprintf("/api/alerts/%d/acknowledge", a.AlertId), ""); status != http.StatusNotFound {
		t.Errorf("acknowledged twice: %d", status)
	}
	step(t, h, now.Add(4*time.Second))
	step(t, h, now.Add(6*time.Second))
	if got := rec.take(); len(got) != 0 {
		t.Errorf("repeated after acknowledging: %+v", got)
	}
	a = firing0(t, server)
	if a.AckedBy == nil || *a.AckedBy != "admin@example.com" {
		t.Errorf("acknowledged: %+v", a)
	}
	if states, _ := h.db.AlertStates(t.Context()); len(states) != 1 || states[0].AckedAt == nil {
		t.Errorf("acknowledgement not kept: %+v", states)
	}
}

func TestHostRules(t *testing.T) {
	h, server, rec, _, _ := alertHub(t)
	ctx := t.Context()
	envs, _ := h.db.Scopes(ctx)
	env := envs[len(envs)-1]
	now := time.Now()
	host, _, _ := h.db.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1", LastSeen: now.Unix()})
	h.db.SetHostEnvironment(ctx, ada(), host.ID, &env.ID)
	h.db.UpdateHost(ctx, store.Host{ID: host.ID, Hostname: "web1", Cores: 2, MemTotal: 1000, LastSeen: now.Unix()})
	h.db.Write(ctx, store.Batch{
		HostSamples: []store.HostSample{{HostID: host.ID, TS: now.Unix(), CPU: ptr(10.0), MemUsed: ptr[int64](100)}},
		FSSamples: []store.FSSample{
			{HostID: host.ID, Mount: "/", TS: now.Unix(), Used: 50, Total: 100},
			{HostID: host.ID, Mount: "/data", TS: now.Unix(), Used: 97, Total: 100},
		},
		Seen: map[int64]int64{host.ID: now.Unix()},
	})
	ids, _ := h.db.SyncContainers(ctx, host.ID, []store.Container{{Slug: "db", DockerID: "d1", Name: "shop-db-1", State: "running", Health: "unhealthy", Labels: map[string]string{}}}, now.Unix())
	if status, body := do(t, server, "PUT", "/api/settings", fmt.Sprintf(`{"subject":"host:%d","key":"rules.host.disk.for","value":"0s"}`, host.ID)); status != http.StatusNoContent {
		t.Fatalf("for: %d %s", status, body)
	}
	if status, body := do(t, server, "PUT", "/api/settings", fmt.Sprintf(`{"subject":"container:%d","key":"rules.container.unhealthy.for","value":"0s"}`, ids["d1"])); status != http.StatusNoContent {
		t.Fatalf("container setting: %d %s", status, body)
	}
	h.agents.add(nil, host.ID) // as if its agent were connected

	step(t, h, now)
	got := rec.take()
	if len(got) != 1 || len(got[0].Alerts) != 2 {
		t.Fatalf("sent: %+v", got)
	}
	byRule := map[string]notify.Item{}
	for _, a := range got[0].Alerts {
		byRule[a.Rule] = a
	}
	disk, unhealthy := byRule["host.disk"], byRule["container.unhealthy"]
	if disk.State != "crit" || disk.Instance != "/data" || *disk.Value != 97 || *disk.Threshold != 95 || disk.Subject != "acme/shop/prod/web1" {
		t.Errorf("disk: %+v", disk)
	}
	if unhealthy.State != "crit" || unhealthy.Subject != "acme/shop/prod/web1/db" || unhealthy.Value != nil {
		t.Errorf("unhealthy: %+v", unhealthy)
	}
	var detail api.AlertDetail
	var firing []api.AlertState
	getJSON(t, server, fmt.Sprintf("/api/alerts?subject=host:%d", host.ID), &firing)
	if len(firing) != 2 {
		t.Fatalf("firing on the host, and its container: %+v", firing)
	}
	for _, a := range firing {
		if a.Rule == "host.disk" {
			getJSON(t, server, fmt.Sprintf("/api/alerts/%d", a.AlertId), &detail)
		}
	}
	if s := settingOf(detail.Settings, "rules.host.disk.for"); s.From == nil || s.From.Kind != "host" || s.Value != "0s" {
		t.Errorf("for, set on the host: %+v", s)
	}
	if s := settingOf(detail.Settings, "rules.host.disk.crit"); s.From != nil || s.Value != float64(95) {
		t.Errorf("crit, the default: %+v", s)
	}

	// The agent goes: silent after 2 minutes, and the rest unknown, not
	// sent.
	h.agents.remove(nil)
	step(t, h, now.Add(10*time.Second))
	step(t, h, now.Add(2*time.Minute+10*time.Second))
	got = rec.take()
	if len(got) != 1 || len(got[0].Alerts) != 1 || got[0].Alerts[0].Rule != "host.silent" || got[0].Alerts[0].Reason != "the agent is not connected" {
		t.Fatalf("silent: %+v", got)
	}
	getJSON(t, server, fmt.Sprintf("/api/alerts?subject=host:%d", host.ID), &firing)
	states := map[string]string{}
	for _, a := range firing {
		states[a.Rule] = string(a.State)
	}
	if states["host.silent"] != "crit" || states["host.disk"] != "unknown" || states["container.unhealthy"] != "unknown" {
		t.Errorf("states: %+v", states)
	}
}

func TestChannelsAPI(t *testing.T) {
	h, server := newTestHub(t)
	t.Setenv("ACME_CHAT", "https://chat.example.com/hook")
	for _, c := range []struct {
		body string
		want int
	}{
		{`{"scope_id":1,"key":"chat","name":"Chat","type":"googlechat","spec":{},"secrets":{"url":{"env":"ACME_CHAT"}}}`, http.StatusCreated},
		{`{"scope_id":1,"key":"chat","name":"Again","type":"googlechat","spec":{},"secrets":{"url":{"env":"ACME_CHAT"}}}`, http.StatusConflict},
		{`{"scope_id":1,"key":"nourl","name":"No URL","type":"webhook","spec":{}}`, http.StatusBadRequest},
		{`{"scope_id":1,"key":"mine","name":"Mine","type":"webhook","spec":{},"secrets":{"url":{"env":"LOTSE_SECRET_KEY"}}}`, http.StatusBadRequest},
		{`{"scope_id":1,"key":"file","name":"File","type":"webhook","spec":{},"secrets":{"url":{"file":"/var/lib/lotse/secret.key"}}}`, http.StatusBadRequest},
		{`{"scope_id":1,"key":"mail","name":"Mail","type":"email","spec":{"to":["nobody"]}}`, http.StatusBadRequest},
		{`{"scope_id":1,"key":"mail","name":"Mail","type":"email","spec":{"to":["ops@example.com"]}}`, http.StatusCreated},
		{`{"scope_id":1,"key":"Bad Key","name":"x","type":"webhook","spec":{},"secrets":{"url":{"value":"https://x"}}}`, http.StatusBadRequest},
	} {
		if status, got := do(t, server, "POST", "/api/channels", c.body); status != c.want {
			t.Errorf("%s: %d %s", c.body, status, got)
		}
	}
	var list []api.Channel
	getJSON(t, server, "/api/channels", &list)
	if len(list) != 2 || list[0].Key != "chat" || !list[0].Secrets["url"].Set || ptrValue(list[0].Secrets["url"].Env) != "ACME_CHAT" {
		t.Fatalf("channels: %+v", list)
	}

	// Secrets go in, but never come out.
	status, body := do(t, server, "PATCH", fmt.Sprintf("/api/channels/%d", list[0].Id),
		`{"name":"Chat","spec":{},"secrets":{"url":{"value":"https://chat.googleapis.com/v1/spaces/x?key=s3cret"}}}`)
	if status != http.StatusOK || strings.Contains(body, "s3cret") {
		t.Errorf("update: %d %s", status, body)
	}
	if _, body := do(t, server, "GET", "/api/channels", ""); strings.Contains(body, "s3cret") {
		t.Errorf("a secret in the list: %s", body)
	}
	changes, _ := h.db.Changes(t.Context(), "channel", list[0].Id, 10)
	for _, c := range changes {
		if strings.Contains(string(c.New), "s3cret") || strings.Contains(string(c.Old), "s3cret") {
			t.Errorf("a secret in the history: %+v", c)
		}
	}

	// The email channel without a mail server cannot send; the test says so.
	var result api.TestResult
	_, body = do(t, server, "POST", fmt.Sprintf("/api/channels/%d/test", list[1].Id), "")
	json.Unmarshal([]byte(body), &result)
	if result.Sent || result.Error == nil || !strings.Contains(*result.Error, "no mail server") {
		t.Errorf("test without a mail server: %s", body)
	}

	if status, _ := do(t, server, "GET", "/api/mail-server", ""); status != http.StatusNotFound {
		t.Errorf("no mail server: %d", status)
	}
	status, body = do(t, server, "PUT", "/api/mail-server", `{"host":"127.0.0.1","port":1,"username":"lotse","from":"lotse@example.com","tls":"starttls","password":{"value":"hunter2"}}`)
	if status != http.StatusOK || strings.Contains(body, "hunter2") || !strings.Contains(body, `"password":{"set":true}`) {
		t.Errorf("mail server: %d %s", status, body)
	}
	_, body = do(t, server, "POST", "/api/mail-server/test", `{"to":"ops@example.com"}`)
	json.Unmarshal([]byte(body), &result)
	if result.Sent || result.Error == nil {
		t.Errorf("test to a closed port: %s", body)
	}
	// Keeping the password, changing the rest.
	do(t, server, "PUT", "/api/mail-server", `{"host":"smtp.example.com","port":587,"from":"lotse@example.com","tls":"starttls"}`)
	m, _ := h.mailServer(t.Context())
	if m.Host != "smtp.example.com" || m.Password != "hunter2" {
		t.Errorf("mail server kept: %+v", m)
	}
}

// check.down fires after two failed runs, from the probe's real results.
func TestCheckDown(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			http.Error(w, "broken", http.StatusInternalServerError)
		}
	}))
	defer target.Close()
	h, server, rec, check, _ := alertHub(t)
	h.checkInterval = 100 * time.Millisecond
	status, body := do(t, server, "POST", "/api/checks", fmt.Sprintf(
		`{"environment_id":%d,"name":"Shop","type":"http","spec":{"url":%q}}`, check.EnvironmentID, target.URL))
	if status != http.StatusCreated {
		t.Fatalf("check: %d %s", status, body)
	}
	defer h.builtinProbe.Stop()
	state := func() string {
		h.evaluate(t.Context(), time.Now())
		h.alerts.mu.Lock()
		defer h.alerts.mu.Unlock()
		for k, st := range h.alerts.states {
			if k.Rule == "check.down" {
				return st.State
			}
		}
		return "ok"
	}
	waitFor(t, "check.down to fire", func() bool { return state() == "crit" })
	h.deliver(t.Context(), time.Now().Add(h.gather))
	got := rec.take()
	if len(got) != 1 || got[0].Alerts[0].Rule != "check.down" || *got[0].Alerts[0].Value < 2 || !strings.Contains(got[0].Alerts[0].Reason, "500") {
		t.Fatalf("sent: %+v", got)
	}
	failing.Store(false)
	waitFor(t, "check.down to clear", func() bool { return state() == "ok" })
}

// After a restart, failures go on counting from the results kept.
func TestFailuresAfterRestart(t *testing.T) {
	h, _ := newTestHub(t)
	env := testEnvironment(t, h)
	c, err := h.db.CreateCheck(t.Context(), ada(), store.Check{EnvironmentID: env.ID, Slug: "x", Name: "X", Type: "tcp", Spec: json.RawMessage(`{"address":"127.0.0.1:1"}`), CreatedAt: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	h.db.Write(t.Context(), store.Batch{CheckResults: []store.CheckResult{
		{CheckID: c.ID, ProbeID: store.BuiltinProbe, TS: now.Unix() - 120, OK: true},
		{CheckID: c.ID, ProbeID: store.BuiltinProbe, TS: now.Unix() - 60, OK: false},
	}})
	h.checks.assigned = map[int64]assignment{c.ID: {Assignment: probe.Assignment{CheckID: c.ID, Interval: time.Minute, Timeout: time.Second}, probes: []int64{store.BuiltinProbe}}}
	h.checkResult(t.Context(), store.BuiltinProbe, c.ID, now, probe.Result{OK: false, Error: "refused"})
	if st := h.checkStanding(c.ID, now); st.failed != 2 {
		t.Errorf("failed in a row: %d, want 2", st.failed)
	}
}
