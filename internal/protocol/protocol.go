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
	TypeConfig  = "config"
	TypeHost    = "host"
)

// Message is one frame. Type says which of the other fields is set.
type Message struct {
	Type    string      `json:"type"`
	Hello   *Hello      `json:"hello,omitempty"`
	Welcome *Welcome    `json:"welcome,omitempty"`
	Config  *Config     `json:"config,omitempty"`
	Host    *HostSample `json:"host,omitempty"`
}

// Hello is the agent's first message on every connection: who it is and
// what machine it is on.
type Hello struct {
	Version   string `json:"version"`
	Hostname  string `json:"hostname"`
	MachineID string `json:"machine_id,omitempty"`
	OS        string `json:"os"`
	Platform  string `json:"platform,omitempty"`
	Kernel    string `json:"kernel,omitempty"`
	Arch      string `json:"arch"`
	CPUModel  string `json:"cpu_model,omitempty"`
	Cores     int    `json:"cores,omitempty"`
	MemTotal  int64  `json:"mem_total,omitempty"`
	// BootTime is when the machine started, in Unix seconds.
	BootTime int64 `json:"boot_time,omitempty"`
}

// Welcome is the hub's answer to Hello.
type Welcome struct {
	HubVersion string `json:"hub_version"`
	Config     Config `json:"config"`
}

// Config is how the agent works. The hub sends it in Welcome and again
// whenever it changes.
type Config struct {
	// HostInterval is how often to read the machine, in seconds.
	HostInterval int `json:"host_interval"`
}

// HostSample is one reading of the machine. Times are Unix seconds,
// percentages 0 to 100, rates bytes per second; a missing value was not
// measured.
type HostSample struct {
	TS           int64        `json:"ts"`
	CPU          *float64     `json:"cpu,omitempty"`
	IOWait       *float64     `json:"iowait,omitempty"`
	Steal        *float64     `json:"steal,omitempty"`
	Load1        *float64     `json:"load1,omitempty"`
	Load5        *float64     `json:"load5,omitempty"`
	Load15       *float64     `json:"load15,omitempty"`
	MemUsed      *int64       `json:"mem_used,omitempty"`
	MemAvailable *int64       `json:"mem_available,omitempty"`
	SwapUsed     *int64       `json:"swap_used,omitempty"`
	SwapTotal    *int64       `json:"swap_total,omitempty"`
	NetRx        *float64     `json:"net_rx,omitempty"`
	NetTx        *float64     `json:"net_tx,omitempty"`
	DiskRead     *float64     `json:"disk_read,omitempty"`
	DiskWrite    *float64     `json:"disk_write,omitempty"`
	Uptime       *int64       `json:"uptime,omitempty"`
	Filesystems  []Filesystem `json:"filesystems,omitempty"`
}

// Filesystem is one filesystem's usage, in bytes.
type Filesystem struct {
	Mount       string `json:"mount"`
	Device      string `json:"device"`
	FSType      string `json:"fstype"`
	Used        int64  `json:"used"`
	Total       int64  `json:"total"`
	InodesUsed  *int64 `json:"inodes_used,omitempty"`
	InodesTotal *int64 `json:"inodes_total,omitempty"`
}
