package api

import (
	json "encoding/json"
	fmt "fmt"
	http "net/http"
	os "os"
	sync "sync"
	time "time"
)

// Audit log: one JSON object per line, appended to a bounded file.
// Covers authentication and config mutations — feeds the alerts view.

// auditPath is overridable for tests/recovery mounts.
var auditPath = func() string {
	if p := os.Getenv("STAGENETH_AUDIT_LOG"); p != "" {
		return p
	}
	return "/var/log/stageneth-audit.log"
}()

const auditMaxBytes = 256 * 1024

var auditMu sync.Mutex

type auditEntry struct {
	TS     int64  `json:"ts"`
	Event  string `json:"event"`
	User   string `json:"user,omitempty"`
	IP     string `json:"ip,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// audit appends one entry. Best-effort: never fails the request.
func audit(event, detail string, r *http.Request) {
	e := auditEntry{TS: time.Now().Unix(), Event: event, Detail: detail, IP: clientIP(r)}
	if se, ok := sessionOf(r); ok {
		e.User = se.User
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	auditMu.Lock()
	defer auditMu.Unlock()
	if st, err := os.Stat(auditPath); err == nil && st.Size() > auditMaxBytes {
		// rotate: keep nothing — audit is a rolling record, not an archive
		os.Rename(auditPath, auditPath+".1")
	}
	f, err := os.OpenFile(auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return
	}
	fmt.Fprintln(f, string(b))
	f.Close()
}
