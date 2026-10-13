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
	// Containers is the full list of the machine's containers: after
	// connecting, and every few minutes as a safety net for missed events.
	TypeContainers     = "containers"
	TypeContainerEvent = "container_event"
	TypeContainerStats = "container_stats"
)

// Message is one frame. Type says which of the other fields is set.
type Message struct {
	Type    string      `json:"type"`
	Hello   *Hello      `json:"hello,omitempty"`
	Welcome *Welcome    `json:"welcome,omitempty"`
	Config  *Config     `json:"config,omitempty"`
	Host    *HostSample `json:"host,omitempty"`

	Containers     *ContainerList  `json:"containers,omitempty"`
	ContainerEvent *ContainerEvent `json:"container_event,omitempty"`
	ContainerStats *ContainerStats `json:"container_stats,omitempty"`
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
	// DockerVersion is empty when the agent cannot reach Docker.
	DockerVersion string `json:"docker_version,omitempty"`
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
	// ContainerInterval is how often to read the containers' usage.
	ContainerInterval int `json:"container_interval"`
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

// Container is a container as Docker describes it. The hub decides from its
// labels which of its containers it is (compose project and service, or
// the name).
type Container struct {
	DockerID string `json:"docker_id"`
	Name     string `json:"name"`
	Image    string `json:"image"`
	// State is Docker's: created, running, paused, restarting, exited, dead.
	State string `json:"state"`
	// Health is empty for a container without a health check.
	Health    string            `json:"health,omitempty"`
	ExitCode  int               `json:"exit_code"`
	Restarts  int               `json:"restarts"`
	StartedAt int64             `json:"started_at,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// ContainerList is every container on the machine, running or not.
type ContainerList struct {
	TS int64 `json:"ts"`
	// DockerVersion is sent along because Docker may become reachable only
	// after the agent said hello.
	DockerVersion string      `json:"docker_version"`
	Containers    []Container `json:"containers"`
}

// ContainerEvent is something Docker reported about a container, and the
// container as it is afterwards (nil once it is removed).
type ContainerEvent struct {
	TS        int64      `json:"ts"`
	DockerID  string     `json:"docker_id"`
	Action    string     `json:"action"`
	Container *Container `json:"container,omitempty"`
}

// ContainerStats is one reading of the running containers' usage. CPU is
// in percent of one core, as docker stats shows it; rates are bytes per
// second.
type ContainerStats struct {
	TS    int64           `json:"ts"`
	Stats []ContainerStat `json:"stats"`
}

type ContainerStat struct {
	DockerID string   `json:"docker_id"`
	CPU      *float64 `json:"cpu,omitempty"`
	Mem      *int64   `json:"mem,omitempty"`
	MemLimit *int64   `json:"mem_limit,omitempty"`
	NetRx    *float64 `json:"net_rx,omitempty"`
	NetTx    *float64 `json:"net_tx,omitempty"`
}
