package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"lotse.sh/internal/protocol"
)

// The made-up world: a small IT service provider that looks after a
// logistics company's shop and ERP, a bakery's website and a health
// company's portal and API, and its own infrastructure. Every name, address
// and number here is invented.

const gib = 1 << 30

// machine is a host the demo plays, and how it behaves.
type machine struct {
	Name     string
	Env      string // the environment's path
	Platform string
	Kernel   string
	CPU      string
	Cores    int
	MemGiB   float64
	SwapGiB  float64
	// SwapUsed is the share of swap in use.
	SwapUsed float64
	// VPS machines lose a little to steal.
	VPS bool
	// CPUBase is the CPU at night, CPUPeak what the afternoon adds, in
	// percent.
	CPUBase, CPUPeak float64
	// Mem is the share of memory used at night, MemPeak what the day adds.
	Mem, MemPeak float64
	// Net is the traffic received at the peak, in bytes per second; TX is
	// how much more it sends.
	Net, TX float64
	// Disk is the disk writes at the peak, in bytes per second.
	Disk float64
	// Backup runs a nightly job from 02:00 to 02:40: CPU, reads, uploads.
	Backup bool
	// BackupTarget receives backups from 02:00 to 03:30.
	BackupTarget bool
	// Docker is the version of Docker, empty without.
	Docker      string
	BootDaysAgo float64
	// RebootReason, if set, says updates wait for a restart.
	RebootReason string
	Services     []string
	Filesystems  []filesystem
	Containers   []container
}

type filesystem struct {
	Mount, Device, FSType string
	GiB                   float64
	// From and To are the share used at the start of the history and now.
	From, To float64
	// Points, if set, are the share used over time instead, for a disk
	// that filled up and was cleaned.
	Points []point
}

type point struct {
	t time.Time
	v float64
}

type container struct {
	Project, Service string
	// Replica is the compose container number; 0 means 1.
	Replica int
	Image   string
	// CPU is the usage at night and CPUPeak what the day adds, in percent
	// of one core.
	CPU, CPUPeak float64
	MemMiB       float64
	LimitMiB     float64
	Net          float64
	// Health is healthy or unhealthy for a container with a health check.
	Health string
	// Restarting keeps crashing and coming back.
	Restarting  bool
	StartedDays float64
	Labels      map[string]string
}

func (c container) name() string {
	return fmt.Sprintf("%s-%s-%d", c.Project, c.Service, max(c.Replica, 1))
}

// slug is how the hub names the container (design 5.1).
func (c container) slug() string {
	s := c.Project + "-" + c.Service
	if c.Replica > 1 {
		s += fmt.Sprintf("-%d", c.Replica)
	}
	return s
}

func shopApp(replica int, labels bool) container {
	c := container{Project: "shop", Service: "app", Replica: replica, Image: "registry.nordwind-logistik.example/shop/app:2.14.3",
		CPU: 4, CPUPeak: 55, MemMiB: 780, LimitMiB: 2048, Net: 1.6e6, Health: "healthy", StartedDays: 1.2}
	if labels {
		c.Labels = map[string]string{
			"lotse.check.web.http.url":     "https://shop.nordwind-logistik.example/health",
			"lotse.check.web.http.keyword": "ok",
		}
	}
	return c
}

var (
	ubuntu24 = [2]string{"ubuntu 24.04", "6.8.0-45-generic"}
	ubuntu22 = [2]string{"ubuntu 22.04", "5.15.0-122-generic"}
	debian12 = [2]string{"debian 12.7", "6.1.0-25-amd64"}
	epyc     = "AMD EPYC 7543P 32-Core Processor"
	xeon     = "Intel(R) Xeon(R) Gold 6338 CPU @ 2.00GHz"
	vcpu     = "AMD EPYC-Milan Processor"
)

var baseServices = []string{"cron.service", "ssh.service", "systemd-journald.service", "systemd-timesyncd.service", "unattended-upgrades.service", "lotse-agent.service"}

