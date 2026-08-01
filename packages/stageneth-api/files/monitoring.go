package main

import (
	"context"
	"fmt"
	http "net/http"
	os "os"
	exec "os/exec"
	strconv "strconv"
	strings "strings"
	time "time"
)

func parseMemInfo() map[string]interface{} {
	data := map[string]interface{}{"total_kb": 1, "free_kb": 1, "available_kb": 1, "used_percent": 0.0}
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
		name := strings.TrimSuffix(f[0], ":")
		v, _ := strconv.ParseUint(f[1], 10, 64)
		vals[name] = v
	}
	total := vals["MemTotal"]
	free := vals["MemFree"]
	available := vals["MemAvailable"]
	used := total - available
	usedPercent := 0.0
	if total > 0 {
		usedPercent = float64(used) * 100.0 / float64(total)
	}
	data["total_kb"] = total
	data["free_kb"] = free
	data["available_kb"] = available
	data["used_kb"] = used
	data["used_percent"] = float64(int64(usedPercent*100)) / 100.0
	return data
}
func parseDiskUsage() map[string]interface{} {
	data := map[string]interface{}{"total_kb": 1, "used_kb": 0, "available_kb": 0, "used_percent": 0.0, "partitions": []map[string]interface{}{}}
	out, err := exec.Command("df", "-P").Output()
	if err != nil {
		return data
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	partitions := []map[string]interface{}{}
	seen := map[string]bool{}
	for i, line := range lines {
		if i == 0 {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		mount := f[5]
		if seen[mount] {
			continue
		}
		seen[mount] = true
		total, _ := strconv.ParseUint(f[1], 10, 64)
		used, _ := strconv.ParseUint(f[2], 10, 64)
		available, _ := strconv.ParseUint(f[3], 10, 64)
		p, _ := strconv.ParseFloat(strings.TrimSuffix(f[4], "%"), 64)
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
	if data["total_kb"].(uint64) == 0 {
		data["total_kb"] = 1
	}
	data["partitions"] = partitions
	return data
}
func cpuUsage() float64 {
	readStat := func() (uint64, uint64) {
		s := readFileTrim("/proc/stat")
		for _, line := range strings.Split(s, "\n") {
			if !strings.HasPrefix(line, "cpu ") {
				continue
			}
			f := strings.Fields(line)
			if len(f) < 8 {
				continue
			}
			var vals [7]uint64
			total := uint64(0)
			for i := 0; i < 7; i++ {
				vals[i], _ = strconv.ParseUint(f[i+1], 10, 64)
				total += vals[i]
			}
			idle := vals[3] + vals[4]
			return idle, total
		}
		return 0, 0
	}
	idle1, total1 := readStat()
	time.Sleep(500 * time.Millisecond)
	idle2, total2 := readStat()
	diffTotal := total2 - total1
	diffIdle := idle2 - idle1
	if diffTotal == 0 {
		return 0.0
	}
	p := 100.0 - (float64(diffIdle) * 100.0 / float64(diffTotal))
	return float64(int64(p*100)) / 100.0
}
func parseNetDev() []map[string]interface{} {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil
	}
	var out []map[string]interface{}
	lines := strings.Split(string(b), "\n")
	for i, line := range lines {
		if i < 2 || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.Split(line, ":")
		name := strings.TrimSpace(parts[0])
		if name == "lo" {
			continue
		}
		f := strings.Fields(parts[1])
		if len(f) < 9 {
			continue
		}
		rxBytes, _ := strconv.ParseUint(f[0], 10, 64)
		rxPackets, _ := strconv.ParseUint(f[1], 10, 64)
		txBytes, _ := strconv.ParseUint(f[8], 10, 64)
		txPackets, _ := strconv.ParseUint(f[9], 10, 64)
		out = append(out, map[string]interface{}{
			"name":       name,
			"rx_bytes":   rxBytes,
			"rx_packets": rxPackets,
			"tx_bytes":   txBytes,
			"tx_packets": txPackets,
		})
	}
	return out
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
			"timestamp": f[0],
			"mac":       f[1],
			"ip":        f[2],
			"hostname":  f[3],
			"client_id": f[4],
		})
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
		b, err := exec.Command("ethtool", "-S", name).CombinedOutput()
		if err != nil {
			continue
		}
		stats := map[string]interface{}{}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			if !relevant[key] {
				continue
			}
			val := strings.TrimSpace(parts[1])
			v, _ := strconv.ParseUint(val, 10, 64)
			stats[key] = v
		}
		if len(stats) > 0 {
			out[name] = stats
		}
	}
	return out
}

