package api

import (
	fmt "fmt"
	http "net/http"
	sort "sort"
	strconv "strconv"
	strings "strings"

	"stageneth/internal/mon"
)

var monitor *mon.Collector

// InitMonitor attaches the running collector used by monitoring endpoints.
func InitMonitor(c *mon.Collector) { monitor = c }

func snapshot() map[string]interface{} {
	if monitor == nil {
		return nil
	}
	if snap := monitor.Latest(); snap != nil {
		return snap
	}
	return monitor.CollectNow()
}

func monitoringSummary(w http.ResponseWriter, r *http.Request) {
	snap := snapshot()
	if snap == nil {
		respond(w, 503, nil, "monitor unavailable")
		return
	}
	respond(w, 200, snap, "monitoring summary")
}

// --- typed accessors over the snapshot map ------------------------------------

func mapFloat(m map[string]interface{}, k string) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	case uint64:
		return float64(v)
	case int:
		return float64(v)
	}
	return 0
}
func mapUint(m map[string]interface{}, k string) uint64 {
	switch v := m[k].(type) {
	case uint64:
		return v
	case int64:
		return uint64(v)
	case int:
		return uint64(v)
	case float64:
		return uint64(v)
	}
	return 0
}
func mapMap(m map[string]interface{}, k string) map[string]interface{} {
	v, _ := m[k].(map[string]interface{})
	return v
}
func mapSlice(m map[string]interface{}, k string) []map[string]interface{} {
	v, _ := m[k].([]map[string]interface{})
	return v
}
func mapStrMap(m map[string]interface{}, k string) map[string]string {
	v, _ := m[k].(map[string]string)
	return v
}
func mapBoolMap(m map[string]interface{}, k string) map[string]bool {
	v, _ := m[k].(map[string]bool)
	return v
}

// escLabel escapes a string for use as a Prometheus label value
// (backslash, double-quote and newline only).
func escLabel(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func metricsPrometheus(w http.ResponseWriter, r *http.Request) {
	snap := snapshot()
	if snap == nil {
		respond(w, 503, nil, "monitor unavailable")
		return
	}
	mem := mapMap(snap, "memory")
	interfaces := mapSlice(snap, "interfaces")
	addrs := mapStrMap(snap, "interface_addrs")
	services := mapBoolMap(snap, "services")
	ptp := mapMap(snap, "ptp_status")
	conn := mapMap(snap, "connections")
	dhcpLeases := snap["dhcp_leases"]

	var b strings.Builder
	fmt.Fprintf(&b, "# HELP stageneth_cpu_percent CPU usage percent\n# TYPE stageneth_cpu_percent gauge\nstageneth_cpu_percent %.2f\n", mapFloat(snap, "cpu_percent"))
	for _, n := range []string{"load1", "load5", "load15"} {
		fmt.Fprintf(&b, "# HELP stageneth_%s Load average %s\n# TYPE stageneth_%s gauge\nstageneth_%s %.3f\n", n, n, n, n, mapFloat(snap, n))
	}
	fmt.Fprintf(&b, "# HELP stageneth_uptime_seconds Uptime in seconds\n# TYPE stageneth_uptime_seconds gauge\nstageneth_uptime_seconds %d\n", mapUint(snap, "uptime_seconds"))
	fmt.Fprintf(&b, "# HELP stageneth_memory_used_percent Memory used percent\n# TYPE stageneth_memory_used_percent gauge\nstageneth_memory_used_percent %.2f\n", mapFloat(mem, "used_percent"))
	fmt.Fprintf(&b, "# HELP stageneth_memory_total_bytes Total memory in bytes\n# TYPE stageneth_memory_total_bytes gauge\nstageneth_memory_total_bytes %d\n", mapUint(mem, "total_kb")*1024)
	fmt.Fprintf(&b, "# HELP stageneth_connections_current Current conntrack connections\n# TYPE stageneth_connections_current gauge\nstageneth_connections_current %d\n", mapUint(conn, "current"))
	fmt.Fprintf(&b, "# HELP stageneth_connections_max Max conntrack connections\n# TYPE stageneth_connections_max gauge\nstageneth_connections_max %d\n", mapUint(conn, "max"))
	leaseCount := 0
	if l, ok := dhcpLeases.([]map[string]string); ok {
		leaseCount = len(l)
	}
	fmt.Fprintf(&b, "# HELP stageneth_dhcp_leases_count Number of DHCP leases\n# TYPE stageneth_dhcp_leases_count gauge\nstageneth_dhcp_leases_count %d\n", leaseCount)
	fmt.Fprintf(&b, "# HELP stageneth_ptp_offset_ns PTP offset\n# TYPE stageneth_ptp_offset_ns gauge\nstageneth_ptp_offset_ns %.3f\n", mapFloat(ptp, "offset_ns"))
	fmt.Fprintf(&b, "# HELP stageneth_ptp_delay_ns PTP delay\n# TYPE stageneth_ptp_delay_ns gauge\nstageneth_ptp_delay_ns %.3f\n", mapFloat(ptp, "delay_ns"))

	emitGauge := func(name, help, field string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
		for _, iface := range interfaces {
			iname, _ := iface["name"].(string)
			fmt.Fprintf(&b, "%s{name=\"%s\"} %s\n", name, escLabel(iname), strconv.FormatUint(mapUint(iface, field), 10))
		}
	}
	emitGauge("stageneth_net_rx_bytes", "Received bytes per interface", "rx_bytes")
	emitGauge("stageneth_net_tx_bytes", "Transmitted bytes per interface", "tx_bytes")
	emitGauge("stageneth_net_rx_packets", "Received packets per interface", "rx_packets")
	emitGauge("stageneth_net_tx_packets", "Transmitted packets per interface", "tx_packets")

	fmt.Fprintln(&b, "# HELP stageneth_interface_info Interface address info")
	fmt.Fprintln(&b, "# TYPE stageneth_interface_info gauge")
	for _, iface := range interfaces {
		name, _ := iface["name"].(string)
		if ip := addrs[name]; ip != "" {
			fmt.Fprintf(&b, "stageneth_interface_info{name=\"%s\",ip=\"%s\"} 1\n", escLabel(name), escLabel(ip))
		}
	}
	fmt.Fprintln(&b, "# HELP stageneth_service_up Service is running")
	fmt.Fprintln(&b, "# TYPE stageneth_service_up gauge")
	svcNames := make([]string, 0, len(services))
	for svc := range services {
		svcNames = append(svcNames, svc)
	}
	sort.Strings(svcNames)
	for _, svc := range svcNames {
		upVal := 0
		if services[svc] {
			upVal = 1
		}
		fmt.Fprintf(&b, "stageneth_service_up{name=\"%s\"} %d\n", escLabel(svc), upVal)
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(b.String()))
}
