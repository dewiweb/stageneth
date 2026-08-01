package main

import (
	json "encoding/json"
	fmt "fmt"
	http "net/http"
	os "os"
	exec "os/exec"
	strings "strings"
)

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