func machines(s *story) []machine {
	withDocker := append([]string{"containerd.service", "docker.service"}, baseServices...)
	return []machine{
		{
			Name: "web-01", Env: "nordwind/shop/production", Platform: ubuntu24[0], Kernel: ubuntu24[1], CPU: epyc,
			Cores: 4, MemGiB: 8, CPUBase: 5, CPUPeak: 38, Mem: 0.41, MemPeak: 0.12, Net: 2.4e6, TX: 3.1, Disk: 1.2e6,
			Docker: "27.3.1", BootDaysAgo: 23, Services: withDocker,
			Filesystems: []filesystem{{Mount: "/", Device: "/dev/sda1", FSType: "ext4", GiB: 80, From: 0.41, To: 0.44}},
			Containers: []container{
				{Project: "shop", Service: "nginx", Image: "nginx:1.27-alpine", CPU: 0.4, CPUPeak: 6, MemMiB: 24, Net: 2.2e6, StartedDays: 23},
				shopApp(1, true),
				{Project: "shop", Service: "worker", Image: "registry.nordwind-logistik.example/shop/app:2.14.3", CPU: 1.5, CPUPeak: 12, MemMiB: 410, LimitMiB: 1024, Net: 1e5, StartedDays: 1.2},
				{Project: "shop", Service: "redis", Image: "redis:7.4-alpine", CPU: 0.3, CPUPeak: 2.5, MemMiB: 96, Net: 4e5, StartedDays: 23},
			},
		},
		{
			Name: "web-02", Env: "nordwind/shop/production", Platform: ubuntu24[0], Kernel: ubuntu24[1], CPU: epyc,
			Cores: 4, MemGiB: 8, CPUBase: 4, CPUPeak: 33, Mem: 0.38, MemPeak: 0.11, Net: 2.1e6, TX: 3.0, Disk: 1e6,
			Docker: "27.3.1", BootDaysAgo: 23, Services: withDocker,
			Filesystems: []filesystem{{Mount: "/", Device: "/dev/sda1", FSType: "ext4", GiB: 80, From: 0.39, To: 0.41}},
			Containers: []container{
				{Project: "shop", Service: "nginx", Image: "nginx:1.27-alpine", CPU: 0.4, CPUPeak: 5, MemMiB: 22, Net: 1.9e6, StartedDays: 23},
				shopApp(1, true),
				{Project: "shop", Service: "worker", Image: "registry.nordwind-logistik.example/shop/app:2.14.3", CPU: 1.2, CPUPeak: 10, MemMiB: 395, LimitMiB: 1024, Net: 1e5, StartedDays: 1.2},
			},
		},
		{
			Name: "db-01", Env: "nordwind/shop/production", Platform: debian12[0], Kernel: debian12[1], CPU: xeon,
			Cores: 8, MemGiB: 32, SwapGiB: 4, SwapUsed: 0.06, CPUBase: 4, CPUPeak: 26, Mem: 0.74, MemPeak: 0.06, Net: 3.2e6, TX: 1.4, Disk: 6e6,
			Backup: true, Docker: "27.3.1", BootDaysAgo: 61, Services: withDocker,
			Filesystems: []filesystem{
				{Mount: "/", Device: "/dev/sda1", FSType: "ext4", GiB: 40, From: 0.34, To: 0.35},
				{Mount: "/var/lib/postgresql", Device: "/dev/sdb", FSType: "xfs", GiB: 500, From: 0.61, To: 0.64},
			},
			Containers: []container{
				{Project: "db", Service: "postgres", Image: "postgres:16.4", CPU: 6, CPUPeak: 160, MemMiB: 9800, Net: 3e6, Health: "healthy", StartedDays: 61},
				{Project: "db", Service: "pgbouncer", Image: "edoburu/pgbouncer:1.23.1", CPU: 0.5, CPUPeak: 7, MemMiB: 18, Net: 3e6, StartedDays: 61},
				{Project: "db", Service: "exporter", Image: "quay.io/prometheuscommunity/postgres-exporter:v0.15.0", CPU: 0.2, CPUPeak: 0.4, MemMiB: 21, Net: 2e3, StartedDays: 61},
			},
		},
		{
			Name: "staging-01", Env: "nordwind/shop/staging", Platform: ubuntu24[0], Kernel: ubuntu24[1], CPU: vcpu,
			Cores: 4, MemGiB: 8, VPS: true, CPUBase: 3, CPUPeak: 9, Mem: 0.52, MemPeak: 0.04, Net: 2e5, TX: 1.3, Disk: 4e5,
			Docker: "27.3.1", BootDaysAgo: 9, Services: withDocker,
			Filesystems: []filesystem{{Mount: "/", Device: "/dev/vda1", FSType: "ext4", GiB: 60, Points: []point{
				{s.start.Add(-7 * 24 * time.Hour), 0.61}, {s.diskWarn.Add(-10 * time.Hour), 0.66}, {s.diskWarn, 0.853},
				{s.diskCrit, 0.952}, {s.diskCleaned.Add(-time.Minute), 0.961}, {s.diskCleaned, 0.52}, {s.start, 0.58},
			}}},
			Containers: []container{
				{Project: "shop-staging", Service: "nginx", Image: "nginx:1.27-alpine", CPU: 0.1, CPUPeak: 0.6, MemMiB: 14, Net: 1e5, StartedDays: 9},
				{Project: "shop-staging", Service: "app", Image: "registry.nordwind-logistik.example/shop/app:2.15.0-rc.2", CPU: 1, CPUPeak: 6, MemMiB: 640, LimitMiB: 2048, Net: 1e5, Health: "healthy", StartedDays: 0.3},
				{Project: "shop-staging", Service: "queue", Image: "registry.nordwind-logistik.example/shop/app:2.15.0-rc.2", CPU: 2, CPUPeak: 0, MemMiB: 180, LimitMiB: 512, Net: 1e4, Restarting: true, StartedDays: 0.3},
				{Project: "shop-staging", Service: "postgres", Image: "postgres:16.4", CPU: 0.6, CPUPeak: 3, MemMiB: 310, Net: 5e4, Health: "healthy", StartedDays: 9},
				{Project: "shop-staging", Service: "mailpit", Image: "axllent/mailpit:v1.20", CPU: 0.1, CPUPeak: 0.2, MemMiB: 31, Net: 1e3, StartedDays: 9},
			},
		},
		{
			Name: "erp-app-01", Env: "nordwind/erp/production", Platform: ubuntu22[0], Kernel: ubuntu22[1], CPU: xeon,
			Cores: 8, MemGiB: 16, SwapGiB: 4, SwapUsed: 0.22, CPUBase: 3, CPUPeak: 31, Mem: 0.58, MemPeak: 0.17, Net: 9e5, TX: 2.2, Disk: 8e5,
			Docker: "26.1.4", BootDaysAgo: 112, Services: withDocker,
			Filesystems: []filesystem{{Mount: "/", Device: "/dev/sda2", FSType: "ext4", GiB: 120, From: 0.52, To: 0.53}},
			Containers: []container{
				{Project: "erp", Service: "odoo", Image: "odoo:17.0", CPU: 3, CPUPeak: 140, MemMiB: 3100, LimitMiB: 6144, Net: 8e5, StartedDays: 34},
				{Project: "erp", Service: "nginx", Image: "nginx:1.27-alpine", CPU: 0.2, CPUPeak: 3, MemMiB: 19, Net: 9e5, StartedDays: 34},
				{Project: "erp", Service: "redis", Image: "redis:7.4-alpine", CPU: 0.2, CPUPeak: 1.2, MemMiB: 64, Net: 1e5, StartedDays: 34},
			},
		},
		{
			Name: "erp-db-01", Env: "nordwind/erp/production", Platform: debian12[0], Kernel: debian12[1], CPU: xeon,
			Cores: 8, MemGiB: 32, CPUBase: 2, CPUPeak: 18, Mem: 0.69, MemPeak: 0.05, Net: 7e5, TX: 1.8, Disk: 3e6,
			Backup: true, Docker: "26.1.4", BootDaysAgo: 112, Services: withDocker,
			Filesystems: []filesystem{
				{Mount: "/", Device: "/dev/sda1", FSType: "ext4", GiB: 40, From: 0.29, To: 0.29},
				{Mount: "/srv/pgdata", Device: "/dev/sdb1", FSType: "xfs", GiB: 250, From: 0.47, To: 0.48},
			},
			Containers: []container{
				{Project: "erp", Service: "postgres", Image: "postgres:15.8", CPU: 2, CPUPeak: 95, MemMiB: 8400, Net: 7e5, Health: "healthy", StartedDays: 112},
			},
		},
		{
			Name: "www-01", Env: "sommer/website/production", Platform: ubuntu22[0], Kernel: ubuntu22[1], CPU: vcpu,
			Cores: 2, MemGiB: 4, SwapGiB: 1, SwapUsed: 0.31, VPS: true, CPUBase: 3, CPUPeak: 21, Mem: 0.63, MemPeak: 0.09, Net: 3.5e5, TX: 4.2, Disk: 2e5,
			Docker: "27.3.1", BootDaysAgo: 41, Services: withDocker,
			Filesystems: []filesystem{{Mount: "/", Device: "/dev/vda1", FSType: "ext4", GiB: 40, From: 0.861, To: 0.884}},
			Containers: []container{
				{Project: "website", Service: "wordpress", Image: "wordpress:6.6-php8.2-apache", CPU: 0.8, CPUPeak: 32, MemMiB: 420, Net: 3.2e5, StartedDays: 41},
				{Project: "website", Service: "mariadb", Image: "mariadb:11.4", CPU: 0.4, CPUPeak: 6, MemMiB: 260, Net: 6e4, Health: "healthy", StartedDays: 41},
			},
		},
		{
			Name: "portal-01", Env: "atlas/portal/production", Platform: ubuntu24[0], Kernel: ubuntu24[1], CPU: epyc,
			Cores: 8, MemGiB: 16, CPUBase: 3, CPUPeak: 24, Mem: 0.47, MemPeak: 0.10, Net: 1.3e6, TX: 2.6, Disk: 9e5,
			Docker: "27.3.1", BootDaysAgo: 17, Services: withDocker,
			Filesystems: []filesystem{{Mount: "/", Device: "/dev/nvme0n1p2", FSType: "ext4", GiB: 160, From: 0.36, To: 0.38}},
			Containers: []container{
				{Project: "portal", Service: "web", Image: "ghcr.io/atlas-health/portal:5.2.0", CPU: 1.5, CPUPeak: 48, MemMiB: 690, LimitMiB: 2048, Net: 1.1e6, Health: "healthy", StartedDays: 4},
				{Project: "portal", Service: "nginx", Image: "nginx:1.27-alpine", CPU: 0.2, CPUPeak: 4, MemMiB: 21, Net: 1.2e6, StartedDays: 17},
				{Project: "portal", Service: "keycloak", Image: "quay.io/keycloak/keycloak:25.0", CPU: 1.2, CPUPeak: 9, MemMiB: 1150, LimitMiB: 2048, Net: 1.5e5, Health: "healthy", StartedDays: 17},
			},
		},
		{
			Name: "api-01", Env: "atlas/api/production", Platform: ubuntu24[0], Kernel: ubuntu24[1], CPU: epyc,
			Cores: 8, MemGiB: 16, CPUBase: 4, CPUPeak: 29, Mem: 0.44, MemPeak: 0.12, Net: 1.8e6, TX: 1.9, Disk: 7e5,
			Docker: "27.3.1", BootDaysAgo: 17, Services: withDocker,
			Filesystems: []filesystem{{Mount: "/", Device: "/dev/nvme0n1p2", FSType: "ext4", GiB: 160, From: 0.31, To: 0.33}},
			Containers: []container{
				{Project: "api", Service: "app", Image: "ghcr.io/atlas-health/api:3.8.1", CPU: 2, CPUPeak: 61, MemMiB: 910, LimitMiB: 2048, Net: 1.6e6, Health: "unhealthy", StartedDays: 2.1},
				{Project: "api", Service: "nginx", Image: "nginx:1.27-alpine", CPU: 0.3, CPUPeak: 5, MemMiB: 23, Net: 1.7e6, StartedDays: 17},
				{Project: "api", Service: "redis", Image: "redis:7.4-alpine", CPU: 0.4, CPUPeak: 3, MemMiB: 210, Net: 5e5, StartedDays: 17},
			},
		},
		{
			Name: "worker-02", Env: "atlas/api/production", Platform: ubuntu24[0], Kernel: ubuntu24[1], CPU: epyc,
			Cores: 4, MemGiB: 8, CPUBase: 7, CPUPeak: 34, Mem: 0.55, MemPeak: 0.14, Net: 4e5, TX: 0.8, Disk: 1.5e6,
			Docker: "27.3.1", BootDaysAgo: s.start.Sub(s.workerUp.Add(-50*time.Second)).Hours() / 24, Services: withDocker,
			Filesystems: []filesystem{{Mount: "/", Device: "/dev/sda1", FSType: "ext4", GiB: 80, From: 0.22, To: 0.24}},
			Containers: []container{
				{Project: "api", Service: "worker", Image: "ghcr.io/atlas-health/api:3.8.1", CPU: 4, CPUPeak: 70, MemMiB: 820, LimitMiB: 1536, Net: 2e5, StartedDays: s.start.Sub(s.workerUp.Add(-30*time.Second)).Hours() / 24},
				{Project: "api", Service: "worker", Replica: 2, Image: "ghcr.io/atlas-health/api:3.8.1", CPU: 4, CPUPeak: 66, MemMiB: 790, LimitMiB: 1536, Net: 2e5, StartedDays: s.start.Sub(s.workerUp.Add(-30*time.Second)).Hours() / 24},
			},
		},
		{
			Name: "edge-fra", Env: "ops/infra/production", Platform: debian12[0], Kernel: debian12[1], CPU: vcpu,
			Cores: 2, MemGiB: 4, VPS: true, CPUBase: 2, CPUPeak: 14, Mem: 0.31, MemPeak: 0.05, Net: 7e6, TX: 1.05, Disk: 1e5,
			Docker: "27.3.1", BootDaysAgo: 33, Services: withDocker,
			Filesystems: []filesystem{{Mount: "/", Device: "/dev/vda1", FSType: "ext4", GiB: 25, From: 0.27, To: 0.28}},
			Containers: []container{
				{Project: "edge", Service: "traefik", Image: "traefik:v3.1", CPU: 0.8, CPUPeak: 17, MemMiB: 88, Net: 7e6, StartedDays: 33},
				{Project: "edge", Service: "crowdsec", Image: "crowdsecurity/crowdsec:v1.6.3", CPU: 0.5, CPUPeak: 3, MemMiB: 130, Net: 2e4, Health: "healthy", StartedDays: 33},
			},
		},
		{
			Name: "backup-01", Env: "ops/infra/production", Platform: debian12[0], Kernel: debian12[1], CPU: "Intel(R) Xeon(R) E-2236 CPU @ 3.40GHz",
			Cores: 6, MemGiB: 16, CPUBase: 1, CPUPeak: 2, Mem: 0.18, MemPeak: 0.01, Net: 5e4, TX: 0.6, Disk: 3e5,
			BackupTarget: true, BootDaysAgo: 58, RebootReason: "linux-image-6.1.0-26-amd64", Services: append([]string{"rest-server.service"}, baseServices...),
			Filesystems: []filesystem{
				{Mount: "/", Device: "/dev/sda1", FSType: "ext4", GiB: 30, From: 0.24, To: 0.24},
				{Mount: "/srv/backup", Device: "/dev/md0", FSType: "ext4", GiB: 4096, From: 0.708, To: 0.731},
			},
		},
	}
}

