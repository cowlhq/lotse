package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Chat and push services. Each gets the same message in its own markup,
// within its own limit of length.

// fit writes the items one by one, as long as they fit in limit bytes, and
// says how many more there are at the hub.
func fit(m Message, limit int, test func() string, item func(Item) string) string {
	if m.Test {
		return test()
	}
	var b strings.Builder
	for i, it := range m.Items {
		s := item(it)
		if i > 0 {
			s = "\n\n" + s
		}
		more := fmt.Sprintf("\n\n… and %d more at %s", len(m.Items)-i, m.Hub)
		if b.Len()+len(s) > limit-len(more) && i > 0 {
			b.WriteString(more)
			break
		}
		b.WriteString(s)
	}
	return truncate(b.String(), limit)
}

// truncate cuts s to at most n bytes, on a rune's edge.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n-len("…")]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "…"
}

// worst is the state of the worst item: what decides how loud a push is.
func worst(m Message) string {
	w := "ok"
	for _, it := range m.Items {
		switch {
		case it.State == "crit":
			return "crit"
		case it.State == "warn":
			w = "warn"
		}
	}
	return w
}

func testText(m Message) string {
	return fmt.Sprintf("This is a test message from lotse at %s. The channel works.", m.Hub)
}

// Ntfy publishes to a topic on an ntfy server, such as ntfy.sh, which
// pushes it to phones. The topic is a secret: who knows it can read it.
type Ntfy struct {
	Server string
	Topic  string
	// Token is an access token, for a server that asks for one.
	Token string
}

func (n Ntfy) Send(ctx context.Context, m Message) error {
	priority, tags := 3, []string{"warning"}
	switch worst(m) {
	case "crit":
		priority, tags = 5, []string{"rotating_light"}
	case "ok":
		priority, tags = 2, []string{"white_check_mark"}
	}
	if m.Test {
		priority, tags = 3, []string{"wave"}
	}
	body := map[string]any{
		"topic":    n.Topic,
		"title":    m.Title(),
		"message":  fit(m, 4000, func() string { return testText(m) }, func(it Item) string { return label(it) + " " + it.Rule + " on " + where(it) + "\n" + Describe(it) }),
		"priority": priority,
		"tags":     tags,
	}
	if len(m.Items) == 1 {
		body["click"] = m.Items[0].URL
	}
	b, _ := json.Marshal(body)
	header := http.Header{}
	if n.Token != "" {
		header.Set("Authorization", "Bearer "+n.Token)
	}
	return post(ctx, strings.TrimSuffix(n.Server, "/"), b, header)
}

// Telegram sends through a bot to a chat, a group or a channel.
type Telegram struct {
	BotToken string
	ChatID   string
	// API is Telegram's address; tests change it.
	API string
}

func (t Telegram) Send(ctx context.Context, m Message) error {
	text := fit(m, 4000, func() string { return html.EscapeString(testText(m)) }, func(it Item) string {
		return fmt.Sprintf("<b>%s</b> %s on <a href=\"%s\">%s</a>\n%s",
			label(it), html.EscapeString(it.Rule), html.EscapeString(it.URL), html.EscapeString(where(it)), html.EscapeString(Describe(it)))
	})
	b, _ := json.Marshal(map[string]any{
		"chat_id": t.ChatID, "text": text, "parse_mode": "HTML", "disable_web_page_preview": true,
	})
	api := t.API
	if api == "" {
		api = "https://api.telegram.org"
	}
	// The bot's token is in the address; post keeps addresses out of its
	// errors.
	return post(ctx, api+"/bot"+t.BotToken+"/sendMessage", b, nil)
}

// Slack posts to a channel through an incoming webhook.
type Slack struct {
	URL string
}

// slackEscape escapes what Slack's markup reads as its own.
var slackEscape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func (s Slack) Send(ctx context.Context, m Message) error {
	text := fit(m, 3500, func() string { return testText(m) }, func(it Item) string {
		return fmt.Sprintf("*%s* %s on <%s|%s>\n%s", label(it), slackEscape.Replace(it.Rule), it.URL, slackEscape.Replace(where(it)), slackEscape.Replace(Describe(it)))
	})
	b, _ := json.Marshal(map[string]string{"text": text})
	return post(ctx, s.URL, b, nil)
}

// Discord posts to a channel through a webhook.
type Discord struct {
	URL string
}

// discordEscape keeps names from turning into Discord's markup.
var discordEscape = strings.NewReplacer("*", "\\*", "_", "\\_", "~", "\\~", "`", "\\`", "|", "\\|", "[", "\\[", "]", "\\]")

func (d Discord) Send(ctx context.Context, m Message) error {
	text := fit(m, 1900, func() string { return testText(m) }, func(it Item) string {
		return fmt.Sprintf("**%s** %s on [%s](<%s>)\n%s", label(it), discordEscape.Replace(it.Rule), discordEscape.Replace(where(it)), it.URL, discordEscape.Replace(Describe(it)))
	})
	b, _ := json.Marshal(map[string]any{"content": text, "allowed_mentions": map[string]any{"parse": []string{}}})
	return post(ctx, d.URL, b, nil)
}

// Pushover pushes to a user's or a group's devices. Critical alerts come
// with high priority, which sounds even in quiet hours.
type Pushover struct {
	Token string // the application's
	User  string // the user's or group's key
	// API is Pushover's address; tests change it.
	API string
}

func (p Pushover) Send(ctx context.Context, m Message) error {
	priority := 0
	switch worst(m) {
	case "crit":
		priority = 1
	case "ok":
		priority = -1
	}
	if m.Test {
		priority = 0
	}
	body := map[string]any{
		"token": p.Token, "user": p.User, "title": truncate(m.Title(), 250), "priority": priority, "html": 1,
		"message": fit(m, 1000, func() string { return html.EscapeString(testText(m)) }, func(it Item) string {
			return fmt.Sprintf("<b>%s</b> %s on %s\n%s", label(it), html.EscapeString(it.Rule), html.EscapeString(where(it)), html.EscapeString(Describe(it)))
		}),
	}
	if len(m.Items) == 1 {
		body["url"], body["url_title"] = m.Items[0].URL, "Open in lotse"
	}
	b, _ := json.Marshal(body)
	api := p.API
	if api == "" {
		api = "https://api.pushover.net"
	}
	return post(ctx, api+"/1/messages.json", b, nil)
}
