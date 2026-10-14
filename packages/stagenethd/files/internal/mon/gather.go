// Package mon collects system state on a ticker so HTTP handlers serve
// O(1) responses instead of forking subprocesses per request.
package mon

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func readFileTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func firstFieldFloat(path string) float64 {
	if f := strings.Fields(readFileTrim(path)); len(f) > 0 {
		v, _ := strconv.ParseFloat(f[0], 64)
		return v
	}
	return 0
}

func parseFloatDefault(s string, def float64) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

func loadAvgs() (float64, float64, float64) {
	f := strings.Fields(readFileTrim("/proc/loadavg"))
	l := make([]float64, 3)
	for i := range l {
		if i < len(f) {
			l[i], _ = strconv.ParseFloat(f[i], 64)
		}
	}
	return l[0], l[1], l[2]
}

// procStatCPU returns (idle, total) jiffies from /proc/stat.
func procStatCPU() (uint64, uint64) {
	for _, line := range strings.Split(readFileTrim("/proc/stat"), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 8 {
			return 0, 0
		}
		var idle, total uint64
		for i := 0; i < 7; i++ {
			v, _ := strconv.ParseUint(f[i+1], 10, 64)
			total += v
			if i == 3 || i == 4 {
				idle += v
			}
		}
		return idle, total
	}
	return 0, 0
}

func parseMemInfo() map[string]interface{} {
	data := map[string]interface{}{"total_kb": uint64(1), "free_kb": uint64(0), "available_kb": uint64(0), "used_percent": 0.0}
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return data
	}
	vals := map[string]uint64{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(f[1], 10, 64)
		vals[strings.TrimSuffix(f[0], ":")] = v
	}
	total := vals["MemTotal"]
	if total == 0 {
		return data
	}
	available := vals["MemAvailable"]
	if available > total {
		available = total
	}
	used := total - available
	data["total_kb"] = total
	data["free_kb"] = vals["MemFree"]
	data["available_kb"] = available
	data["used_kb"] = used
	data["used_percent"] = float64(int64(float64(used)*10000.0/float64(total))) / 100.0
	return data
}

// pseudo filesystems not worth reporting as storage.
var pseudoFS = map[string]bool{
	"proc": true, "sysfs": true, "devpts": true, "devtmpfs": true,
	"cgroup": true, "cgroup2": true, "pstore": true, "bpf": true,
	"tracefs": true, "debugfs": true, "configfs": true, "fusectl": true,
	"securityfs": true, "mqueue": true, "hugetlbfs": true, "rpc_pipefs": true,
}

// parseDiskUsage replaces `df -P` with /proc/mounts + syscall.Statfs.
func parseDiskUsage() map[string]interface{} {
	data := map[string]interface{}{"total_kb": uint64(1), "used_kb": uint64(0), "available_kb": uint64(0), "used_percent": 0.0, "partitions": []map[string]interface{}{}}
	partitions := []map[string]interface{}{}
	seen := map[string]bool{}
	for _, line := range strings.Split(readFileTrim("/proc/mounts"), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || pseudoFS[f[2]] {
			continue
		}
		mount := strings.ReplaceAll(f[1], `\040`, " ")
		if seen[mount] {
			continue
		}
		var st syscall.Statfs_t
		if syscall.Statfs(mount, &st) != nil || st.Blocks == 0 {
			continue
		}
		seen[mount] = true
		bsize := uint64(st.Bsize)
		total := st.Blocks * bsize / 1024
		free := st.Bfree * bsize / 1024
		available := st.Bavail * bsize / 1024
		used := total - free
		p := 0.0
		if total > 0 {
			p = float64(used) * 100.0 / float64(total)
		}
		partitions = append(partitions, map[string]interface{}{
			"filesystem":   f[0],
			"mount":        mount,
			"total_kb":     total,
			"used_kb":      used,
			"available_kb": available,
			"used_percent": p,
		})
		if mount == "/" {
			data["total_kb"] = total
			data["used_kb"] = used
			data["available_kb"] = available
			data["used_percent"] = p
		}
	}
	data["partitions"] = partitions
	return data
}

func parseNetDev() []map[string]interface{} {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil
	}
	var out []map[string]interface{}
	for i, line := range strings.Split(string(b), "\n") {
		if i < 2 || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		name := strings.TrimSpace(parts[0])
		if name == "lo" {
			continue
		}
		f := strings.Fields(parts[1])
		if len(f) < 10 {
			continue
		}
		rxBytes, _ := strconv.ParseUint(f[0], 10, 64)
		rxPackets, _ := strconv.ParseUint(f[1], 10, 64)
		txBytes, _ := strconv.ParseUint(f[8], 10, 64)
		txPackets, _ := strconv.ParseUint(f[9], 10, 64)
		out = append(out, map[string]interface{}{
			"name": name, "rx_bytes": rxBytes, "rx_packets": rxPackets,
			"tx_bytes": txBytes, "tx_packets": txPackets,
		})
	}
	return out
}

// interfaceAddrs replaces `ip -o -4 addr show` with stdlib netlink calls.
func interfaceAddrs() map[string]string {
	addrs := map[string]string{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return addrs
	}
	for _, ifi := range ifaces {
		list, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range list {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			if addrs[ifi.Name] == "" {
				addrs[ifi.Name] = ipnet.IP.String()
			}
		}
	}
	return addrs
}

