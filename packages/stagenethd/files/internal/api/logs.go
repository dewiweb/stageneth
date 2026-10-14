package api

import (
	http "net/http"
	os "os"
	exec "os/exec"
	sort "sort"
	strconv "strconv"
	strings "strings"
	time "time"
)

// appendFileLines appends at most the last 1000 lines of a log file,
// reading at most 512 KiB from the tail so a giant log can't be pulled
// fully into memory per request.
func appendFileLines(lines []string, path string) []string {
	fi, err := os.Stat(path)
	if err != nil {
		return lines
	}
	const maxRead = 512 << 10
	f, err := os.Open(path)
	if err != nil {
		return lines
	}
	defer f.Close()
	start := int64(0)
	if fi.Size() > maxRead {
		start = fi.Size() - maxRead
	}
	buf := make([]byte, fi.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil {
		return lines
	}
	tail := strings.Split(string(buf), "\n")
	if start > 0 && len(tail) > 0 {
		tail = tail[1:] // drop partial first line
	}
	if len(tail) > 1000 {
		tail = tail[len(tail)-1000:]
	}
	for _, line := range tail {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
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
	if limit > 1000 {
		limit = 1000
	}
	out, err := exec.Command("logread", "-l", "1000").CombinedOutput()
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
	seen := map[string]bool{}
	deduped := lines[:0]
	for _, line := range lines {
		if !seen[line] {
			seen[line] = true
			deduped = append(deduped, line)
		}
	}
	lines = deduped
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
