package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capture is a server that keeps what it was sent.
func capture(t *testing.T, status int) (*httptest.Server, *struct {
	path   string
	header http.Header
	body   map[string]any
}) {
	got := &struct {
		path   string
		header http.Header
		body   map[string]any
	}{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got.path, got.header = r.URL.Path, r.Header
		json.Unmarshal(raw, &got.body)
		w.WriteHeader(status)
		w.Write([]byte(`{"ok":false,"description":"Bad Request: chat not found"}`))
	}))
	t.Cleanup(server.Close)
	return server, got
}

func TestPush(t *testing.T) {
	m := Message{Hub: "https://lotse.example.com", Items: []Item{disk, down}}
	ctx := t.Context()

	server, got := capture(t, 200)
	if err := (Ntfy{Server: server.URL + "/", Topic: "lotse-ops", Token: "tk_x"}).Send(ctx, m); err != nil {
		t.Fatal(err)
	}
	if got.body["topic"] != "lotse-ops" || got.body["priority"] != 5.0 || got.header.Get("Authorization") != "Bearer tk_x" || !strings.Contains(got.body["message"].(string), "RESOLVED check.down") {
		t.Errorf("ntfy: %v %v", got.body, got.header)
	}

	if err := (Telegram{BotToken: "123:abc", ChatID: "-100", API: server.URL}).Send(ctx, m); err != nil {
		t.Fatal(err)
	}
	if got.path != "/bot123:abc/sendMessage" || got.body["chat_id"] != "-100" || !strings.Contains(got.body["text"].(string), `<a href="https://lotse.example.com/hosts/7">acme/shop/prod/web1 /data</a>`) {
		t.Errorf("telegram: %s %v", got.path, got.body)
	}

	if err := (Slack{URL: server.URL}).Send(ctx, m); err != nil || !strings.Contains(got.body["text"].(string), "*CRIT* host.disk on <https://lotse.example.com/hosts/7|acme/shop/prod/web1 /data>") {
		t.Errorf("slack: %v %v", err, got.body)
	}

	if err := (Discord{URL: server.URL}).Send(ctx, m); err != nil || !strings.Contains(got.body["content"].(string), `**CRIT** host.disk on [acme/shop/prod/web1 /data](<https://lotse.example.com/hosts/7>)`) {
		t.Errorf("discord: %v %v", err, got.body)
	}

	if err := (Pushover{Token: "app", User: "usr", API: server.URL}).Send(ctx, m); err != nil || got.path != "/1/messages.json" || got.body["priority"] != 1.0 || got.body["user"] != "usr" {
		t.Errorf("pushover: %v %s %v", err, got.path, got.body)
	}

	// A refusal says why, and never the bot's token in the address.
	bad, _ := capture(t, 400)
	err := (Telegram{BotToken: "123:secret", ChatID: "1", API: bad.URL}).Send(ctx, m)
	if err == nil || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "chat not found") {
		t.Errorf("telegram refused: %v", err)
	}
}

// Many alerts at once fit the service's limit, and say how many more there
// are.
func TestFit(t *testing.T) {
	var items []Item
	for range 100 {
		items = append(items, disk)
	}
	text := fit(Message{Hub: "https://lotse.example.com", Items: items}, 1000, nil, func(it Item) string { return label(it) + " " + where(it) + " " + Describe(it) })
	if len(text) > 1000 || !strings.Contains(text, "more at https://lotse.example.com") {
		t.Errorf("%d bytes: %s", len(text), text)
	}
	if got := truncate("ääää", 6); got != "ä…" {
		t.Errorf("truncate: %q", got)
	}
}
