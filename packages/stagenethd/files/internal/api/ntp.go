package api

import (
	json "encoding/json"
	http "net/http"
	os "os"
	exec "os/exec"
	regexp "regexp"
	strings "strings"

	"stageneth/internal/ucifile"
)

// systemConfig reads /etc/config/system via the shared parser.
func systemConfig() (*ucifile.File, error) {
	return ucifile.Read("/etc/config/system")
}

func findByType(f *ucifile.File, typ string) *ucifile.Section {
	for i := range f.Sections {
		if f.Sections[i].Type == typ {
			return &f.Sections[i]
		}
	}
	return nil
}

func listValues(s *ucifile.Section, name string) []string {
	var out []string
	if s == nil {
		return out
	}
	for _, o := range s.Options {
		if o.Name == name {
			for _, v := range strings.Fields(o.Value) {
				out = append(out, v)
			}
		}
	}
	return out
}

func ntpGet(w http.ResponseWriter, r *http.Request) {
	f, err := systemConfig()
	if err != nil {
		respond(w, 500, nil, "system config unreadable")
		return
	}
	ntp := findByType(f, "timeserver")
	sys := findByType(f, "system")
	enabled, enableServer := false, false
	servers := []string{}
	if ntp != nil {
		enabled = ntp.Get("enabled") == "1"
		enableServer = ntp.Get("enable_server") == "1"
		servers = listValues(ntp, "server")
	}
	timezone := "UTC0"
	if sys != nil {
		if tz := sys.Get("timezone"); tz != "" {
			timezone = tz
		}
	}
	respond(w, 200, map[string]interface{}{
		"enabled":       enabled,
		"enable_server": enableServer,
		"servers":       servers,
		"running":       ntpdRunning(),
		"timezone":      timezone,
		"clock":         clockStatus(),
	}, "ntp status")
}

// ntpdRunning checks /proc/*/comm — no shell, no ps parsing.
func ntpdRunning() bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] < '0' || e.Name()[0] > '9' {
			continue
		}
		if b, err := os.ReadFile("/proc/" + e.Name() + "/comm"); err == nil {
			if strings.TrimSpace(string(b)) == "ntpd" {
				return true
			}
		}
	}
	return false
}

var (
	ntpHostRe = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
	ntpTzRe   = regexp.MustCompile(`^[A-Za-z0-9+/,._:-]+$`)
)

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
	if req.Timezone == "" {
		req.Timezone = "UTC0"
	}
	if len(req.Timezone) > 64 || !ntpTzRe.MatchString(req.Timezone) {
		respond(w, 400, nil, "invalid timezone")
		return
	}
	var servers []string
	for _, s := range req.Servers {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if len(s) > 253 || !ntpHostRe.MatchString(s) {
			respond(w, 400, nil, "invalid server: "+s)
			return
		}
		servers = append(servers, s)
	}
	enVal, servVal := "0", "0"
	if req.Enabled {
		enVal = "1"
	}
	if req.EnableServer {
		servVal = "1"
	}
	commands := []string{
		"set system.ntp=timeserver",
		"set system.ntp.enabled='" + enVal + "'",
		"set system.ntp.enable_server='" + servVal + "'",
		"delete system.ntp.server",
		"set system.@system[0].timezone='" + req.Timezone + "'",
	}
	for _, s := range servers {
		commands = append(commands, "add_list system.ntp.server='"+s+"'")
	}
	cmd := exec.Command("uci", "-q", "batch")
	cmd.Stdin = strings.NewReader(strings.Join(commands, "\n") + "\n")
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
	audit("ntp_set", strings.Join(servers, " "), r)
	ntpGet(w, r)
}

// timeGet serves the lightweight clock tick (polled once per second by the
// NTP tab). ntpGet also embeds it as "clock".
func timeGet(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, clockStatus(), "time")
}

// clockStatus returns the current date/time and sync source. One `date`
// fork, timezone read via the parser — it is polled every second.
func clockStatus() map[string]string {
	tz := "UTC0"
	if f, err := systemConfig(); err == nil {
		if sys := findByType(f, "system"); sys != nil {
			if t := sys.Get("timezone"); t != "" {
				tz = t
			}
		}
	}
	env := append(os.Environ(), "TZ="+tz)
	cmd := exec.Command("date", "+%Y-%m-%d %H:%M:%S %Z")
	cmd.Env = env
	out, _ := cmd.Output()
	fields := strings.Fields(string(out))
	date, tm, tzName := "", "", ""
	if len(fields) == 3 {
		date, tm, tzName = fields[0], fields[1], fields[2]
	}

	source, stratum, offset := "-", "", ""
	if out, err := exec.Command("ntpq", "-pn", "127.0.0.1").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "*") {
				if f := strings.Fields(line); len(f) >= 9 {
					source = strings.TrimPrefix(f[0], "*")
					stratum = f[2]
					offset = f[8]
				}
				break
			}
		}
	}
	if source == "-" {
		if f, err := systemConfig(); err == nil {
			if ntp := findByType(f, "timeserver"); ntp != nil {
				if sv := listValues(ntp, "server"); len(sv) > 0 {
					source = sv[0]
				}
			}
		}
	}
	return map[string]string{
		"date":     date,
		"time":     tm,
		"timezone": tzName,
		"source":   source,
		"stratum":  stratum,
		"offset":   offset,
	}
}