// story is when things happened, worked out from when the demo starts.
type story struct {
	start time.Time
	// worker-02 hung and was rebooted yesterday afternoon.
	workerDown, workerUp time.Time
	// db-01 ran a runaway query two nights ago.
	spikeFrom, spikeTo time.Time
	// staging-01's disk filled up five days ago, until someone pruned
	// Docker's images.
	diskWarn, diskCrit, diskCleaned time.Time
	// The bakery's website did not answer three days ago, for twelve
	// minutes.
	siteDown, siteUp time.Time
	// The shop was deployed four days ago and yesterday morning.
	deploys []time.Time
	// staging's queue worker crashes since this morning's release
	// candidate.
	crashingSince time.Time
}

func newStory(start time.Time) *story {
	at := func(daysAgo, hour, minute int) time.Time {
		d := start.Local().AddDate(0, 0, -daysAgo)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, time.Local)
	}
	s := &story{
		start:      start,
		workerDown: at(1, 14, 37), workerUp: at(1, 14, 58),
		spikeFrom: at(2, 1, 6), spikeTo: at(2, 1, 52),
		diskWarn: at(5, 16, 12), diskCrit: at(5, 19, 40), diskCleaned: at(5, 21, 3),
		siteDown: at(3, 11, 20), siteUp: at(3, 11, 32),
		deploys:       []time.Time{at(4, 10, 15), at(1, 9, 42)},
		crashingSince: start.Add(-38 * time.Minute),
	}
	return s
}

