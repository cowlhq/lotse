package hub

import (
	"cmp"
	"context"
	"encoding/json"
	"math"
	"strconv"
	"time"

	"lotse.sh/internal/alert"
	"lotse.sh/internal/config"
	"lotse.sh/internal/notify"
	"lotse.sh/internal/store"
)

// Notifications (design 8.4): queued by the alert loop, sent by the
// notifier. What comes for one channel within gather is sent together, so
// a larger outage arrives as one message. A failed send is tried again,
// later each time, up to maxAttempts.

const (
	gather      = 30 * time.Second
	notifyEvery = 5 * time.Second
	maxAttempts = 6
	// maxItems is the most alerts in one message.
	maxItems = 50
)

var backoff = []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute}

// target is a channel an alert goes to, or a key that names none.
type target struct {
	channel *store.Channel
	name    string
	err     string
}

// targets are the channels of notify.channels on an object: each key is
// the nearest channel of that key, defined on the object's scope or above.
func targets(t *tree, st store.AlertState, chain []config.Level) []target {
	v, _ := t.value(chain, "notify.channels")
	var keys []string
	json.Unmarshal(v, &keys)
	var out []target
	for _, key := range keys {
		var found *store.Channel
		for _, l := range chain {
			for i, c := range t.channels {
				if found == nil && c.ScopeID == l.ID && c.Slug == key && isScope(l.Kind) {
					found = &t.channels[i]
				}
			}
		}
		switch {
		case found == nil:
			out = append(out, target{name: key, err: "there is no channel " + strconv.Quote(key) + " here or above"})
		case st.SubjectKind == "channel" && found.ID == st.SubjectID:
			// A failing channel is not told that it fails.
		default:
			out = append(out, target{channel: found, name: channelKey(*found)})
		}
	}
	return out
}

func isScope(kind string) bool {
	return kind == "environment" || kind == "project" || kind == "customer" || kind == "global"
}

// repeatEvery is notify.repeat on an object; 0 for never.
func repeatEvery(t *tree, chain []config.Level) time.Duration {
	v, _ := t.value(chain, "notify.repeat")
	d, _ := config.Duration(v)
	return d
}

// queue queues what is due about a state: a change not yet sent, or a
// repeat of one that still fires. During maintenance, a change is recorded
// as held back, and goes out when the window ends.
func (h *hub) queue(ctx context.Context, t *tree, st store.AlertState, chain []config.Level, maintenance, transitioned bool, now time.Time) error {
	if st.State == alert.Unknown || st.AlertID == nil {
		return nil
	}
	kind := ""
	switch {
	case st.State != st.NotifiedState && (alert.Firing(st.State) || alert.Firing(st.NotifiedState)):
		kind = "change"
	case alert.Firing(st.State) && st.AckedAt == nil && st.NotifiedAt != nil:
		if every := repeatEvery(t, chain); every > 0 && now.Sub(time.Unix(*st.NotifiedAt, 0)) >= every {
			kind = "repeat"
		}
	}
	if kind == "" {
		return nil
	}
	var list []store.Notification
	for _, tg := range targets(t, st, chain) {
		n := store.Notification{AlertID: *st.AlertID, Channel: tg.name, Kind: kind, Status: "pending", CreatedAt: now.Unix()}
		switch {
		case maintenance:
			n.Status = "suppressed"
		case tg.err != "":
			n.Status, n.Error = "failed", tg.err
		default:
			n.ChannelID, n.NextAt = &tg.channel.ID, ptr(now.Add(cmp.Or(h.gather, gather)).Unix())
		}
		list = append(list, n)
	}
	if maintenance {
		if !transitioned || kind != "change" {
			return nil
		}
		return h.db.Notify(ctx, st, list)
	}
	st.NotifiedState, st.NotifiedAt = st.State, ptr(now.Unix())
	if err := h.db.Notify(ctx, st, list); err != nil {
		return err
	}
	h.setState(st)
	return nil
}

func (h *hub) notifyLoop(ctx context.Context) {
	t := time.NewTicker(cmp.Or(h.notifyEvery, notifyEvery))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if err := h.deliver(ctx, time.Now()); err != nil && ctx.Err() == nil {
			h.log.Error("cannot send notifications", "err", err)
		}
	}
}

