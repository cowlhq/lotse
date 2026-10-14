package notify

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func f(v float64) *float64 { return &v }

var disk = Item{Rule: "host.disk", Subject: "acme/shop/prod/web1", SubjectKind: "host", Instance: "/data", From: "ok", State: "crit",
	Since: 1791900000, Value: f(96.24), Threshold: f(95), Unit: "percent", URL: "https://lotse.example.com/hosts/7"}
var down = Item{Rule: "check.down", Subject: "acme/shop/prod/homepage", SubjectKind: "check", From: "crit", State: "ok",
	Since: 1791900000, URL: "https://lotse.example.com/checks/3"}

func TestText(t *testing.T) {
	one := Message{Hub: "https://lotse.example.com", Items: []Item{disk}}
	if got := one.Title(); got != "[lotse] CRIT host.disk on acme/shop/prod/web1 /data" {
		t.Errorf("title: %s", got)
	}
	if got := Describe(disk); got != "Now 96.2 %, threshold 95 %." {
		t.Errorf("describe: %s", got)
	}
	if got := Describe(down); got != "Back to normal, after CRIT." {
		t.Errorf("describe recovery: %s", got)
	}
	reason := Item{Rule: "check.down", State: "crit", Value: f(2), Threshold: f(2), Unit: "count", Reason: "connection refused"}
	if got := Describe(reason); got != "Now 2, threshold 2. connection refused." {
		t.Errorf("describe with a reason: %s", got)
	}
	two := Message{Items: []Item{disk, down}}
	if got := two.Title(); got != "[lotse] 2 alerts: 1 CRIT, 1 OK" {
		t.Errorf("title of two: %s", got)
	}
	if text := two.Text(); !strings.Contains(text, "RESOLVED check.down on acme/shop/prod/homepage\n") || !strings.Contains(text, "https://lotse.example.com/hosts/7\n") {
		t.Errorf("text: %s", text)
	}
	for v, want := range map[float64]string{1: "1 day", 6.5: "6.5 days"} {
		if got := Format(v, "days"); got != want {
			t.Errorf("format %v days: %s", v, got)
		}
	}
}

func TestWebhook(t *testing.T) {
	var got WebhookBody
	var header http.Header
	var raw []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		header = r.Header
		json.Unmarshal(raw, &got)
	}))
	defer server.Close()
	w := Webhook{URL: server.URL + "/hook?secret=s3cret", SigningKey: "key"}
	if err := w.Send(t.Context(), Message{Hub: "https://lotse.example.com", Items: []Item{disk}}); err != nil {
		t.Fatal(err)
	}
	if got.Hub != "https://lotse.example.com" || len(got.Alerts) != 1 || got.Alerts[0].Instance != "/data" || *got.Alerts[0].Value != 96.24 {
		t.Errorf("body: %+v", got)
	}
	ts := header.Get("X-Lotse-Timestamp")
	if _, err := strconv.ParseInt(ts, 10, 64); err != nil || header.Get("X-Lotse-Signature") != "sha256="+Sign("key", ts, raw) {
		t.Errorf("signature: %v", header)
	}

	// Errors say what went wrong, but not the address, which is secret.
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such space", http.StatusNotFound)
	}))
	defer failing.Close()
	err := GoogleChat{URL: failing.URL + "/v1/spaces/x?key=s3cret"}.Send(t.Context(), Message{Test: true})
	if err == nil || err.Error() != "the server answered 404 Not Found: no such space" {
		t.Errorf("error: %v", err)
	}
	failing.Close()
	err = Webhook{URL: failing.URL + "/hook?secret=s3cret"}.Send(t.Context(), Message{Test: true})
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Errorf("error with the address: %v", err)
	}
}

func TestGoogleChat(t *testing.T) {
	var got map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
	}))
	defer server.Close()
	if err := (GoogleChat{URL: server.URL}).Send(t.Context(), Message{Items: []Item{disk, down}}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got["text"], "*CRIT* host.disk on <https://lotse.example.com/hosts/7|acme/shop/prod/web1 /data>\nNow 96.2 %") {
		t.Errorf("text: %q", got["text"])
	}
}

// fakeSMTP is a mail server that takes one message, without encryption or
// login.
func fakeSMTP(t *testing.T, starttls bool) (port int, mail chan string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	mail = make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		say := func(s string) { conn.Write([]byte(s + "\r\n")) }
		say("220 fake ESMTP")
		var data strings.Builder
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				if starttls {
					say("250-fake")
					say("250 STARTTLS")
				} else {
					say("250 fake")
				}
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				data.WriteString(strings.TrimSpace(line) + "\n")
				say("250 ok")
			case cmd == "DATA":
				say("354 go on")
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
					data.WriteString(l)
				}
				mail <- data.String()
				say("250 queued")
			case cmd == "QUIT":
				say("221 bye")
				return
			default:
				say("502 no")
			}
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, mail
}

func TestEmail(t *testing.T) {
	port, mail := fakeSMTP(t, false)
	e := Email{Server: SMTP{Host: "127.0.0.1", Port: port, From: "lotse <lotse@example.com>", TLS: "none"}, To: []string{"ops@example.com", "Ada <ada@example.com>"}}
	if err := e.Server.Check(); err != nil {
		t.Fatal(err)
	}
	if err := e.Send(t.Context(), Message{Items: []Item{disk}}); err != nil {
		t.Fatal(err)
	}
	got := <-mail
	for _, want := range []string{"MAIL FROM:<lotse@example.com>", "RCPT TO:<ops@example.com>", "RCPT TO:<ada@example.com>",
		"Subject: [lotse] CRIT host.disk on acme/shop/prod/web1 /data\r\n", "To: ops@example.com, Ada <ada@example.com>\r\n",
		"Content-Transfer-Encoding: quoted-printable", "Now 96.2 %, threshold 95 %."} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}

	// STARTTLS asked for, but not offered: refused, rather than sent in
	// the clear.
	port, _ = fakeSMTP(t, false)
	e.Server.Port, e.Server.TLS = port, "starttls"
	if err := e.Send(t.Context(), Message{Test: true}); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("without STARTTLS: %v", err)
	}

	for _, bad := range []SMTP{{Port: 587, From: "a@b.c", TLS: "starttls"}, {Host: "x", Port: 587, From: "nobody", TLS: "starttls"}, {Host: "x", Port: 587, From: "a@b.c", TLS: "ssl"}} {
		if bad.Check() == nil {
			t.Errorf("%+v is fine", bad)
		}
	}
	if CheckAddresses(nil) == nil || CheckAddresses([]string{"ops"}) == nil || CheckAddresses([]string{"ops@example.com"}) != nil {
		t.Error("CheckAddresses")
	}
}
