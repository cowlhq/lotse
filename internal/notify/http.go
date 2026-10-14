package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lotse.sh/internal/version"
)

var client = &http.Client{Timeout: 15 * time.Second}

// post sends a JSON body, and takes any 2xx as success.
func post(ctx context.Context, url string, body []byte, header http.Header) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("the address is not valid")
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "lotse/"+version.Version)
	for k, v := range header {
		req.Header[k] = v
	}
	res, err := client.Do(req)
	if err != nil {
		return plain(err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		text := strings.Join(strings.Fields(string(b)), " ")
		if text != "" {
			return fmt.Errorf("the server answered %s: %s", res.Status, text)
		}
		return fmt.Errorf("the server answered %s", res.Status)
	}
	io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
	return nil
}

// GoogleChat posts messages to a Google Chat space through its webhook.
type GoogleChat struct {
	URL string
}

func (g GoogleChat) Send(ctx context.Context, m Message) error {
	var b strings.Builder
	if m.Test {
		b.WriteString(m.Text())
	}
	for i, it := range m.Items {
		if i > 0 {
			b.WriteString("\n")
		}
		// Google Chat's own markup: *bold*, <url|text>.
		fmt.Fprintf(&b, "*%s* %s on <%s|%s>\n%s\n", label(it), it.Rule, it.URL, where(it), Describe(it))
	}
	body, _ := json.Marshal(map[string]string{"text": b.String()})
	return post(ctx, g.URL, body, nil)
}

// Webhook posts messages as JSON:
//
//	{"hub": "https://lotse.example.com", "test": false, "alerts": [Item, …]}
//
// With a signing key, X-Lotse-Signature is "sha256=" and the hex HMAC-SHA256
// of X-Lotse-Timestamp, a dot, and the body, so that the receiver can tell
// it comes from the hub, and is not an old one sent again.
type Webhook struct {
	URL        string
	SigningKey string
}

// WebhookBody is what a webhook receives.
type WebhookBody struct {
	Hub    string `json:"hub"`
	Test   bool   `json:"test"`
	Alerts []Item `json:"alerts"`
}

func (w Webhook) Send(ctx context.Context, m Message) error {
	items := m.Items
	if items == nil {
		items = []Item{}
	}
	body, err := json.Marshal(WebhookBody{Hub: m.Hub, Test: m.Test, Alerts: items})
	if err != nil {
		return err
	}
	header := http.Header{}
	if w.SigningKey != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		header.Set("X-Lotse-Timestamp", ts)
		header.Set("X-Lotse-Signature", "sha256="+Sign(w.SigningKey, ts, body))
	}
	return post(ctx, w.URL, body, header)
}

// Sign is the signature of a webhook's body at a time.
func Sign(key, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Ping calls a dead man's switch, such as a healthchecks.io check.
func Ping(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("the address is not valid")
	}
	req.Header.Set("User-Agent", "lotse/"+version.Version)
	res, err := client.Do(req)
	if err != nil {
		return plain(err)
	}
	defer res.Body.Close()
	io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("the server answered %s", res.Status)
	}
	return nil
}