// deliver sends what is due, channel by channel.
func (h *hub) deliver(ctx context.Context, now time.Time) error {
	pending, alerts, err := h.db.PendingNotifications(ctx, math.MaxInt64)
	if err != nil {
		return err
	}
	byChannel := map[int64][]store.Notification{}
	var order []int64
	var orphans []store.Notification
	for _, n := range pending {
		if n.ChannelID == nil {
			n.Status, n.Error, n.NextAt = "failed", "the channel was deleted", nil
			orphans = append(orphans, n)
			continue
		}
		id := *n.ChannelID
		if byChannel[id] == nil {
			order = append(order, id)
		}
		byChannel[id] = append(byChannel[id], n)
	}
	if len(orphans) > 0 {
		if err := h.db.FinishNotifications(ctx, orphans); err != nil {
			return err
		}
	}
	for _, id := range order {
		list := byChannel[id]
		due := false
		for _, n := range list {
			due = due || n.NextAt != nil && *n.NextAt <= now.Unix()
		}
		if !due {
			continue
		}
		for len(list) > 0 {
			batch := list[:min(len(list), maxItems)]
			list = list[len(batch):]
			if err := h.sendBatch(ctx, id, batch, alerts, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// sendBatch sends some notifications to one channel, as one message, and
// records how it went.
func (h *hub) sendBatch(ctx context.Context, channelID int64, batch []store.Notification, alerts map[int64]store.Alert, now time.Time) error {
	msg := notify.Message{Hub: h.URL()}
	for _, n := range batch {
		msg.Items = append(msg.Items, h.item(alerts[n.AlertID], n.Kind == "repeat"))
	}
	c, err := h.db.Channel(ctx, channelID)
	if err == nil {
		var sender notify.Sender
		if sender, err = h.sender(ctx, c); err == nil {
			err = sender.Send(ctx, msg)
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	h.alerts.mu.Lock()
	if h.alerts.failures == nil {
		h.alerts.failures = map[int64]int{}
	}
	if err != nil {
		h.alerts.failures[channelID]++
	} else {
		delete(h.alerts.failures, channelID)
	}
	h.alerts.mu.Unlock()
	for i := range batch {
		n := &batch[i]
		n.Attempts++
		switch {
		case err == nil:
			n.Status, n.Error, n.SentAt, n.NextAt = "sent", "", ptr(now.Unix()), nil
		case n.Attempts >= maxAttempts:
			n.Status, n.Error, n.NextAt = "failed", err.Error(), nil
		default:
			n.Error, n.NextAt = err.Error(), ptr(now.Add(backoff[min(n.Attempts, len(backoff))-1]).Unix())
		}
	}
	if err != nil {
		h.log.Warn("cannot send a notification", "channel", c.Slug, "err", err)
	}
	if err := h.db.FinishNotifications(ctx, batch); err != nil {
		return err
	}
	h.live.publish(LiveMessage{Type: "alerts"})
	return nil
}

// item is an alert as a message tells it.
func (h *hub) item(a store.Alert, repeat bool) notify.Item {
	r, _ := config.LookupRule(a.Rule)
	it := notify.Item{
		Rule: a.Rule, Subject: a.SubjectPath, SubjectKind: a.SubjectKind, Instance: a.Instance,
		From: a.From, State: a.To, Repeat: repeat, Since: a.At, Value: a.Value, Threshold: a.Threshold,
		Unit: r.Unit, Reason: a.Reason, URL: h.URL() + "/alerts/" + strconv.FormatInt(a.ID, 10),
	}
	if repeat {
		h.alerts.mu.Lock()
		if v, ok := h.alerts.values[a.AlertKey]; ok && v != nil {
			it.Value = v
		}
		h.alerts.mu.Unlock()
	}
	if r.Condition() {
		it.Value, it.Threshold = nil, nil
	}
	return it
}

// deadman calls the dead man's switch, if one is set and it is time: the
// alert loop has just worked through everything, so it works.
func (h *hub) deadman(ctx context.Context, now time.Time) {
	set, err := h.db.Settings(ctx, []store.Subject{{Kind: "global", ID: store.GlobalScope}})
	if err != nil {
		return
	}
	var url string
	every := time.Minute
	if v, ok := set[0]["deadman.url"]; ok {
		json.Unmarshal(v, &url)
	}
	if v, ok := set[0]["deadman.interval"]; ok {
		if d, err := config.Duration(v); err == nil {
			every = d
		}
	}
	h.alerts.mu.Lock()
	due := url != "" && !h.alerts.pinging && now.Sub(h.alerts.lastPing) >= every
	if due {
		h.alerts.pinging, h.alerts.lastPing = true, now
	}
	h.alerts.mu.Unlock()
	if !due {
		return
	}
	go func() {
		pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := notify.Ping(pctx, url); err != nil && ctx.Err() == nil {
			h.log.Warn("cannot call the dead man's switch", "err", err)
		}
		h.alerts.mu.Lock()
		h.alerts.pinging = false
		h.alerts.mu.Unlock()
	}()
}