// usage is a filesystem's share used at t.
func (f filesystem) usage(m *machine, s *story, t time.Time) float64 {
	noise := 0.0015 * smooth(seed(m.Name, f.Mount, "fs"), t.Unix(), 3600)
	if len(f.Points) > 0 {
		return clamp(interpolate(f.Points, t)+noise, 0, 1)
	}
	total := 7 * 24 * time.Hour
	p := clamp(1-s.start.Sub(t).Seconds()/total.Seconds(), 0, 1)
	return clamp(f.From+(f.To-f.From)*p+noise, 0, 1)
}

func interpolate(ps []point, t time.Time) float64 {
	if !t.After(ps[0].t) {
		return ps[0].v
	}
	for i := 1; i < len(ps); i++ {
		if t.Before(ps[i].t) {
			a, b := ps[i-1], ps[i]
			f := t.Sub(a.t).Seconds() / b.t.Sub(a.t).Seconds()
			return a.v + (b.v-a.v)*f
		}
	}
	return ps[len(ps)-1].v
}

// cpuAt is the machine's CPU without the moment's jitter, which the load
// follows.
func (m *machine) cpuAt(s *story, t time.Time) float64 {
	ts := t.Unix()
	v := m.CPUBase + m.CPUPeak*traffic(t) + m.CPUPeak*0.15*smooth(seed(m.Name, "cpu"), ts, 600)
	if m.Backup && inWindow(t, 2*time.Hour, 2*time.Hour+40*time.Minute) {
		v += 22
	}
	if m.BackupTarget && inWindow(t, 2*time.Hour, 3*time.Hour+30*time.Minute) {
		v += 9
	}
	if m.Name == "db-01" && !t.Before(s.spikeFrom) && t.Before(s.spikeTo) {
		v = 97
	}
	return clamp(v, 0.3, 99.5)
}

