package main

//
// Copyright (C) 2026 StageNeth Contributors
// SPDX-License-Identifier: GPL-2.0-only
//

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

type apiResponse struct {
	Code    int         `json:"code"`
	Data    interface{} `json:"data"`
	Message string      `json:"message"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type crudRequest struct {
	Config  string                 `json:"config"`
	Section string                 `json:"section"`
	Type    string                 `json:"type"`
	Values  map[string]interface{} `json:"values"`
}

var secret string

func respond(w http.ResponseWriter, code int, data interface{}, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(apiResponse{Code: code, Data: data, Message: message})
}

func sign(value string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	return base64.URLEncoding.EncodeToString(mac.Sum(nil))
}

func token(username string) string {
	payload := fmt.Sprintf("%s:%d", username, time.Now().Unix())
	return payload + ":" + sign(payload)
}

func tokenValid(t string) string {
	parts := strings.Split(t, ":")
	if len(parts) != 3 {
		return ""
	}
	if sign(parts[0]+":"+parts[1]) != parts[2] {
		return ""
	}
	username := strings.Split(parts[0], ":")[0]
	return username
}

func login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Username != "root" {
		respond(w, 401, nil, "invalid credentials")
		return
	}
	// Simple password check against /etc/shadow via openssl passwd
	out, err := exec.Command("openssl", "passwd", "-1", "-salt", "stageneth", req.Password).Output()
	if err != nil {
		respond(w, 500, nil, "auth error")
		return
	}
	hash := strings.TrimSpace(string(out))
	data, err := os.ReadFile("/etc/shadow")
	if err != nil {
		respond(w, 500, nil, "shadow read error")
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) >= 2 && fields[0] == req.Username && fields[1] == hash {
			respond(w, 200, map[string]string{"token": token(req.Username)}, "login success")
			return
		}
	}
	respond(w, 401, nil, "invalid credentials")
}

func auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			respond(w, 403, nil, "missing token")
			return
		}
		if tokenValid(auth[7:]) == "" {
			respond(w, 403, nil, "invalid token")
			return
		}
		next(w, r)
	}
}

func ubusCall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string                 `json:"path"`
		Method  string                 `json:"method"`
		Payload map[string]interface{} `json:"payload"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		respond(w, 400, nil, "invalid request")
		return
	}
	payload, _ := json.Marshal(req.Payload)
	out, err := exec.Command("ubus", "call", req.Path, req.Method, string(payload)).Output()
	if err != nil {
		respond(w, 500, nil, "ubus call failed")
		return
	}
	var data interface{}
	json.Unmarshal(out, &data)
	respond(w, 200, data, "ubus call success")
}

func uciSet(w http.ResponseWriter, r *http.Request) {
	var req crudRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		respond(w, 400, nil, "invalid request")
		return
	}
	commands := []string{}
	if req.Type == "" {
		// delete the whole section
		commands = append(commands, fmt.Sprintf("delete %s.%s", req.Config, req.Section))
	} else {
		commands = append(commands, fmt.Sprintf("set %s.%s=%s", req.Config, req.Section, req.Type))
		for k, v := range req.Values {
			commands = append(commands, fmt.Sprintf("set %s.%s.%s='%v'", req.Config, req.Section, k, v))
		}
	}
	in := strings.Join(commands, "\n") + "\n"
	cmd := exec.Command("uci", "-q", "batch")
	cmd.Stdin = strings.NewReader(in)
	if err := cmd.Run(); err != nil {
		respond(w, 500, nil, "uci batch failed")
		return
	}
	respond(w, 200, nil, "uci set success")
}

func parseUciShow(output string) map[string]interface{} {
	sections := map[string]interface{}{}
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "=") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		key, value := kv[0], strings.Trim(strings.TrimSpace(kv[1]), "'\"")
		parts := strings.Split(key, ".")
		if len(parts) != 2 && len(parts) != 3 {
			continue
		}
		section := parts[1]
		if _, ok := sections[section]; !ok {
			sections[section] = map[string]interface{}{}
		}
		if len(parts) == 2 {
			sections[section].(map[string]interface{})[".type"] = value
		} else {
			sections[section].(map[string]interface{})[parts[2]] = value
		}
	}
	return sections
}

