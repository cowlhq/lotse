package protocol

import (
	"errors"
	"io"
	"net"

	"github.com/coder/websocket"
)

// Disconnected says in words why a connection to the hub ended, and
// whether the hub meant it, such as when it restarts: no cause for a
// warning.
func Disconnected(err error) (reason string, planned bool) {
	switch websocket.CloseStatus(err) {
	case websocket.StatusGoingAway:
		return "the hub is restarting", true
	case websocket.StatusNormalClosure:
		return "the hub closed the connection", true
	}
	// A write to a connection that just broke reads, from the WebSocket
	// library, as "failed to marshal JSON"; it is the connection.
	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "the connection broke off", false
	}
	if err == nil {
		return "the connection ended", false
	}
	return err.Error(), false
}