// down says whether the machine's agent was away at t.
func (m *machine) down(s *story, t time.Time) bool {
	return m.Name == "worker-02" && !t.Before(s.workerDown) && t.Before(s.workerUp)
}

func (m *machine) bootTime(s *story) int64 {
	return s.start.Add(-time.Duration(m.BootDaysAgo * 24 * float64(time.Hour))).Unix()
}

// sample is a reading of the machine at t.
func (m *machine) sample(s *story, t time.Time) protocol.HostSample {
	ts := t.Unix()
	tr := traffic(t)
	sd := func(what string) uint64 { return seed(m.Name, what) }
	cpu := clamp(m.cpuAt(s, t)+1.2*white(sd("cpu-jitter"), ts/5), 0.2, 99.9)
	iowait := 0.15 + 0.5*math.Abs(smooth(sd("iowait"), ts, 900)) + 0.2*tr
	backup := m.Backup && inWindow(t, 2*time.Hour, 2*time.Hour+40*time.Minute)
	target := m.BackupTarget && inWindow(t, 2*time.Hour, 3*time.Hour+30*time.Minute)
	if backup {
		iowait += 7 + 2*white(sd("iowait-jitter"), ts/15)
	}
	if target {
		iowait += 3
	}
	var steal float64
	if m.VPS {
		steal = clamp(0.4+0.6*smooth(sd("steal"), ts, 1800), 0, 5)
	}
	cores := float64(m.Cores)
	load1 := cores*cpu/100*1.1 + 0.03
	load5 := cores * m.cpuAt(s, t.Add(-150*time.Second)) / 100 * 1.05
	load15 := cores * m.cpuAt(s, t.Add(-450*time.Second)) / 100
	total := m.MemGiB * gib
	memUsed := total * clamp(m.Mem+m.MemPeak*tr+0.012*smooth(sd("mem"), ts, 1800), 0.05, 0.97)
	net := m.Net * (0.07 + tr) * (1 + 0.25*smooth(sd("net"), ts, 300) + 0.08*white(sd("net-jitter"), ts/5))
	rx, tx := net, net*m.TX
	read := m.Disk * 0.35 * (0.1 + tr) * (1 + 0.4*smooth(sd("read"), ts, 600))
	write := m.Disk * (0.15 + tr) * (1 + 0.3*smooth(sd("write"), ts, 300))
	if backup {
		read += 24e6 * (1 + 0.2*white(sd("backup"), ts/15))
		tx += 9e6 * (1 + 0.2*white(sd("backup-tx"), ts/15))
	}
	if target {
		rx += 12e6 * (1 + 0.25*smooth(sd("target"), ts, 120))
		write += 11e6 * (1 + 0.25*smooth(sd("target"), ts, 120))
	}
	x := protocol.HostSample{
		TS: ts, CPU: ptr(cpu), IOWait: ptr(iowait), Steal: ptr(steal),
		Load1: ptr(load1), Load5: ptr(load5), Load15: ptr(load15),
		MemUsed: ptr(int64(memUsed)), MemAvailable: ptr(int64(total - memUsed)),
		NetRx: ptr(rx), NetTx: ptr(tx), DiskRead: ptr(read), DiskWrite: ptr(write),
		Uptime: ptr(ts - m.bootTime(s)),
	}
	if m.SwapGiB > 0 {
		swapTotal := m.SwapGiB * gib
		x.SwapTotal = ptr(int64(swapTotal))
		x.SwapUsed = ptr(int64(swapTotal * clamp(m.SwapUsed+0.01*smooth(sd("swap"), ts, 7200), 0, 1)))
	} else {
		x.SwapTotal, x.SwapUsed = ptr(int64(0)), ptr(int64(0))
	}
	for _, f := range m.Filesystems {
		size := int64(f.GiB * gib)
		inodes := int64(f.GiB * 65536)
		x.Filesystems = append(x.Filesystems, protocol.Filesystem{
			Mount: f.Mount, Device: f.Device, FSType: f.FSType, Total: size,
			Used:       int64(float64(size) * f.usage(m, s, t)),
			InodesUsed: ptr(int64(float64(inodes) * (0.04 + 0.2*f.usage(m, s, t)))), InodesTotal: ptr(inodes),
		})
	}
	return x
}