func parseBridgeMdb() map[string]interface{} {
	out := map[string]interface{}{}
	b, err := exec.Command("bridge", "mdb", "show").CombinedOutput()
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
		bridge := ""
		port := ""
		grp := ""
		vid := ""
		for i := 0; i < len(f); i++ {
			if f[i] == "dev" && i+1 < len(f) {
				bridge = f[i+1]
			} else if f[i] == "port" && i+1 < len(f) {
				port = f[i+1]
			} else if f[i] == "grp" && i+1 < len(f) {
				grp = f[i+1]
			} else if f[i] == "vid" && i+1 < len(f) {
				vid = f[i+1]
			}
		}
		if bridge == "" {
			continue
		}
		if _, ok := out[bridge]; !ok {
			out[bridge] = []map[string]string{}
		}
		out[bridge] = append(out[bridge].([]map[string]string), map[string]string{"port": port, "group": grp, "vid": vid, "raw": line})
	}
	return out
}

func parsePtpStatus() map[string]interface{} {
	out := map[string]interface{}{"state": "unknown", "offset_ns": 0.0, "delay_ns": 0.0}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	portOut, err := exec.CommandContext(ctx, "pmc", "-u", "-b", "0", "GET PORT_DATA_SET").CombinedOutput()
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(portOut), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "portState") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				out["state"] = f[1]
			}
		}
	}

	currentOut, err := exec.CommandContext(ctx, "pmc", "-u", "-b", "0", "GET CURRENT_DATA_SET").CombinedOutput()
	if err == nil {
		for _, line := range strings.Split(string(currentOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "offsetFromMaster") {
				f := strings.Fields(line)
				if len(f) >= 2 {
					out["offset_ns"] = parseFloatDefault(f[1], 0.0)
				}
			} else if strings.HasPrefix(line, "meanPathDelay") {
				f := strings.Fields(line)
				if len(f) >= 2 {
					out["delay_ns"] = parseFloatDefault(f[1], 0.0)
				}
			}
		}
	}
	return out
}

func parseFloatDefault(s string, def float64) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

func serviceStatuses() map[string]bool {
	out := map[string]bool{}
	for _, svc := range []string{"network", "firewall", "dnsmasq", "stageneth-api", "nginx"} {
		if svc == "firewall" {
			b, _ := exec.Command("/etc/init.d/firewall", "status").Output()
			out[svc] = strings.Contains(string(b), "active")
			continue
		}
		cmd := exec.Command("/etc/init.d/"+svc, "running")
		err := cmd.Run()
		out[svc] = err == nil
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
		if len(line) == 0 {
			continue
		}
		if f := strings.Fields(line); len(f) >= 2 {
			if _, err := strconv.Atoi(f[0]); err == nil {
				current = strings.TrimSuffix(f[1], ":")
				continue
			}
		}
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		groupHex := f[0]
		if len(groupHex) != 8 {
			continue
		}
		var ip [4]byte
		for i := 0; i < 4; i++ {
			v, _ := strconv.ParseUint(groupHex[2*i:2*i+2], 16, 8)
			ip[3-i] = byte(v)
		}
		entry := map[string]interface{}{
			"group_hex": groupHex,
			"ip":        fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3]),
			"users":     f[1],
			"timer":     f[2],
			"reporter":  f[3],
		}
		out[current] = append(out[current], entry)
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
		b, err := os.ReadFile("/sys/class/net/" + name + "/bridge/multicast_snooping")
		if err != nil {
			continue
		}
		out[name] = strings.TrimSpace(string(b)) == "1"
	}
	return out
}
func monitoringSummary(w http.ResponseWriter, r *http.Request) {
	load, _ := strconv.ParseFloat(strings.Fields(readFileTrim("/proc/loadavg"))[0], 64)
	uptime, _ := strconv.ParseFloat(strings.Fields(readFileTrim("/proc/uptime"))[0], 64)
	conntrackCurrent := readFileTrim("/proc/net/nf_conntrack_count")
	conntrackMax := readFileTrim("/proc/sys/net/netfilter/nf_conntrack_max")
	cc, _ := strconv.ParseUint(conntrackCurrent, 10, 64)
	cm, _ := strconv.ParseUint(conntrackMax, 10, 64)
	interfaces := parseNetDev()
	addrs := parseInterfaceAddrs()
	serviceTraffic := map[string]interface{}{}
	for _, iface := range interfaces {
		name, _ := iface["name"].(string)
		if !strings.HasPrefix(name, "br_") {
			continue
		}
		svc := name[3:]
		serviceTraffic[svc] = map[string]interface{}{
			"rx_bytes":   iface["rx_bytes"],
			"rx_packets": iface["rx_packets"],
			"tx_bytes":   iface["tx_bytes"],
			"tx_packets": iface["tx_packets"],
			"ipaddr":     addrs[name],
		}
	}
	data := map[string]interface{}{
		"load":            load,
		"uptime_seconds":  uint64(uptime),
		"cpu_percent":     cpuUsage(),
		"memory":          parseMemInfo(),
		"storage":         parseDiskUsage(),
		"interfaces":      interfaces,
		"interface_addrs": addrs,
		"dhcp_leases":     parseDHCPLeases(),
		"services":        serviceStatuses(),
		"service_traffic": serviceTraffic,
		"igmp_groups":     parseIGMP(),
		"igmp_snooping":   parseIGMPSnooping(),
		"ptp_status":      parsePtpStatus(),
		"nic_stats":       parseEthtoolStats(),
		"bridge_mdb":      parseBridgeMdb(),
		"connections":     map[string]uint64{"current": cc, "max": cm},
	}
	respond(w, 200, data, "monitoring summary")
}

