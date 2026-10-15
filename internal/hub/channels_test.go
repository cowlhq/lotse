package hub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lotse.sh/internal/apply"
	"lotse.sh/internal/hub/api"
)

// Chat and push channels: what each needs, a test message through ntfy,
// and the way to lotse.yaml and back.
func TestPushChannels(t *testing.T) {
	h, server := newTestHub(t)
	var got map[string]any
	ntfy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &got)
	}))
	defer ntfy.Close()
	for _, c := range []struct {
		body string
		want int
	}{
		{`{"scope_id":1,"key":"slack","name":"Slack","type":"slack","spec":{},"secrets":{"url":{"value":"https://hooks.slack.com/services/x"}}}`, http.StatusCreated},
		{`{"scope_id":1,"key":"discord","name":"Discord","type":"discord","spec":{},"secrets":{"url":{"value":"ftp://x"}}}`, http.StatusBadRequest},
		{`{"scope_id":1,"key":"tg","name":"Telegram","type":"telegram","spec":{},"secrets":{"bot_token":{"value":"1:a"}}}`, http.StatusBadRequest},
		{`{"scope_id":1,"key":"tg","name":"Telegram","type":"telegram","spec":{"chat_id":"-100"},"secrets":{"bot_token":{"value":"1:a"}}}`, http.StatusCreated},
		{`{"scope_id":1,"key":"push","name":"Push","type":"pushover","spec":{},"secrets":{"token":{"value":"app"}}}`, http.StatusBadRequest},
		{`{"scope_id":1,"key":"push","name":"Push","type":"pushover","spec":{},"secrets":{"token":{"value":"app"},"user":{"value":"usr"}}}`, http.StatusCreated},
		{fmt.Sprintf(`{"scope_id":1,"key":"ntfy","name":"Phone","type":"ntfy","spec":{"server":%q},"secrets":{"topic":{"value":"lotse-x"}}}`, ntfy.URL), http.StatusCreated},
		{`{"scope_id":1,"key":"sms","name":"SMS","type":"sms","spec":{}}`, http.StatusBadRequest},
	} {
		if status, body := do(t, server, "POST", "/api/channels", c.body); status != c.want {
			t.Errorf("%s: %d %s", c.body, status, body)
		}
	}
	var list []api.Channel
	getJSON(t, server, "/api/channels", &list)
	byKey := map[string]api.Channel{}
	for _, c := range list {
		byKey[c.Key] = c
	}
	var result api.TestResult
	_, body := do(t, server, "POST", fmt.Sprintf("/api/channels/%d/test", byKey["ntfy"].Id), "")
	json.Unmarshal([]byte(body), &result)
	if !result.Sent || got["topic"] != "lotse-x" {
		t.Errorf("ntfy test: %s, %v", body, got)
	}

	// Exported, the secrets are kept by reference; the rest is as it was.
	stored, _ := h.db.Channels(t.Context())
	for _, c := range stored {
		out := h.exportChannel(c)
		typ, raw, secrets, err := channelOf(out)
		if err != nil || typ != c.Type {
			t.Errorf("%s: %s, %v", c.Slug, typ, err)
		}
		for field, s := range secrets {
			if s.Keep != c.Slug {
				t.Errorf("%s: %s is %+v", c.Slug, field, s)
			}
		}
		if c.Type == "telegram" && raw["chat_id"] != "-100" {
			t.Errorf("telegram: %v", raw)
		}
	}
	y, _ := apply.Marshal(apply.Document{Channels: []apply.Channel{h.exportChannel(stored[0])}})
	if strings.Contains(string(y), "hooks.slack.com") {
		t.Errorf("a secret in YAML: %s", y)
	}
}