// dockerID is a container's Docker ID, the same on every run.
func dockerID(host string, c container) string {
	var b strings.Builder
	for i := range 4 {
		fmt.Fprintf(&b, "%016x", mix(seed(host, c.name())+uint64(i)))
	}
	return b.String()
}

// stat is a reading of a running container at t.
func (c container) stat(m *machine, s *story, t time.Time) protocol.ContainerStat {
	ts := t.Unix()
	sd := func(what string) uint64 { return seed(m.Name, c.name(), what) }
	tr := traffic(t)
	cpu := c.CPU*(1+0.3*smooth(sd("cpu"), ts, 600)) + c.CPUPeak*tr*(1+0.2*smooth(sd("cpu-day"), ts, 900)) + 0.15*c.CPU*white(sd("cpu-jitter"), ts/5)
	if m.Name == "db-01" && c.Service == "postgres" && !t.Before(s.spikeFrom) && t.Before(s.spikeTo) {
		cpu = 610 + 30*white(sd("spike"), ts/15)
	}
	mem := c.MemMiB * (1 + 0.06*smooth(sd("mem"), ts, 3600) + 0.04*tr) * (1 << 20)
	net := c.Net * (0.08 + tr) * (1 + 0.25*smooth(sd("net"), ts, 300))
	st := protocol.ContainerStat{
		DockerID: dockerID(m.Name, c), CPU: ptr(math.Max(cpu, 0.02)), Mem: ptr(int64(mem)),
		NetRx: ptr(net), NetTx: ptr(net * 1.7),
	}
	if c.LimitMiB > 0 {
		st.MemLimit = ptr(int64(c.LimitMiB * (1 << 20)))
	} else {
		st.MemLimit = ptr(int64(m.MemGiB * gib))
	}
	return st
}