func metricsPrometheus(w http.ResponseWriter, r *http.Request) {
	load, _ := strconv.ParseFloat(strings.Fields(readFileTrim("/proc/loadavg"))[0], 64)
	uptime, _ := strconv.ParseFloat(strings.Fields(readFileTrim("/proc/uptime"))[0], 64)
	mem := parseMemInfo()
	interfaces := parseNetDev()
	addrs := parseInterfaceAddrs()
	services := serviceStatuses()
	ptp := parsePtpStatus()
	conntrackCurrent := readFileTrim("/proc/net/nf_conntrack_count")
	conntrackMax := readFileTrim("/proc/sys/net/netfilter/nf_conntrack_max")
	cc, _ := strconv.ParseUint(conntrackCurrent, 10, 64)
	cm, _ := strconv.ParseUint(conntrackMax, 10, 64)
	dhcpLeases := parseDHCPLeases()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "# HELP stageneth_cpu_percent CPU usage percent\n# TYPE stageneth_cpu_percent gauge\nstageneth_cpu_percent %.2f\n", cpuUsage())
	fmt.Fprintf(w, "# HELP stageneth_load Load average\n# TYPE stageneth_load gauge\nstageneth_load %.3f\n", load)
	fmt.Fprintf(w, "# HELP stageneth_uptime_seconds Uptime in seconds\n# TYPE stageneth_uptime_seconds counter\nstageneth_uptime_seconds %d\n", uint64(uptime))
	fmt.Fprintf(w, "# HELP stageneth_memory_used_percent Memory used percent\n# TYPE stageneth_memory_used_percent gauge\nstageneth_memory_used_percent %.2f\n", mem["used_percent"])
	fmt.Fprintf(w, "# HELP stageneth_memory_total_kb Memory total in kB\n# TYPE stageneth_memory_total_kb gauge\nstageneth_memory_total_kb %d\n", mem["total_kb"])
	fmt.Fprintf(w, "# HELP stageneth_connections_current Current conntrack connections\n# TYPE stageneth_connections_current gauge\nstageneth_connections_current %d\n", cc)
	fmt.Fprintf(w, "# HELP stageneth_connections_max Max conntrack connections\n# TYPE stageneth_connections_max gauge\nstageneth_connections_max %d\n", cm)
	fmt.Fprintf(w, "# HELP stageneth_dhcp_leases_count Number of DHCP leases\n# TYPE stageneth_dhcp_leases_count gauge\nstageneth_dhcp_leases_count %d\n", len(dhcpLeases))
	fmt.Fprintf(w, "# HELP stageneth_ptp_offset_ns PTP offset\n# TYPE stageneth_ptp_offset_ns gauge\nstageneth_ptp_offset_ns %.3f\n", ptp["offset_ns"])
	fmt.Fprintf(w, "# HELP stageneth_ptp_delay_ns PTP delay\n# TYPE stageneth_ptp_delay_ns gauge\nstageneth_ptp_delay_ns %.3f\n", ptp["delay_ns"])
	fmt.Fprintln(w, "# HELP stageneth_net_rx_bytes Received bytes per interface")
	fmt.Fprintln(w, "# TYPE stageneth_net_rx_bytes gauge")
	for _, iface := range interfaces {
		name, _ := iface["name"].(string)
		fmt.Fprintf(w, "stageneth_net_rx_bytes{name=%q} %d\n", name, iface["rx_bytes"])
		fmt.Fprintf(w, "stageneth_net_tx_bytes{name=%q} %d\n", name, iface["tx_bytes"])
		fmt.Fprintf(w, "stageneth_net_rx_packets{name=%q} %d\n", name, iface["rx_packets"])
		fmt.Fprintf(w, "stageneth_net_tx_packets{name=%q} %d\n", name, iface["tx_packets"])
		ip := addrs[name]
		if ip != "" {
			fmt.Fprintf(w, "stageneth_interface_ip{name=%q,ip=%q} 1\n", name, ip)
		}
	}
	fmt.Fprintln(w, "# HELP stageneth_service_up Service is running")
	fmt.Fprintln(w, "# TYPE stageneth_service_up gauge")
	for svc, up := range services {
		upVal := 0
		if up {
			upVal = 1
		}
		fmt.Fprintf(w, "stageneth_service_up{name=%q} %d\n", svc, upVal)
	}
}
