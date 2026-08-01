package main

import (
	http "net/http"
	exec "os/exec"
	sort "sort"
	strconv "strconv"
	strings "strings"
	time "time"
)

type alertPattern struct {
	category string
	severity string
	keywords []string
}

var alertPatterns = []alertPattern{
	{category: "ssh", severity: "warning", keywords: []string{"Failed password", "Invalid user", "Connection closed by authenticating user", "authentication attempt", "error: maximum authentication", "bad password", "Password not changed", "dropbear"}},
	{category: "ui", severity: "warning", keywords: []string{"login failed", "Unauthorized", "invalid credentials"}},
	{category: "arp", severity: "warning", keywords: []string{"duplicate IP", "IPv4 duplicate", "Replaced by foreign", "ARP conflict"}},
	{category: "firewall", severity: "info", keywords: []string{"DROP", "REJECT", "refused"}},
}

type alertEntry struct {
	Time     string `json:"time"`
	Source   string `json:"source"`
	Message  string `json:"message"`
	Category string `json:"category"`
	Severity string `json:"severity"`
}

func alertsList(w http.ResponseWriter, r *http.Request) {
	l := r.URL.Query().Get("limit")
	limit := 50
	if l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	lines := []string{}
	out, err := exec.Command("logread").CombinedOutput()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				lines = append(lines, line)
			}
		}
	}
	for _, f := range []string{"/var/log/messages", "/var/log/secure"} {
		lines = appendFileLines(lines, f)
	}

	now := time.Now()
	var alerts []alertEntry
	for _, line := range lines {
		ts, ok := parseLogTime(line)
		if !ok {
			ts = now
		}
		for _, p := range alertPatterns {
			for _, kw := range p.keywords {
				if strings.Contains(line, kw) {
					alerts = append(alerts, alertEntry{
						Time:     ts.Format(time.RFC3339),
						Source:   "log",
						Message:  line,
						Category: p.category,
						Severity: p.severity,
					})
					break
				}
			}
		}
	}
	sort.Slice(alerts, func(i, j int) bool { return alerts[i].Time > alerts[j].Time })
	if len(alerts) > limit {
		alerts = alerts[:limit]
	}
	respond(w, 200, alerts, "alerts")
}