// latency is how long a site takes to answer at t: longer when it is
// busy. jitter is in [-1, 1].
func (s site) latency(t time.Time, jitter float64) time.Duration {
	return time.Duration(float64(s.Latency)*(0.85+0.35*traffic(t)) + jitter*float64(s.Jitter))
}

// site is a made-up web site, served by the demo itself.
type site struct {
	// Latency is how long it takes to answer, and Jitter how much that
	// varies.
	Latency, Jitter time.Duration
	// CertDays is how long its certificate is still valid.
	CertDays int
	// Status is what it answers now; 0 means 200.
	Status int
	Body   string
}

var sites = map[string]site{
	"shop.nordwind-logistik.example":         {Latency: 85 * time.Millisecond, Jitter: 40 * time.Millisecond, CertDays: 61, Body: "<h1>Nordwind Logistik – Shop</h1>"},
	"staging.shop.nordwind-logistik.example": {Latency: 140 * time.Millisecond, Jitter: 60 * time.Millisecond, CertDays: 9, Body: "<h1>Nordwind Logistik – Shop (staging)</h1>"},
	"erp.nordwind-logistik.example":          {Latency: 2100 * time.Millisecond, Jitter: 500 * time.Millisecond, CertDays: 47, Body: "<h1>Nordwind ERP – Login</h1>"},
	"www.baeckerei-sommer.example":           {Latency: 190 * time.Millisecond, Jitter: 80 * time.Millisecond, CertDays: 33, Body: "<h1>Bäckerei Sommer</h1><p>Frische Brötchen ab 6 Uhr.</p>"},
	"portal.atlas-health.example":            {Latency: 120 * time.Millisecond, Jitter: 45 * time.Millisecond, CertDays: 74, Body: "<h1>Atlas Health – Patient portal</h1>"},
	"api.atlas-health.example":               {Latency: 60 * time.Millisecond, Jitter: 25 * time.Millisecond, CertDays: 52, Status: 503, Body: `{"status":"unavailable","reason":"database pool exhausted"}`},
}

