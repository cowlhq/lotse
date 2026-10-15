package apply

import (
	"strings"
	"testing"
)

const example = `
settings:
  notify.channels: [ops]
channels:
  - key: ops
    googlechat:
      webhook: {env: OPS_CHAT}
customers:
  - key: acme
    name: ACME GmbH
    channels:
      - key: acme-mail
        email:
          to: [ops@acme.example]
          smtp: {host: smtp.acme.example, from: lotse@acme.example, password: {from_env: ACME_SMTP}}
    projects:
      - key: shop
        environments:
          - key: prod
            settings:
              rules.host.disk.warn: 80
            maintenance:
              - schedule: sun 02:00-04:00
                reason: backups
            checks:
              - key: homepage
                http: {url: "https://shop.acme.example", keyword: Willkommen}
              - key: nightly-backup
                heartbeat: {period: 24h, grace: 1h}
            hosts:
              - key: web1
                settings:
                  agent.auto_update: true
                containers:
                  - key: shop-cron
                    settings:
                      rules.container.restarting.enabled: false
`

func TestParse(t *testing.T) {
	d, err := Parse([]byte(example))
	if err != nil {
		t.Fatal(err)
	}
	env := d.Customers[0].Projects[0].Environments[0]
	if env.Settings["rules.host.disk.warn"] != 80 || env.Maintenance[0].Schedule != "sun 02:00-04:00" || env.Hosts[0].Containers[0].Key != "shop-cron" {
		t.Errorf("environment: %+v", env)
	}
	if typ, spec, err := env.Checks[0].Type(); err != nil || typ != "http" || spec["keyword"] != "Willkommen" {
		t.Errorf("check: %s %v %v", typ, spec, err)
	}
	if _, err := Parse([]byte("customers:\n  - key: acme\n    color: blue\n")); err == nil || !strings.Contains(err.Error(), "color") {
		t.Errorf("an unknown field: %v", err)
	}
	if _, _, err := (Check{Key: "x", HTTP: map[string]any{}, TCP: map[string]any{}}).Type(); err == nil {
		t.Error("a check of two types")
	}
	if d, err := Parse(nil); err != nil || len(d.Customers) != 0 {
		t.Errorf("an empty file: %v", err)
	}
}

func TestResolveFromEnv(t *testing.T) {
	d, _ := Parse([]byte(example))
	if err := d.ResolveFromEnv(func(string) string { return "" }); err == nil || !strings.Contains(err.Error(), "ACME_SMTP") {
		t.Errorf("missing: %v", err)
	}
	d, _ = Parse([]byte(example))
	if err := d.ResolveFromEnv(func(k string) string { return "pw-" + k }); err != nil {
		t.Fatal(err)
	}
	pw := d.Customers[0].Channels[0].Email.SMTP.Password
	if pw.Value != "pw-ACME_SMTP" || pw.FromEnv != "" {
		t.Errorf("resolved: %+v", pw)
	}
	if d.Channels[0].GoogleChat.Webhook.Env != "OPS_CHAT" {
		t.Error("a reference for the hub was resolved")
	}
	// A value never goes into a file.
	out, _ := Marshal(d)
	if strings.Contains(string(out), "pw-ACME_SMTP") {
		t.Errorf("a secret in the YAML:\n%s", out)
	}
}

// The channels of every type, as docs/install.md shows them, read, and their
// secrets found.
func TestChannelTypes(t *testing.T) {
	d, err := Parse([]byte(`
channels:
  - key: ops-phone
    ntfy: {topic: {from_env: NTFY_TOPIC}}
  - key: ops-telegram
    telegram: {chat_id: "-1001234567890", bot_token: {env: TELEGRAM_BOT}}
  - key: ops-slack
    slack: {webhook: {from_env: SLACK_WEBHOOK}}
  - key: ops-discord
    discord: {webhook: {from_env: DISCORD_WEBHOOK}}
  - key: oncall
    pushover: {user: {env: PUSHOVER_USER}, token: {env: PUSHOVER_APP}}
`))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	d.EachSecret(func(*Secret) { n++ })
	if n != 6 || d.Channels[1].Telegram.ChatID != "-1001234567890" {
		t.Errorf("%d secrets, %+v", n, d.Channels[1].Telegram)
	}
	env := map[string]string{"NTFY_TOPIC": "t", "SLACK_WEBHOOK": "s", "DISCORD_WEBHOOK": "d"}
	if err := d.ResolveFromEnv(func(k string) string { return env[k] }); err != nil || d.Channels[0].Ntfy.Topic.Value != "t" {
		t.Errorf("from_env: %v", err)
	}
}

func TestCheckTypes(t *testing.T) {
	for _, c := range []Check{
		{Key: "dns", DNS: map[string]any{"name": "shop.acme.de"}},
		{Key: "domain", Domain: map[string]any{"domain": "acme.com"}},
	} {
		if typ, _, err := c.Type(); err != nil || typ != c.Key {
			t.Errorf("%s: %s, %v", c.Key, typ, err)
		}
	}
}