func uciGet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config string `json:"config"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Config == "" {
		respond(w, 400, nil, "invalid request")
		return
	}
	out, err := exec.Command("uci", "-q", "show", req.Config).Output()
	if err != nil {
		respond(w, 500, nil, "uci show failed")
		return
	}
	respond(w, 200, map[string]interface{}{"values": parseUciShow(string(out))}, "uci get success")
}

func uciCommit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config string `json:"config"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	cmd := exec.Command("uci", "-q", "commit", req.Config)
	if err := cmd.Run(); err != nil {
		respond(w, 500, nil, "uci commit failed")
		return
	}
	respond(w, 200, nil, "uci commit success")
}

func applyNetwork(w http.ResponseWriter, r *http.Request) {
	out, err := exec.Command("/usr/libexec/stageneth/stageneth-network.py", "apply").CombinedOutput()
	if err != nil {
		respond(w, 500, map[string]string{"log": string(out)}, "apply failed")
		return
	}
	respond(w, 200, map[string]string{"log": string(out)}, "apply success")
}

func networkReload(w http.ResponseWriter, r *http.Request) {
	for _, svc := range []string{"network", "firewall", "dnsmasq"} {
		cmd := exec.Command("/etc/init.d/"+svc, "reload")
		if err := cmd.Run(); err != nil {
			respond(w, 500, nil, svc+" reload failed")
			return
		}
	}
	respond(w, 200, nil, "network reloaded")
}

func networkInterfaces(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		respond(w, 500, nil, "failed to read network interfaces")
		return
	}
	interfaces := []map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		mac := readFileTrim("/sys/class/net/" + name + "/address")
		interfaces = append(interfaces, map[string]string{"name": name, "mac": mac})
	}
	respond(w, 200, map[string]interface{}{"interfaces": interfaces}, "interfaces listed")
}

func readFileTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

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

func parseInterfaceAddrs() map[string]string {
	addrs := map[string]string{}
	b, err := exec.Command("ip", "-o", "-4", "addr", "show").Output()
	if err != nil {
		return addrs
	}
	re := regexp.MustCompile(`(?m)^\d+:\s+(\S+)\s+.*\s+inet\s+([0-9.]+)/`)
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		addrs[m[1]] = m[2]
	}
	return addrs
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
		"connections":     map[string]uint64{"current": cc, "max": cm},
	}
	respond(w, 200, data, "monitoring summary")
}

type presetService struct {
	Name, VlanID, Priority, DSCP, PTP, Multicast, Untagged, Description string
}

type presetForwarding struct {
	Name, Src, Dest string
}

type preset struct {
	Name, Label, Description string
	Services                 []presetService
	Forwardings              []presetForwarding
}

