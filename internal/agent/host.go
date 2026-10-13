package agent

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/common"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"

	"lotse.sh/internal/protocol"
)

// hostCollector reads the machine. Root is where the machine's own
// filesystem is: "/" for an agent installed on it, or wherever the
// machine's / is mounted into the agent's container, such as /host.
//
// Rates (CPU, network, disk I/O) need two readings, so the first sample has
// none.
type hostCollector struct {
	root string

	prevAt   time.Time
	prevCPU  cpu.TimesStat
	prevNet  [2]uint64 // received, sent
	prevDisk [2]uint64 // read, written
}

func newHostCollector(root string) *hostCollector {
	return &hostCollector{root: root}
}

// ctx makes gopsutil read the machine through root.
func (c *hostCollector) ctx(ctx context.Context) context.Context {
	if c.root == "/" || c.root == "" {
		return ctx
	}
	return context.WithValue(ctx, common.EnvKey, common.EnvMap{
		common.HostProcEnvKey: filepath.Join(c.root, "proc"),
		common.HostSysEnvKey:  filepath.Join(c.root, "sys"),
		common.HostEtcEnvKey:  filepath.Join(c.root, "etc"),
		common.HostVarEnvKey:  filepath.Join(c.root, "var"),
		common.HostRunEnvKey:  filepath.Join(c.root, "run"),
		common.HostDevEnvKey:  filepath.Join(c.root, "dev"),
		common.HostRootEnvKey: c.root,
	})
}

func (c *hostCollector) path(p ...string) string {
	return filepath.Join(append([]string{c.root}, p...)...)
}

// hello describes the machine.
func (c *hostCollector) hello(ctx context.Context) protocol.Hello {
	ctx = c.ctx(ctx)
	h := protocol.Hello{
		Hostname:  c.hostname(),
		MachineID: c.machineID(),
	}
	if info, err := host.InfoWithContext(ctx); err == nil {
		h.OS = info.OS
		h.Platform = strings.TrimSpace(info.Platform + " " + info.PlatformVersion)
		h.Kernel = info.KernelVersion
		h.Arch = info.KernelArch
		h.BootTime = int64(info.BootTime)
	}
	if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
		h.CPUModel = infos[0].ModelName
	}
	if n, err := cpu.CountsWithContext(ctx, true); err == nil {
		h.Cores = n
	}
	if m, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		h.MemTotal = int64(m.Total)
	}
	return h
}

// hostname is the machine's name, not that of the agent's container.
func (c *hostCollector) hostname() string {
	if c.root != "/" && c.root != "" {
		if b, err := os.ReadFile(c.path("etc", "hostname")); err == nil {
			if name := strings.TrimSpace(string(b)); name != "" {
				return name
			}
		}
	}
	name, _ := os.Hostname()
	return name
}