// Certificates of the services that are not web sites.
var certDays = map[string]int{"mx.ops.localhost": 80}

// The DNS records the demo's resolver knows.
var dnsRecords = map[string][]string{
	"shop.nordwind-logistik.example":         {"203.0.113.10"},
	"staging.shop.nordwind-logistik.example": {"203.0.113.24"},
	"erp.nordwind-logistik.example":          {"203.0.113.31"},
	"www.baeckerei-sommer.example":           {"198.51.100.7"},
	"portal.atlas-health.example":            {"192.0.2.41"},
	"api.atlas-health.example":               {"192.0.2.42"},
}

// environments are the paths of the environments that have machines.
func environments(ms []machine) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range ms {
		if !seen[m.Env] {
			seen[m.Env] = true
			out = append(out, m.Env)
		}
	}
	sort.Strings(out)
	return out
}

// configuration is the demo's lotse.yaml. The addresses of the services
// the demo plays are filled in.
func configuration(dns, mx, ssh string) string {
	return strings.NewReplacer("{{dns}}", dns, "{{mx}}", mx, "{{ssh}}", ssh).Replace(configYAML)
}

const configYAML = `# The demo hub's configuration: made-up customers, their projects and checks.
settings:
  rules.host.cpu.enabled: true
  notify.channels: [ops-mail]

channels:
  - key: ops-mail
    name: Ops on call
    email: {to: [oncall@ops.example]}
  - key: ops-slack
    name: "#ops-alerts"
    slack: {webhook: {from_env: DEMO_SLACK_WEBHOOK}}

customers:
  - key: nordwind
    name: Nordwind Logistik
    settings:
      notify.channels: [ops-mail, ops-slack]
    projects:
      - key: shop
        name: Online shop
        environments:
          - key: production
            name: Production
            maintenance:
              - schedule: sun 02:00-04:00
                reason: database maintenance
            checks:
              - key: homepage
                name: Homepage
                http: {url: "https://shop.nordwind-logistik.example/", keyword: Nordwind}
              - key: checkout-api
                name: Checkout API
                http: {url: "https://shop.nordwind-logistik.example/api/health", keyword: '"status":"ok"'}
              - key: shop-dns
                name: shop.nordwind-logistik.example
                dns: {name: shop.nordwind-logistik.example, type: A, expect: [203.0.113.10], server: "{{dns}}"}
            hosts:
              - key: db-01
                settings:
                  rules.host.disk.warn: 80
                  rules.host.disk.crit: 90
          - key: staging
            name: Staging
            settings:
              notify.channels: []
            checks:
              - key: homepage
                name: Homepage
                http: {url: "https://staging.shop.nordwind-logistik.example/"}
      - key: erp
        name: ERP
        environments:
          - key: production
            name: Production
            checks:
              - key: login
                name: ERP login
                http: {url: "https://erp.nordwind-logistik.example/web/login", keyword: Login}
              - key: stock-sync
                name: Stock sync
                heartbeat: {period: 5m, grace: 2m}

  - key: sommer
    name: Bäckerei Sommer
    projects:
      - key: website
        name: Website
        environments:
          - key: production
            name: Production
            checks:
              - key: website
                name: Website
                http: {url: "https://www.baeckerei-sommer.example/", keyword: Brötchen}

  - key: atlas
    name: Atlas Health
    channels:
      - key: atlas-it
        name: Atlas IT
        email: {to: [it-alerts@atlas-health.example]}
    settings:
      notify.channels: [ops-mail, atlas-it]
    projects:
      - key: portal
        name: Patient portal
        environments:
          - key: production
            name: Production
            checks:
              - key: portal
                name: Portal
                http: {url: "https://portal.atlas-health.example/", keyword: Atlas}
      - key: api
        name: API
        environments:
          - key: production
            name: Production
            checks:
              - key: health
                name: API health
                http: {url: "https://api.atlas-health.example/v2/health"}
              - key: nightly-export
                name: Nightly export
                heartbeat: {period: 24h, grace: 1h}

  - key: ops
    name: Ops
    projects:
      - key: infra
        name: Infrastructure
        environments:
          - key: production
            name: Production
            checks:
              - key: mail-relay
                name: Mail relay
                tls: {address: "{{mx}}"}
              - key: backup-ssh
                name: Backup SSH
                tcp: {address: "{{ssh}}"}
              - key: nightly-backup
                name: Nightly backup
                heartbeat: {period: 24h, grace: 2h}
`
