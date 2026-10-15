package protocol

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestDisconnected(t *testing.T) {
	if reason, planned := Disconnected(fmt.Errorf("failed to write JSON message: failed to marshal JSON: failed to write msg: %w", net.ErrClosed)); reason != "the connection broke off" || planned {
		t.Errorf("a broken connection: %q, %v", reason, planned)
	}
	if reason, planned := Disconnected(errors.New("dial tcp: connection refused")); reason != "dial tcp: connection refused" || planned {
		t.Errorf("refused: %q, %v", reason, planned)
	}
}