func parseDHCPLeases() []map[string]string {
	b, err := os.ReadFile("/tmp/dhcp.leases")
	if err != nil {
		return nil
	}
	var out []map[string]string
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		out = append(out, map[string]string{
			"timestamp": f[0], "mac": f[1], "ip": f[2],
			"hostname": f[3], "client_id": f[4],
		})
	}
	return out
}

func parseIGMP() map[string][]map[string]interface{} {
	b, err := os.ReadFile("/proc/net/igmp")
	if err != nil {
		return nil
	}
	out := map[string][]map[string]interface{}{}
	var current string
	for _, line := range strings.Split(string(b), "\n") {
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 2 {
			if _, err := strconv.Atoi(f[0]); err == nil {
				current = strings.TrimSuffix(f[1], ":")
				continue
			}
		}
		if len(f) < 4 || len(f[0]) != 8 {
			continue
		}
		var ip [4]byte
		for i := 0; i < 4; i++ {
			v, _ := strconv.ParseUint(f[0][2*i:2*i+2], 16, 8)
			ip[3-i] = byte(v)
		}
		out[current] = append(out[current], map[string]interface{}{
			"group_hex": f[0],
			"ip":        fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3]),
			"users":     f[1], "timer": f[2], "reporter": f[3],
		})
	}
	return out
}

func parseIGMPSnooping() map[string]bool {
	out := map[string]bool{}
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return out
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "br_") {
			continue
		}
		out[name] = readFileTrim("/sys/class/net/"+name+"/bridge/multicast_snooping") == "1"
	}
	return out
}

// --- slow collectors (exec-based) --------------------------------------------

// execBounded runs an external command with a hard timeout — a stuck
// driver/init script must never freeze the collector goroutine.
func execBounded(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func serviceStatuses() map[string]bool {
	out := map[string]bool{}
	for _, svc := range []string{"network", "firewall", "dnsmasq", "stageneth-api", "nginx"} {
		if svc == "firewall" {
			b, _ := execBounded(3*time.Second, "/etc/init.d/firewall", "status")
			out[svc] = strings.TrimSpace(string(b)) == "active"
			continue
		}
		_, err := execBounded(3*time.Second, "/etc/init.d/"+svc, "running")
		out[svc] = err == nil
	}
	return out
}

func parsePtpStatus() map[string]interface{} {
	out := map[string]interface{}{"state": "unknown", "offset_ns": 0.0, "delay_ns": 0.0}
	pmc := func(args ...string) ([]byte, error) {
		return execBounded(3*time.Second, "pmc", args...)
	}
	portOut, err := pmc("-u", "-b", "0", "GET PORT_DATA_SET")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(portOut), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "portState") {
			if f := strings.Fields(line); len(f) >= 2 {
				out["state"] = f[1]
			}
		}
	}
	currentOut, err := pmc("-u", "-b", "0", "GET CURRENT_DATA_SET")
	if err == nil {
		for _, line := range strings.Split(string(currentOut), "\n") {
			line = strings.TrimSpace(line)
			f := strings.Fields(line)
			if len(f) < 2 {
				continue
			}
			switch f[0] {
			case "offsetFromMaster":
				out["offset_ns"] = parseFloatDefault(f[1], 0.0)
			case "meanPathDelay":
				out["delay_ns"] = parseFloatDefault(f[1], 0.0)
			}
		}
	}
	return out
}

func parseEthtoolStats() map[string]interface{} {
	out := map[string]interface{}{}
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return out
	}
	relevant := map[string]bool{
		"rx_errors": true, "tx_errors": true, "rx_dropped": true, "tx_dropped": true,
		"rx_crc_errors": true, "rx_length_errors": true, "rx_over_errors": true,
		"rx_frame_errors": true, "rx_missed_errors": true, "rx_no_buffer_count": true,
		"rx_align_errors": true, "rx_short_length_errors": true, "rx_long_length_errors": true,
		"tx_missed_errors": true, "tx_carrier_errors": true, "tx_aborted_errors": true,
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "lo") || strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "bond") ||
			strings.Contains(name, ".") || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "wg") ||
			strings.HasPrefix(name, "tun") {
			continue
		}
		b, err := execBounded(2*time.Second, "ethtool", "-S", name)
		if err != nil {
			continue
		}
		stats := map[string]interface{}{}
		for _, line := range strings.Split(string(b), "\n") {
			parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
			if len(parts) != 2 || !relevant[strings.TrimSpace(parts[0])] {
				continue
			}
			v, _ := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
			stats[strings.TrimSpace(parts[0])] = v
		}
		if len(stats) > 0 {
			out[name] = stats
		}
	}
	return out
}

func parseBridgeMdb() map[string]interface{} {
	out := map[string]interface{}{}
	b, err := execBounded(3*time.Second, "bridge", "mdb", "show")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		var bridge, port, grp, vid string
		for i := 0; i < len(f); i++ {
			if i+1 >= len(f) {
				break
			}
			switch f[i] {
			case "dev":
				bridge = f[i+1]
			case "port":
				port = f[i+1]
			case "grp":
				grp = f[i+1]
			case "vid":
				vid = f[i+1]
			}
		}
		if bridge == "" {
			continue
		}
		if _, ok := out[bridge]; !ok {
			out[bridge] = []map[string]string{}
		}
		out[bridge] = append(out[bridge].([]map[string]string),
			map[string]string{"port": port, "group": grp, "vid": vid, "raw": line})
	}
	return out
}
