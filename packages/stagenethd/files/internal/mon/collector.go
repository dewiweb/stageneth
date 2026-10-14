package mon

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// Collector samples system state on a ticker and keeps a small history ring.
// Fast collectors (/proc, /sys, stdlib — zero forks) run every interval;
// exec-based collectors (pmc, ethtool, init.d, bridge) run every slowInterval.
type Collector struct {
	mu       sync.RWMutex
	latest   map[string]interface{}
	history  []map[string]interface{}
	maxHist  int
	interval time.Duration
	slow     time.Duration
	lastSlow time.Time
	slowData map[string]interface{}
	prevIdle uint64
	prevTot  uint64
	stopCh   chan struct{}
}

func NewCollector(interval, slow time.Duration) *Collector {
	return &Collector{
		interval: interval,
		slow:     slow,
		maxHist:  120, // ~10 min at 5s
		stopCh:   make(chan struct{}),
		slowData: map[string]interface{}{},
	}
}

// Latest returns the most recent snapshot, or nil if none yet.
func (c *Collector) Latest() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest
}

// History returns up to n most recent snapshots (oldest first).
func (c *Collector) History(n int) []map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if n <= 0 || n > len(c.history) {
		n = len(c.history)
	}
	out := make([]map[string]interface{}, n)
	copy(out, c.history[len(c.history)-n:])
	return out
}

// CollectNow runs one collection cycle synchronously.
func (c *Collector) CollectNow() map[string]interface{} {
	snap := c.collect()
	c.mu.Lock()
	c.latest = snap
	c.history = append(c.history, snap)
	if len(c.history) > c.maxHist {
		c.history = c.history[len(c.history)-c.maxHist:]
	}
	c.mu.Unlock()
	return snap
}

// Run collects immediately, then every interval, until Stop.
func (c *Collector) Run() {
	c.CollectNow()
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-t.C:
			c.CollectNow()
		}
	}
}

func (c *Collector) Stop() { close(c.stopCh) }

func (c *Collector) collect() map[string]interface{} {
	l1, l5, l15 := loadAvgs()
	uptime := firstFieldFloat("/proc/uptime")

	// CPU% from delta between consecutive /proc/stat reads.
	idle, tot := procStatCPU()
	cpu := 0.0
	if dTot := tot - c.prevTot; dTot > 0 && c.prevTot > 0 {
		cpu = 100.0 - float64(idle-c.prevIdle)*100.0/float64(dTot)
		cpu = float64(int64(cpu*100)) / 100.0
	}
	c.prevIdle, c.prevTot = idle, tot

	interfaces := parseNetDev()
	addrs := interfaceAddrs()
	serviceTraffic := map[string]interface{}{}
	for _, iface := range interfaces {
		name, _ := iface["name"].(string)
		if !strings.HasPrefix(name, "br_") {
			continue
		}
		svc := name[3:]
		serviceTraffic[svc] = map[string]interface{}{
			"rx_bytes": iface["rx_bytes"], "rx_packets": iface["rx_packets"],
			"tx_bytes": iface["tx_bytes"], "tx_packets": iface["tx_packets"],
			"ipaddr": addrs[name],
		}
	}
	cc, _ := strconv.ParseUint(readFileTrim("/proc/net/nf_conntrack_count"), 10, 64)
	cm, _ := strconv.ParseUint(readFileTrim("/proc/sys/net/netfilter/nf_conntrack_max"), 10, 64)

	data := map[string]interface{}{
		"collected_at":    time.Now().Unix(),
		"load":            l1,
		"load1":           l1,
		"load5":           l5,
		"load15":          l15,
		"uptime_seconds":  uint64(uptime),
		"cpu_percent":     cpu,
		"memory":          parseMemInfo(),
		"storage":         parseDiskUsage(),
		"interfaces":      interfaces,
		"interface_addrs": addrs,
		"dhcp_leases":     parseDHCPLeases(),
		"service_traffic": serviceTraffic,
		"igmp_groups":     parseIGMP(),
		"igmp_snooping":   parseIGMPSnooping(),
		"connections":     map[string]uint64{"current": cc, "max": cm},
	}

	// exec-based collectors, on the slow cadence
	if c.lastSlow.IsZero() || time.Since(c.lastSlow) >= c.slow {
		c.slowData = map[string]interface{}{
			"services":   serviceStatuses(),
			"ptp_status": parsePtpStatus(),
			"nic_stats":  parseEthtoolStats(),
			"bridge_mdb": parseBridgeMdb(),
		}
		c.lastSlow = time.Now()
	}
	for k, v := range c.slowData {
		data[k] = v
	}
	return data
}
