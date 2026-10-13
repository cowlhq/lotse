package agent

import (
	"testing"
	"time"

	"lotse.sh/internal/protocol"
)

func hostMsg(ts int64) protocol.Message {
	return protocol.Message{Type: protocol.TypeHost, Host: &protocol.HostSample{TS: ts}}
}

func TestOutbox(t *testing.T) {
	now := time.Unix(1000, 0)
	o := newOutbox(time.Minute)
	o.now = func() time.Time { return now }

	o.add(hostMsg(1))
	now = now.Add(30 * time.Second)
	o.add(hostMsg(2))
	pending := o.pending()
	if len(pending) != 2 {
		t.Fatalf("%d pending, want 2", len(pending))
	}

	// While those are being sent, one more comes and the first gets too
	// old.
	now = now.Add(45 * time.Second)
	o.add(hostMsg(3))
	o.sent(pending)
	left := o.pending()
	if len(left) != 1 || left[0].Host.TS != 3 {
		t.Errorf("left: %d messages, want only the third", len(left))
	}
}

func TestOutboxLatest(t *testing.T) {
	o := newOutbox(time.Minute)
	list := func() protocol.Message {
		return protocol.Message{Type: protocol.TypeContainers, Containers: &protocol.ContainerList{}}
	}
	o.addLatest(list())
	o.add(hostMsg(1))
	pending := o.pending()
	newest := list()
	o.addLatest(newest)
	o.sent(pending)
	left := o.pending()
	if len(left) != 1 || left[0].Containers != newest.Containers {
		t.Errorf("left %d messages, want only the newest list", len(left))
	}
}
