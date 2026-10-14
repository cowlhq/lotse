// Package notify sends alerts to channels: email, Google Chat and webhooks
// (design 8.4). A message holds one alert or several, gathered over a few
// seconds, so that a larger outage arrives as one message.
package notify

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Item is one alert in a message.
type Item struct {
	Rule string `json:"rule"`
	// Subject is the path of the object, such as acme/shop/prod/web1, and
	// Instance what on it, such as a filesystem's mount point.
	Subject     string `json:"subject"`
	SubjectKind string `json:"subject_kind"`
	Instance    string `json:"instance,omitempty"`
	From        string `json:"from"`
	State       string `json:"state"`
	// Repeat says the alert is sent again, still firing.
	Repeat    bool     `json:"repeat,omitempty"`
	Since     int64    `json:"since"`
	Value     *float64 `json:"value,omitempty"`
	Threshold *float64 `json:"threshold,omitempty"`
	// Unit is what value and threshold are in: percent, count, days or
	// load.
	Unit   string `json:"unit,omitempty"`
	Reason string `json:"reason,omitempty"`
	// URL is where the alert is in the hub.
	URL string `json:"url"`
}

// Message is what a channel is sent.
type Message struct {
	Hub   string
	Items []Item
	// Test marks a message sent to try a channel.
	Test bool
}

// Sender sends messages to one channel.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Title sums a message up in a line, as for an email's subject.
func (m Message) Title() string {
	if m.Test {
		return "[lotse] Test message"
	}
	if len(m.Items) == 1 {
		it := m.Items[0]
		return "[lotse] " + label(it) + " " + it.Rule + " on " + where(it)
	}
	counts := map[string]int{}
	for _, it := range m.Items {
		counts[strings.ToUpper(it.State)]++
	}
	var parts []string
	for _, s := range []string{"CRIT", "WARN", "OK"} {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	return fmt.Sprintf("[lotse] %d alerts: %s", len(m.Items), strings.Join(parts, ", "))
}

// Text writes a message out, one alert after the other.
func (m Message) Text() string {
	var b strings.Builder
	if m.Test {
		fmt.Fprintf(&b, "This is a test message from lotse at %s. The channel works.\n", m.Hub)
		return b.String()
	}
	for i, it := range m.Items {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s %s on %s\n", label(it), it.Rule, where(it))
		fmt.Fprintf(&b, "%s\n", Describe(it))
		fmt.Fprintf(&b, "%s\n", it.URL)
	}
	return b.String()
}

// label is how an item's state is written: CRIT, WARN, or RESOLVED.
func label(it Item) string {
	if it.State == "ok" {
		return "RESOLVED"
	}
	return strings.ToUpper(it.State)
}

func where(it Item) string {
	if it.Instance != "" {
		return it.Subject + " " + it.Instance
	}
	return it.Subject
}

// Describe says in a sentence what an item is about: the value and the
// threshold, and why.
func Describe(it Item) string {
	var parts []string
	switch {
	case it.State == "ok":
		parts = append(parts, "Back to normal, after "+strings.ToUpper(it.From)+".")
	case it.Repeat:
		parts = append(parts, "Still "+strings.ToUpper(it.State)+", since "+time.Unix(it.Since, 0).Format("2006-01-02 15:04 MST")+".")
	}
	if it.Value != nil {
		s := "Now " + Format(*it.Value, it.Unit)
		if it.Threshold != nil {
			s += ", threshold " + Format(*it.Threshold, it.Unit)
		}
		parts = append(parts, s+".")
	}
	if it.Reason != "" {
		parts = append(parts, strings.TrimSuffix(it.Reason, ".")+".")
	}
	if len(parts) == 0 {
		parts = append(parts, strings.ToUpper(it.State)+" since "+time.Unix(it.Since, 0).Format("2006-01-02 15:04 MST")+".")
	}
	return strings.Join(parts, " ")
}

// Format writes a value in its unit.
func Format(v float64, unit string) string {
	n := strconv.FormatFloat(v, 'f', 1, 64)
	n = strings.TrimSuffix(n, ".0")
	switch unit {
	case "percent":
		return n + " %"
	case "days":
		if n == "1" {
			return "1 day"
		}
		return n + " days"
	case "load":
		return strconv.FormatFloat(v, 'f', 2, 64) + " per core"
	}
	return n
}

// plain strips the address from an HTTP client's error: a webhook's
// address is its secret, and errors are shown to everyone.
func plain(err error) error {
	var u *url.Error
	if errors.As(err, &u) {
		return u.Err
	}
	return err
}
