package main

import (
	http "net/http"
	os "os"
	exec "os/exec"
	sort "sort"
	strconv "strconv"
	strings "strings"
	time "time"
)

func appendFileLines(lines []string, path string) []string {
	if b, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				lines = append(lines, line)
			}
		}
	}
	return lines
}

type logEntry struct {
	ts   time.Time
	line string
}

func parseLogTime(line string) (time.Time, bool) {
	line = strings.TrimSpace(line)
	if len(line) >= 24 {
		if t, err := time.Parse("Mon Jan _2 15:04:05 2006", line[:24]); err == nil {
			return t, true
		}
	}
	if len(line) >= 15 {
		if t, err := time.Parse("Jan _2 15:04:05", line[:15]); err == nil {
			now := time.Now()
			return time.Date(now.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local), true
		}
		if len(line) >= 16 {
			if t, err := time.Parse("Jan _2 15:04:05", line[:16]); err == nil {
				now := time.Now()
				return time.Date(now.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local), true
			}
		}
	}
	if len(line) >= 19 {
		if t, err := time.Parse("2006/01/02 15:04:05", line[:19]); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
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
	for _, f := range []string{"/var/log/messages", "/var/log/secure", "/var/log/cron", "/var/log/nginx/error.log"} {
		lines = appendFileLines(lines, f)
	}
	entries := make([]logEntry, 0, len(lines))
	now := time.Now()
	for _, line := range lines {
		ts, ok := parseLogTime(line)
		if !ok {
			ts = now
		}
		entries = append(entries, logEntry{ts, line})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ts.Before(entries[j].ts) })
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	outLines := make([]string, len(entries))
	for i, e := range entries {
		outLines[i] = e.line
	}
	respond(w, 200, outLines, "logs")
}
