package api

import (
	fmt "fmt"
	http "net/http"
	exec "os/exec"
	regexp "regexp"
	strings "strings"
)

// rawConfigs is the allowlist for expert-mode raw UCI editing.
// stageneth itself is excluded — it is only writable via typed v1 resources.
var rawConfigs = map[string]bool{
	"network": true, "dhcp": true, "firewall": true, "system": true,
}

var uciNameRe = regexp.MustCompile(`^[a-zA-Z0-9_@.\[\]-]+$`)

func validUciName(s string) bool {
	return s != "" && uciNameRe.MatchString(s)
}

func rawConfigName(r *http.Request) (string, bool) {
	c := r.PathValue("config")
	return c, rawConfigs[c]
}

// rawGet returns `uci show <config>` parsed into the same shape the UI
// consumed from the legacy endpoint: {values: {section: {opts}}}.
func rawGet(w http.ResponseWriter, r *http.Request) {
	cfg, ok := rawConfigName(r)
	if !ok {
		respond(w, 400, nil, "config not writable via raw API")
		return
	}
	out, err := exec.Command("uci", "-q", "show", cfg).Output()
	if err != nil {
		respond(w, 500, nil, "uci show failed")
		return
	}
	respond(w, 200, map[string]interface{}{"values": parseUciShow(string(out))}, "uci get success")
}

// rawCommit runs `uci commit <config>` plus the system reload side-effects
// the legacy uciCommit endpoint had.
func rawCommit(cfg string) error {
	if err := exec.Command("uci", "-q", "commit", cfg).Run(); err != nil {
		return err
	}
	if cfg == "system" {
		exec.Command("/etc/init.d/system", "reload").Run()
		if name, err := exec.Command("uci", "-q", "get", "system.@system[0].hostname").Output(); err == nil && len(name) > 0 {
			exec.Command("hostname", strings.TrimSpace(string(name))).Run()
		}
	}
	return nil
}

// rawPut creates or replaces a section: {section, type, values}.
// `type` may be "" to rename-in-place semantics? No: type is required when
// creating; for existing sections it may be "" to keep the current type.
func rawPut(w http.ResponseWriter, r *http.Request) {
	cfg, ok := rawConfigName(r)
	if !ok {
		respond(w, 400, nil, "config not writable via raw API")
		return
	}
	var req struct {
		Section string                 `json:"section"`
		Type    string                 `json:"type"`
		Values  map[string]interface{} `json:"values"`
	}
	if !decodeBody(r, &req) || req.Section == "" {
		respond(w, 400, nil, "invalid request")
		return
	}
	if !validUciName(req.Section) {
		respond(w, 400, nil, "invalid section name")
		return
	}
	commands := []string{}
	if req.Type != "" {
		if !validUciName(req.Type) {
			respond(w, 400, nil, "invalid section type")
			return
		}
		commands = append(commands, fmt.Sprintf("set %s.%s=%s", cfg, req.Section, req.Type))
	}
	for k, v := range req.Values {
		val := fmt.Sprintf("%v", v)
		if !validUciName(k) || strings.ContainsAny(val, "\r\n") {
			respond(w, 400, nil, "invalid option name or value")
			return
		}
		commands = append(commands, fmt.Sprintf("set %s.%s.%s=%v", cfg, req.Section, k, v))
	}
	if len(commands) == 0 {
		respond(w, 400, nil, "nothing to write")
		return
	}
	cmd := exec.Command("uci", "-q", "batch")
	cmd.Stdin = strings.NewReader(strings.Join(commands, "\n") + "\n")
	if err := cmd.Run(); err != nil {
		respond(w, 500, nil, "uci batch failed")
		return
	}
	if err := rawCommit(cfg); err != nil {
		respond(w, 500, nil, "uci commit failed")
		return
	}
	audit("raw_set", cfg+"."+req.Section, r)
	respond(w, 200, nil, "section saved")
}

// rawDelete removes a section: DELETE /api/v1/raw/{config}?section=<name>.
func rawDelete(w http.ResponseWriter, r *http.Request) {
	cfg, ok := rawConfigName(r)
	if !ok {
		respond(w, 400, nil, "config not writable via raw API")
		return
	}
	section := r.URL.Query().Get("section")
	if !validUciName(section) {
		respond(w, 400, nil, "invalid section name")
		return
	}
	cmd := exec.Command("uci", "-q", "batch")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("delete %s.%s\n", cfg, section))
	if err := cmd.Run(); err != nil {
		respond(w, 500, nil, "uci batch failed")
		return
	}
	if err := rawCommit(cfg); err != nil {
		respond(w, 500, nil, "uci commit failed")
		return
	}
	audit("raw_delete", cfg+"."+section, r)
	respond(w, 200, nil, "section deleted")
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