var stagenethPresets = []preset{
	{
		Name:        "base",
		Label:       "Base management + PTP",
		Description: "VLANs d'administration (mgmt) et de timing PTP",
		Services: []presetService{
			{Name: "mgmt", VlanID: "10", Priority: "0", DSCP: "0", PTP: "0", Multicast: "0", Description: "Administration routeur"},
			{Name: "ptp", VlanID: "50", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", Description: "Precision Time Protocol"},
		},
		Forwardings: []presetForwarding{},
	},
	{
		Name:        "audio",
		Label:       "Audio (Dante / AES67 / AVB)",
		Description: "VLANs pour les flux audio professionnels",
		Services: []presetService{
			{Name: "mgmt", VlanID: "10", Priority: "0", DSCP: "0", PTP: "0", Multicast: "0", Description: "Administration routeur"},
			{Name: "ptp", VlanID: "50", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", Description: "Precision Time Protocol"},
			{Name: "dante", VlanID: "20", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", Description: "Dante audio"},
			{Name: "aes67", VlanID: "21", Priority: "6", DSCP: "34", PTP: "0", Multicast: "1", Description: "AES67 / RAVENNA"},
			{Name: "avb", VlanID: "22", Priority: "0", DSCP: "0", PTP: "1", Multicast: "0", Description: "AVB / Milan"},
		},
		Forwardings: []presetForwarding{
			{Name: "dante_to_mgmt", Src: "dante", Dest: "mgmt"},
			{Name: "aes67_to_mgmt", Src: "aes67", Dest: "mgmt"},
			{Name: "avb_to_mgmt", Src: "avb", Dest: "mgmt"},
		},
	},
	{
		Name:        "video",
		Label:       "Vidéo (NDI / ST 2110)",
		Description: "VLANs pour les flux vidéo sur IP",
		Services: []presetService{
			{Name: "mgmt", VlanID: "10", Priority: "0", DSCP: "0", PTP: "0", Multicast: "0", Description: "Administration routeur"},
			{Name: "ptp", VlanID: "50", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", Description: "Precision Time Protocol"},
			{Name: "ndihx", VlanID: "30", Priority: "0", DSCP: "0", PTP: "0", Multicast: "1", Description: "NDI / NDI|HX"},
			{Name: "st2110", VlanID: "31", Priority: "6", DSCP: "34", PTP: "0", Multicast: "1", Description: "SMPTE ST 2110"},
		},
		Forwardings: []presetForwarding{
			{Name: "ndihx_to_mgmt", Src: "ndihx", Dest: "mgmt"},
			{Name: "st2110_to_mgmt", Src: "st2110", Dest: "mgmt"},
		},
	},
	{
		Name:        "light",
		Label:       "Lumière (Art-Net / sACN)",
		Description: "VLANs pour le contrôle lumière",
		Services: []presetService{
			{Name: "mgmt", VlanID: "10", Priority: "0", DSCP: "0", PTP: "0", Multicast: "0", Description: "Administration routeur"},
			{Name: "ptp", VlanID: "50", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", Description: "Precision Time Protocol"},
			{Name: "artnet", VlanID: "40", Priority: "5", DSCP: "0", PTP: "0", Multicast: "0", Description: "Art-Net"},
			{Name: "sacn", VlanID: "41", Priority: "5", DSCP: "0", PTP: "0", Multicast: "1", Description: "sACN E1.31"},
			{Name: "proprietary", VlanID: "42", Priority: "5", DSCP: "0", PTP: "0", Multicast: "1", Description: "MA-Net / autre"},
		},
		Forwardings: []presetForwarding{
			{Name: "artnet_to_mgmt", Src: "artnet", Dest: "mgmt"},
			{Name: "sacn_to_mgmt", Src: "sacn", Dest: "mgmt"},
			{Name: "proprietary_to_mgmt", Src: "proprietary", Dest: "mgmt"},
		},
	},
	{
		Name:        "full-show",
		Label:       "Spectacle complet",
		Description: "Tous les VLANs audio, vidéo, lumière, management et PTP",
		Services: []presetService{
			{Name: "mgmt", VlanID: "10", Priority: "0", DSCP: "0", PTP: "0", Multicast: "0", Description: "Administration routeur"},
			{Name: "ptp", VlanID: "50", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", Description: "Precision Time Protocol"},
			{Name: "guest", VlanID: "99", Priority: "0", DSCP: "0", PTP: "0", Multicast: "0", Description: "Internet / backoffice"},
			{Name: "dante", VlanID: "20", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", Description: "Dante audio"},
			{Name: "aes67", VlanID: "21", Priority: "6", DSCP: "34", PTP: "0", Multicast: "1", Description: "AES67 / RAVENNA"},
			{Name: "avb", VlanID: "22", Priority: "0", DSCP: "0", PTP: "1", Multicast: "0", Description: "AVB / Milan"},
			{Name: "ndihx", VlanID: "30", Priority: "0", DSCP: "0", PTP: "0", Multicast: "1", Description: "NDI / NDI|HX"},
			{Name: "st2110", VlanID: "31", Priority: "6", DSCP: "34", PTP: "0", Multicast: "1", Description: "SMPTE ST 2110"},
			{Name: "artnet", VlanID: "40", Priority: "5", DSCP: "0", PTP: "0", Multicast: "0", Description: "Art-Net"},
			{Name: "sacn", VlanID: "41", Priority: "5", DSCP: "0", PTP: "0", Multicast: "1", Description: "sACN E1.31"},
			{Name: "proprietary", VlanID: "42", Priority: "5", DSCP: "0", PTP: "0", Multicast: "1", Description: "MA-Net / autre"},
		},
		Forwardings: []presetForwarding{
			{Name: "mgmt_to_dante", Src: "mgmt", Dest: "dante"},
			{Name: "mgmt_to_aes67", Src: "mgmt", Dest: "aes67"},
			{Name: "mgmt_to_avb", Src: "mgmt", Dest: "avb"},
			{Name: "mgmt_to_ndihx", Src: "mgmt", Dest: "ndihx"},
			{Name: "mgmt_to_st2110", Src: "mgmt", Dest: "st2110"},
			{Name: "mgmt_to_artnet", Src: "mgmt", Dest: "artnet"},
			{Name: "mgmt_to_sacn", Src: "mgmt", Dest: "sacn"},
			{Name: "mgmt_to_proprietary", Src: "mgmt", Dest: "proprietary"},
			{Name: "dante_to_mgmt", Src: "dante", Dest: "mgmt"},
			{Name: "aes67_to_mgmt", Src: "aes67", Dest: "mgmt"},
			{Name: "avb_to_mgmt", Src: "avb", Dest: "mgmt"},
			{Name: "ndihx_to_mgmt", Src: "ndihx", Dest: "mgmt"},
			{Name: "st2110_to_mgmt", Src: "st2110", Dest: "mgmt"},
			{Name: "artnet_to_mgmt", Src: "artnet", Dest: "mgmt"},
			{Name: "sacn_to_mgmt", Src: "sacn", Dest: "mgmt"},
			{Name: "proprietary_to_mgmt", Src: "proprietary", Dest: "mgmt"},
		},
	},
}

func presetsList(w http.ResponseWriter, r *http.Request) {
	out := []map[string]interface{}{}
	for _, p := range stagenethPresets {
		svcs := []string{}
		for _, s := range p.Services {
			svcs = append(svcs, s.Name)
		}
		out = append(out, map[string]interface{}{
			"name":        p.Name,
			"label":       p.Label,
			"description": p.Description,
			"services":    svcs,
		})
	}
	respond(w, 200, out, "presets list")
}

func presetApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string   `json:"name"`
		Services []string `json:"services"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Name == "" {
		respond(w, 400, nil, "invalid preset name")
		return
	}
	selectedSet := map[string]bool{}
	for _, s := range req.Services {
		selectedSet[s] = true
	}
	var selected *preset
	for i := range stagenethPresets {
		if stagenethPresets[i].Name == req.Name {
			selected = &stagenethPresets[i]
			break
		}
	}
	if selected == nil {
		respond(w, 404, nil, "preset not found")
		return
	}
	// Reset stageneth config so only selected preset services remain
	exec.Command("rm", "-f", "/etc/config/stageneth").Run()
	exec.Command("touch", "/etc/config/stageneth").Run()

	commands := []string{}
	for _, s := range selected.Services {
		if len(req.Services) > 0 && !selectedSet[s.Name] {
			continue
		}
		untagged := s.Untagged
		if untagged == "" {
			untagged = "0"
		}
		commands = append(commands, fmt.Sprintf("set stageneth.%s=service", s.Name))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.vlan_id='%s'", s.Name, s.VlanID))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.priority='%s'", s.Name, s.Priority))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.dscp='%s'", s.Name, s.DSCP))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.ptp='%s'", s.Name, s.PTP))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.multicast='%s'", s.Name, s.Multicast))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.untagged='%s'", s.Name, untagged))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.description='%s'", s.Name, s.Description))
	}
	for _, f := range selected.Forwardings {
		if len(req.Services) > 0 && (!selectedSet[f.Src] || !selectedSet[f.Dest]) {
			continue
		}
		commands = append(commands, fmt.Sprintf("set stageneth.%s=forwarding", f.Name))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.src='%s'", f.Name, f.Src))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.dest='%s'", f.Name, f.Dest))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.enabled='1'", f.Name))
	}
	in := strings.Join(commands, "\n") + "\n"
	cmd := exec.Command("uci", "-q", "batch")
	cmd.Stdin = strings.NewReader(in)
	if err := cmd.Run(); err != nil {
		respond(w, 500, nil, "uci batch failed")
		return
	}
	exec.Command("uci", "-q", "commit", "stageneth").Run()
	out, err := exec.Command("/usr/libexec/stageneth/stageneth-network.py", "apply").CombinedOutput()
	if err != nil {
		respond(w, 500, map[string]string{"log": string(out)}, "preset apply failed")
		return
	}
	respond(w, 200, map[string]string{"log": string(out)}, "preset applied")
}

func ntpGet(w http.ResponseWriter, r *http.Request) {
	enOut, _ := exec.Command("uci", "-q", "get", "system.ntp.enabled").Output()
	enabled := string(enOut)
	esOut, _ := exec.Command("uci", "-q", "get", "system.ntp.enable_server").Output()
	enableServer := string(esOut)
	serversOut, _ := exec.Command("uci", "-q", "get", "system.ntp.server").Output()
	servers := []string{}
	for _, s := range strings.Fields(string(serversOut)) {
		if s = strings.TrimSpace(s); s != "" {
			servers = append(servers, s)
		}
	}
	tzOut, _ := exec.Command("uci", "-q", "get", "system.@system[0].timezone").Output()
	timezone := strings.TrimSpace(string(tzOut))
	if timezone == "" {
		timezone = "UTC0"
	}
	running := false
	if out, _ := exec.Command("sh", "-c", "ps 2>/dev/null | grep -q '[n]tpd' && echo yes").Output(); strings.TrimSpace(string(out)) == "yes" {
		running = true
	}
	respond(w, 200, map[string]interface{}{
		"enabled":       strings.TrimSpace(enabled) == "1",
		"enable_server": strings.TrimSpace(enableServer) == "1",
		"servers":       servers,
		"running":       running,
		"timezone":      timezone,
	}, "ntp status")
}

func ntpSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled      bool     `json:"enabled"`
		EnableServer bool     `json:"enable_server"`
		Servers      []string `json:"servers"`
		Timezone     string   `json:"timezone"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		respond(w, 400, nil, "invalid ntp request")
		return
	}
	enVal := "0"
	if req.Enabled {
		enVal = "1"
	}
	servVal := "0"
	if req.EnableServer {
		servVal = "1"
	}
	if req.Timezone == "" {
		req.Timezone = "UTC0"
	}
	commands := []string{
		"set system.ntp=timeserver",
		fmt.Sprintf("set system.ntp.enabled='%s'", enVal),
		fmt.Sprintf("set system.ntp.enable_server='%s'", servVal),
		"delete system.ntp.server",
		fmt.Sprintf("set system.@system[0].timezone='%s'", req.Timezone),
	}
	for _, s := range req.Servers {
		if s = strings.TrimSpace(s); s != "" {
			commands = append(commands, fmt.Sprintf("add_list system.ntp.server='%s'", s))
		}
	}
	in := strings.Join(commands, "\n") + "\n"
	cmd := exec.Command("uci", "-q", "batch")
	cmd.Stdin = strings.NewReader(in)
	if err := cmd.Run(); err != nil {
		respond(w, 500, nil, "uci batch failed")
		return
	}
	exec.Command("uci", "-q", "commit", "system").Run()
	if _, err := os.Stat("/etc/init.d/sysntpd"); err == nil {
		exec.Command("/etc/init.d/sysntpd", "restart").Run()
	} else {
		exec.Command("killall", "ntpd").Run()
		exec.Command("ntpd", "-n", "-N", "-S", "/usr/sbin/ntpd-hotplug").Start()
	}
	ntpGet(w, r)
}

func timeGet(w http.ResponseWriter, r *http.Request) {
	tz := r.URL.Query().Get("tz")
	if tz == "" {
		if out, err := exec.Command("uci", "-q", "get", "system.@system[0].timezone").Output(); err == nil {
			tz = strings.TrimSpace(string(out))
		}
	}
	if tz == "" {
		tz = "UTC0"
	}
	env := append(os.Environ(), "TZ="+tz)

	dateCmd := exec.Command("date", "+%Y-%m-%d")
	dateCmd.Env = env
	dateOut, _ := dateCmd.Output()

	timeCmd := exec.Command("date", "+%H:%M:%S")
	timeCmd.Env = env
	timeOut, _ := timeCmd.Output()

	tzCmd := exec.Command("date", "+%Z")
	tzCmd.Env = env
	tzOut, _ := tzCmd.Output()

	source, stratum, offset := "-", "", ""
	if out, err := exec.Command("ntpq", "-pn", "127.0.0.1").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "*") {
				fields := strings.Fields(line)
				if len(fields) >= 9 {
					source = strings.TrimPrefix(fields[0], "*")
					stratum = fields[2]
					offset = fields[8]
				}
				break
			}
		}
	} else if out, err := exec.Command("uci", "-q", "get", "system.ntp.server").Output(); err == nil {
		servers := strings.Fields(string(out))
		if len(servers) > 0 {
			source = servers[0]
		}
	}

	respond(w, 200, map[string]string{
		"date":     strings.TrimSpace(string(dateOut)),
		"time":     strings.TrimSpace(string(timeOut)),
		"timezone": strings.TrimSpace(string(tzOut)),
		"source":   source,
		"stratum":  stratum,
		"offset":   offset,
	}, "time")
}

func logsGet(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	out, err := exec.Command("logread").CombinedOutput()
	if err != nil {
		respond(w, 500, nil, "logread failed")
		return
	}
	lines := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	respond(w, 200, lines, "logs")
}

func mdnsParseZone(out []byte, serviceType, domain string) ([]string, map[string]map[string]interface{}) {
	names := []string{}
	seen := map[string]bool{}
	entries := map[string]map[string]interface{}{}
	ips := map[string]string{}
	srvRe := regexp.MustCompile(`^([^\s]+)\s+SRV\s+\S+\s+\S+\s+(\d+)\s+(\S+)\.?`)
	txtRe := regexp.MustCompile(`^([^\s]+)\s+TXT\s+(.+)$`)
	ipRe := regexp.MustCompile(`^([^\s]+)\s+A\s+(\d+\.\d+\.\d+\.\d+)`)
	ptrRe := regexp.MustCompile(`^([^\s]+)\s+PTR\s+(\S+)`)

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if m := srvRe.FindStringSubmatch(line); m != nil {
			name := strings.Trim(strings.ReplaceAll(strings.TrimSuffix(strings.TrimSuffix(m[1], "."), "."+serviceType), "\\032", " "), "\"")
			if entries[name] == nil {
				entries[name] = map[string]interface{}{"name": name, "type": serviceType, "domain": domain}
			}
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
			entries[name]["hostname"] = strings.TrimSuffix(m[3], ".")
			if port, err := strconv.Atoi(m[2]); err == nil {
				entries[name]["port"] = port
			}
			continue
		}
		if m := txtRe.FindStringSubmatch(line); m != nil {
			name := strings.Trim(strings.ReplaceAll(strings.TrimSuffix(strings.TrimSuffix(m[1], "."), "."+serviceType), "\\032", " "), "\"")
			if entries[name] == nil {
				entries[name] = map[string]interface{}{"name": name, "type": serviceType, "domain": domain}
			}
			txt := []string{}
			for _, q := range regexp.MustCompile(`"([^"]*)"`).FindAllStringSubmatch(m[2], -1) {
				txt = append(txt, q[1])
			}
			entries[name]["txt"] = txt
			continue
		}
		if m := ipRe.FindStringSubmatch(line); m != nil {
			ips[strings.TrimSuffix(m[1], ".")] = m[2]
			continue
		}
		if m := ptrRe.FindStringSubmatch(line); m != nil {
			name := strings.Trim(strings.ReplaceAll(strings.TrimSuffix(strings.TrimSuffix(m[2], "."), "."+serviceType), "\\032", " "), "\"")
			if entries[name] == nil {
				entries[name] = map[string]interface{}{"name": name, "type": serviceType, "domain": domain}
			}
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}

	for _, entry := range entries {
		if hostname, ok := entry["hostname"].(string); ok && hostname != "" {
			if ip, ok := ips[hostname]; ok {
				entry["ip"] = ip
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				ipOut, _ := exec.CommandContext(ctx, "dns-sd", "-G", "v4", hostname+".").CombinedOutput()
				cancel()
				if m := regexp.MustCompile(`\b(\d+\.\d+\.\d+\.\d+)\b`).FindSubmatch(ipOut); m != nil {
					entry["ip"] = string(m[1])
				}
			}
		}
	}

	return names, entries
}

func mdnsResolve(serviceType, domain string, duration int) (map[string]map[string]interface{}, []string) {
	cmd := fmt.Sprintf("dns-sd -Z %s %s & PID=$!; sleep %d; kill $PID 2>/dev/null; wait $PID 2>/dev/null", serviceType, domain, duration)
	out, _ := exec.Command("sh", "-c", cmd).CombinedOutput()
	names, entries := mdnsParseZone(out, serviceType, domain)
	return entries, names
}

func mdnsDiscover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Service  string `json:"service"`
		Domain   string `json:"domain"`
		Duration int    `json:"duration"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		req.Service = ""
		req.Domain = ""
		req.Duration = 0
	}
	if req.Service == "" {
		req.Service = "_services._dns-sd._udp"
	}
	if req.Domain == "" {
		req.Domain = "local"
	}
	if req.Duration <= 0 || req.Duration > 10 {
		req.Duration = 3
	}
	if matched, _ := regexp.MatchString("^[A-Za-z0-9_.-]+$", req.Service); !matched {
		respond(w, 400, nil, "invalid service name")
		return
	}
	if matched, _ := regexp.MatchString("^[A-Za-z0-9_.-]+$", req.Domain); !matched {
		respond(w, 400, nil, "invalid domain")
		return
	}

	cmd := fmt.Sprintf("dns-sd -B %s %s & PID=$!; sleep %d; kill $PID 2>/dev/null; wait $PID 2>/dev/null", req.Service, req.Domain, req.Duration)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil && len(out) == 0 {
		respond(w, 500, nil, "mdns command failed: "+err.Error())
		return
	}

	if req.Service == "_services._dns-sd._udp" {
		seen := map[string]bool{}
		services := []string{}
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.Contains(line, " Add ") {
				continue
			}
			f := strings.Fields(line)
			if len(f) < 2 {
				continue
			}
			domainTok := f[len(f)-2]
			nameTok := f[len(f)-1]
			transport := strings.TrimSuffix(domainTok, ".local.")
			transport = strings.TrimSuffix(transport, ".local")
			if transport == "" || !strings.HasPrefix(transport, "_") {
				continue
			}
			serviceType := nameTok + "." + transport
			if serviceType == "_dns-sd._udp" || serviceType == "_services._dns-sd._udp" {
				continue
			}
			if !seen[serviceType] {
				seen[serviceType] = true
				services = append(services, serviceType)
			}
		}

		results := []map[string]interface{}{}
		resolveDuration := req.Duration
		if resolveDuration > 3 {
			resolveDuration = 2
		} else if resolveDuration > 1 {
			resolveDuration = 1
		}
		for _, serviceType := range services {
			entries, names := mdnsResolve(serviceType, req.Domain, resolveDuration)
			for _, name := range names {
				if e, ok := entries[name]; ok {
					if h, ok := e["hostname"].(string); ok && h != "" {
						results = append(results, e)
					}
				}
			}
		}
		respond(w, 200, results, "mdns discover success")
		return
	}

	names, entries := mdnsParseZone(out, req.Service, req.Domain)
	results := []map[string]interface{}{}
	for _, name := range names {
		if e, ok := entries[name]; ok {
			if h, ok := e["hostname"].(string); ok && h != "" {
				results = append(results, e)
			}
		}
	}
	respond(w, 200, results, "mdns discover success")
}

func snmpWalk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host      string `json:"host"`
		Port      int    `json:"port"`
		Community string `json:"community"`
		OID       string `json:"oid"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Host == "" || req.OID == "" {
		respond(w, 400, nil, "invalid request: host and oid required")
		return
	}
	if req.Port == 0 {
		req.Port = 161
	}
	if req.Community == "" {
		req.Community = "public"
	}

	g := &gosnmp.GoSNMP{
		Target:             req.Host,
		Port:               uint16(req.Port),
		Community:          req.Community,
		Version:            gosnmp.Version2c,
		Timeout:            5 * time.Second,
		Retries:            1,
		ExponentialTimeout: false,
		MaxRepetitions:     10,
	}
	if err := g.Connect(); err != nil {
		respond(w, 500, nil, "snmp connect failed: "+err.Error())
		return
	}
	defer g.Conn.Close()

	results := []map[string]interface{}{}
	err := g.Walk(req.OID, func(pdu gosnmp.SnmpPDU) error {
		results = append(results, map[string]interface{}{
			"oid":   pdu.Name,
			"type":  pdu.Type.String(),
			"value": fmt.Sprintf("%v", pdu.Value),
		})
		return nil
	})
	if err != nil {
		respond(w, 500, nil, "snmp walk failed: "+err.Error())
		return
	}
	respond(w, 200, results, "snmp walk success")
}

func shadowRootHash() (string, error) {
	data, err := os.ReadFile("/etc/shadow")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) >= 2 && fields[0] == "root" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("root not found in shadow")
}

