package api

import (
	json "encoding/json"
	fmt "fmt"
	http "net/http"
	os "os"
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
	if limit > 500 {
		limit = 500
	}
	lines := []string{}
	out, err := outBounded(10*time.Second, "logread", "-l", "500")
	if err == nil || len(out) > 0 {
		for _, line := range strings.Split(string(out), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				lines = append(lines, line)
			}
		}
	}
	for _, f := range []string{"/var/log/messages", "/var/log/secure"} {
		lines = appendFileLines(lines, f)
	}

	seen := map[string]bool{}
	deduped := lines[:0]
	for _, line := range lines {
		if !seen[line] {
			seen[line] = true
			deduped = append(deduped, line)
		}
	}
	lines = deduped

	now := time.Now()
	alerts := []alertEntry{}
	for _, line := range lines {
		ts, ok := parseLogTime(line)
		if !ok {
			ts = now
		}
		for _, p := range alertPatterns {
			matched := false
			for _, kw := range p.keywords {
				if strings.Contains(line, kw) {
					alerts = append(alerts, alertEntry{
						Time:     ts.Format(time.RFC3339),
						Source:   "log",
						Message:  line,
						Category: p.category,
						Severity: p.severity,
					})
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
	}
	// audit-log entries: UI auth events and config mutations
	for _, a := range auditAlerts(200) {
		alerts = append(alerts, a)
	}
	sort.Slice(alerts, func(i, j int) bool { return alerts[i].Time > alerts[j].Time })
	if len(alerts) > limit {
		alerts = alerts[:limit]
	}
	respond(w, 200, alerts, "alerts")
}

// auditAlerts converts recent audit-log lines into alert entries.
func auditAlerts(max int) []alertEntry {
	b, err := os.ReadFile(auditPath)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	var out []alertEntry
	for _, line := range lines {
		var e auditEntry
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		severity := "info"
		if strings.Contains(e.Event, "login_failed") || strings.Contains(e.Event, "failed") {
			severity = "warning"
		}
		out = append(out, alertEntry{
			Time:     time.Unix(e.TS, 0).Format(time.RFC3339),
			Source:   "audit",
			Message:  fmt.Sprintf("%s: %s %s", e.Event, e.User, e.Detail),
			Category: e.Event,
			Severity: severity,
		})
	}
	return out
}