// machineID identifies the machine across reinstalls of the agent.
func (c *hostCollector) machineID() string {
	b, err := os.ReadFile(c.path("etc", "machine-id"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// sample reads the machine now. What cannot be read is left out.
func (c *hostCollector) sample(ctx context.Context, now time.Time) protocol.HostSample {
	ctx = c.ctx(ctx)
	s := protocol.HostSample{TS: now.Unix()}
	elapsed := now.Sub(c.prevAt).Seconds()
	first := c.prevAt.IsZero()
	c.prevAt = now

	if times, err := cpu.TimesWithContext(ctx, false); err == nil && len(times) == 1 {
		t := times[0]
		if !first {
			s.CPU, s.IOWait, s.Steal = cpuShares(c.prevCPU, t)
		}
		c.prevCPU = t
	}
	if l, err := load.AvgWithContext(ctx); err == nil {
		s.Load1, s.Load5, s.Load15 = &l.Load1, &l.Load5, &l.Load15
	}
	if m, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		used, avail := int64(m.Total-m.Available), int64(m.Available)
		s.MemUsed, s.MemAvailable = &used, &avail
	}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		used, total := int64(sw.Used), int64(sw.Total)
		s.SwapUsed, s.SwapTotal = &used, &total
	}
	if up, err := host.UptimeWithContext(ctx); err == nil {
		u := int64(up)
		s.Uptime = &u
	}
	if rx, tx, ok := c.network(ctx); ok {
		if !first && elapsed > 0 {
			s.NetRx, s.NetTx = rate(c.prevNet[0], rx, elapsed), rate(c.prevNet[1], tx, elapsed)
		}
		c.prevNet = [2]uint64{rx, tx}
	}
	if r, w, ok := c.diskIO(ctx); ok {
		if !first && elapsed > 0 {
			s.DiskRead, s.DiskWrite = rate(c.prevDisk[0], r, elapsed), rate(c.prevDisk[1], w, elapsed)
		}
		c.prevDisk = [2]uint64{r, w}
	}
	s.Filesystems = c.filesystems(ctx)
	return s
}

// cpuShares returns the busy, I/O wait and steal time between two readings,
// in percent of all CPU time.
func cpuShares(prev, cur cpu.TimesStat) (busy, iowait, steal *float64) {
	total := func(t cpu.TimesStat) float64 {
		// Guest time is already counted in user time.
		return t.User + t.Nice + t.System + t.Idle + t.Iowait + t.Irq + t.Softirq + t.Steal
	}
	d := total(cur) - total(prev)
	if d <= 0 {
		return nil, nil, nil
	}
	idle := (cur.Idle - prev.Idle) + (cur.Iowait - prev.Iowait)
	b := 100 * (d - idle) / d
	w := 100 * (cur.Iowait - prev.Iowait) / d
	st := 100 * (cur.Steal - prev.Steal) / d
	return &b, &w, &st
}

// rate is how fast a counter grew, per second. A counter that went back
// was reset (a reboot, a device that came again) and says nothing.
func rate(prev, cur uint64, seconds float64) *float64 {
	if cur < prev {
		return nil
	}
	r := float64(cur-prev) / seconds
	return &r
}

// network sums the traffic of the machine's physical interfaces: those with
// a device behind them. Bridges, veth pairs and tunnels only pass on what a
// physical interface carries as well. From /proc/1/net/dev, which shows the
// machine's network even from inside a container.
func (c *hostCollector) network(ctx context.Context) (rx, tx uint64, ok bool) {
	counters, err := net.IOCountersByFileWithContext(ctx, true, c.path("proc", "1", "net", "dev"))
	if err != nil {
		counters, err = net.IOCountersWithContext(ctx, true)
		if err != nil {
			return 0, 0, false
		}
	}
	physical := func(name string) bool {
		_, err := os.Stat(c.path("sys", "class", "net", name, "device"))
		return err == nil
	}
	found := false
	for _, n := range counters {
		if physical(n.Name) {
			rx, tx, found = rx+n.BytesRecv, tx+n.BytesSent, true
		}
	}
	if found {
		return rx, tx, true
	}
	// No interface with a device, as in some virtual machines: everything
	// but the obviously virtual.
	for _, n := range counters {
		if n.Name == "lo" || hasAnyPrefix(n.Name, "veth", "br-", "docker", "virbr", "cni", "flannel", "cali") {
			continue
		}
		rx, tx = rx+n.BytesRecv, tx+n.BytesSent
	}
	return rx, tx, true
}

// diskIO sums reads and writes of whole disks. Partitions, device mapper
// and RAID devices would count the same bytes again.
func (c *hostCollector) diskIO(ctx context.Context) (read, written uint64, ok bool) {
	counters, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return 0, 0, false
	}
	for name, d := range counters {
		if hasAnyPrefix(name, "loop", "ram", "zram", "dm-", "md", "sr", "nbd") {
			continue
		}
		if _, err := os.Stat(c.path("sys", "block", name)); err != nil {
			continue // a partition
		}
		read, written = read+d.ReadBytes, written+d.WriteBytes
	}
	return read, written, true
}

// Filesystems that hold data. Everything else (tmpfs, overlay, squashfs,
// the EFI partition, …) is left out.
var dataFilesystems = []string{"ext2", "ext3", "ext4", "xfs", "btrfs", "zfs", "f2fs", "bcachefs", "jfs", "reiserfs"}

// filesystems reports each device once, at its shortest mount point. It
// asks for all mounts, binds included, because on some machines / itself is
// a bind mount (a directory of a btrfs subvolume, for one), which gopsutil
// would otherwise leave out.
func (c *hostCollector) filesystems(ctx context.Context) []protocol.Filesystem {
	parts, err := disk.PartitionsWithContext(ctx, true)
	if err != nil {
		return nil
	}
	slices.SortFunc(parts, func(a, b disk.PartitionStat) int { return len(a.Mountpoint) - len(b.Mountpoint) })
	seen := map[string]bool{}
	var list []protocol.Filesystem
	for _, p := range parts {
		if !slices.Contains(dataFilesystems, p.Fstype) || seen[p.Device] {
			continue
		}
		seen[p.Device] = true
		u, err := disk.UsageWithContext(ctx, c.path(p.Mountpoint))
		if err != nil || u.Total == 0 {
			continue
		}
		fs := protocol.Filesystem{
			Mount: p.Mountpoint, Device: p.Device, FSType: p.Fstype,
			Used: int64(u.Used), Total: int64(u.Total),
		}
		if u.InodesTotal > 0 {
			iu, it := int64(u.InodesUsed), int64(u.InodesTotal)
			fs.InodesUsed, fs.InodesTotal = &iu, &it
		}
		list = append(list, fs)
	}
	return list
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