func opensslHash(password string) (string, error) {
	// Must match the salt used by the login endpoint
	out, err := exec.Command("openssl", "passwd", "-1", "-salt", "stageneth", password).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func firstbootDone() bool {
	out, _ := exec.Command("uci", "-q", "get", "stageneth.globals.firstboot_done").Output()
	return strings.TrimSpace(string(out)) == "1"
}

func listServices() []string {
	out, err := exec.Command("uci", "-q", "show", "stageneth").Output()
	if err != nil {
		return []string{}
	}
	sections := parseUciShow(string(out))
	names := []string{}
	for s, data := range sections {
		m, ok := data.(map[string]interface{})
		if !ok {
			continue
		}
		if t, ok := m[".type"].(string); ok && t == "service" {
			names = append(names, s)
		}
	}
	sort.Strings(names)
	return names
}

func firstbootStatus(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, map[string]interface{}{"firstboot_done": firstbootDone(), "services": listServices()}, "ok")
}

func wizard(w http.ResponseWriter, r *http.Request) {
	if firstbootDone() {
		respond(w, 403, nil, "wizard already completed")
		return
	}
	var req struct {
		Password string   `json:"password"`
		Services []string `json:"services"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Password == "" {
		respond(w, 400, nil, "invalid wizard request")
		return
	}
	hash, err := opensslHash(req.Password)
	if err != nil {
		respond(w, 500, nil, "hash error")
		return
	}
	if err := exec.Command("sed", "-i", fmt.Sprintf("s|^root:[^:]*:|root:%s:|", hash), "/etc/shadow").Run(); err != nil {
		respond(w, 500, nil, "shadow update error")
		return
	}

	selected := map[string]bool{}
	for _, s := range req.Services {
		selected[s] = true
	}
	known := listServices()
	commands := []string{}
	for _, s := range known {
		if !selected[s] {
			commands = append(commands, fmt.Sprintf("delete stageneth.%s", s))
		}
	}
	commands = append(commands, "set stageneth.globals='stageneth'", "set stageneth.globals.firstboot_done='1'")
	in := strings.Join(commands, "\n") + "\n"
	cmd := exec.Command("uci", "-q", "batch")
	cmd.Stdin = strings.NewReader(in)
	if err := cmd.Run(); err != nil {
		respond(w, 500, nil, "uci batch failed")
		return
	}
	exec.Command("uci", "-q", "commit", "stageneth").Run()
	if out, err := exec.Command("/usr/libexec/stageneth/stageneth-network.py", "apply").CombinedOutput(); err != nil {
		respond(w, 500, map[string]string{"log": string(out)}, "network apply failed")
		return
	}
	respond(w, 200, map[string]string{"token": token("root")}, "wizard complete")
}

func changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.CurrentPassword == "" || req.NewPassword == "" || req.NewPassword != req.ConfirmPassword {
		respond(w, 400, nil, "invalid password change request")
		return
	}
	currentHash, err := opensslHash(req.CurrentPassword)
	if err != nil {
		respond(w, 500, nil, "hash error")
		return
	}
	rootHash, err := shadowRootHash()
	if err != nil {
		respond(w, 500, nil, "shadow read error")
		return
	}
	if currentHash != rootHash {
		respond(w, 401, nil, "current password incorrect")
		return
	}
	newHash, err := opensslHash(req.NewPassword)
	if err != nil {
		respond(w, 500, nil, "hash error")
		return
	}
	// Update root password field in /etc/shadow
	if err := exec.Command("sed", "-i", fmt.Sprintf("s|^root:[^:]*:|root:%s:|", newHash), "/etc/shadow").Run(); err != nil {
		respond(w, 500, nil, "shadow update error")
		return
	}
	respond(w, 200, nil, "password changed")
}

func main() {
	secret = os.Getenv("STAGENETH_SECRET")
	if secret == "" {
		secret = "change-me"
	}
	bind := os.Getenv("STAGENETH_BIND")
	if bind == "" {
		bind = "127.0.0.1"
	}
	port := os.Getenv("STAGENETH_PORT")
	if port == "" {
		port = "8090"
	}

	http.HandleFunc("/api/login", login)
	http.HandleFunc("/api/firstboot", firstbootStatus)
	http.HandleFunc("/api/wizard", wizard)
	http.HandleFunc("/api/change-password", auth(changePassword))
	http.HandleFunc("/api/ubus/call", auth(ubusCall))
	http.HandleFunc("/api/uci/get", auth(uciGet))
	http.HandleFunc("/api/uci/set", auth(uciSet))
	http.HandleFunc("/api/uci/commit", auth(uciCommit))
	http.HandleFunc("/api/stageneth/apply", auth(applyNetwork))
	http.HandleFunc("/api/stageneth/presets", auth(presetsList))
	http.HandleFunc("/api/stageneth/preset-apply", auth(presetApply))
	http.HandleFunc("/api/network/reload", auth(networkReload))
	http.HandleFunc("/api/network/interfaces", auth(networkInterfaces))
	http.HandleFunc("/api/monitoring/summary", auth(monitoringSummary))
	http.HandleFunc("/api/snmp/walk", auth(snmpWalk))
	http.HandleFunc("/api/mdns/discover", auth(mdnsDiscover))
	http.HandleFunc("/api/ntp", auth(ntpGet))
	http.HandleFunc("/api/ntp/set", auth(ntpSet))
	http.HandleFunc("/api/time", auth(timeGet))
	http.HandleFunc("/api/logs", auth(logsGet))

	addr := fmt.Sprintf("%s:%s", bind, port)
	fmt.Println("StageNeth API listening on", addr)
	http.ListenAndServe(addr, nil)
}
