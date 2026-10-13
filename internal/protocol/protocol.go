// Package protocol is what agents and the hub say to each other.
//
// An agent opens a WebSocket to ConnectPath and offers the subprotocols of
// the versions it speaks, newest first. The hub picks the newest one it
// speaks itself; it speaks the current version and the one before, so
// agents and hub need not be updated in lockstep. Every message is a JSON
// text frame with a type. A side that gets a type it does not know logs it
// and goes on.
package protocol

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is the newest version of the protocol this build speaks.
const Version = 1

// Versions are all the versions this build speaks, newest first.
var Versions = []int{1}

// ConnectPath is where agents open their WebSocket.
const ConnectPath = "/api/agent/v1/connect"

// Subprotocol is the WebSocket subprotocol for version v.
func Subprotocol(v int) string { return "lotse." + strconv.Itoa(v) }

// Subprotocols are the subprotocols of Versions.
func Subprotocols() []string {
	s := make([]string, len(Versions))
	for i, v := range Versions {
		s[i] = Subprotocol(v)
	}
	return s
}

// ParseSubprotocol returns the version of a subprotocol.
func ParseSubprotocol(s string) (int, error) {
	v, err := strconv.Atoi(strings.TrimPrefix(s, "lotse."))
	if err != nil || !strings.HasPrefix(s, "lotse.") {
		return 0, fmt.Errorf("%q is not a lotse protocol", s)
	}
	return v, nil
}

// Message types.
const (
	TypeHello   = "hello"
	TypeWelcome = "welcome"
)

// Message is one frame. Type says which of the other fields is set.
type Message struct {
	Type    string   `json:"type"`
	Hello   *Hello   `json:"hello,omitempty"`
	Welcome *Welcome `json:"welcome,omitempty"`
}

// Hello is the agent's first message on every connection.
type Hello struct {
	Version   string `json:"version"`
	Hostname  string `json:"hostname"`
	MachineID string `json:"machine_id,omitempty"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Welcome is the hub's answer to Hello.
type Welcome struct {
	HubVersion string `json:"hub_version"`
}
