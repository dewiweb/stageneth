package api

import (
	fmt "fmt"
	http "net/http"
	exec "os/exec"
	regexp "regexp"
	"stageneth/internal/ucifile"
	strings "strings"
	time "time"
)

// rawConfigs is the allowlist for expert-mode raw UCI editing.
// stageneth itself is excluded — it is only writable via typed v1 resources.
var rawConfigs = map[string]bool{
	"network": true, "dhcp": true, "firewall": true, "system": true,
}

// section names may be anonymous references like @system[0]; option names
// cannot — `set cfg.sec..x=v` would produce a malformed batch directive.
var uciNameRe = regexp.MustCompile(`^[a-zA-Z0-9_@.\[\]-]+$`)
var uciOptRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// uciCommand wraps exec.Command("uci", ...) pinning the config dir so the
// daemon can run against a non-/etc/config tree (tests, recovery mounts).
func uciCommand(args ...string) *exec.Cmd {
	return boundedCmd(10*time.Second, "uci", append([]string{"-c", ucifile.ConfigDir}, args...)...)
}

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
	out, err := uciCommand("-q", "show", cfg).Output()
	if err != nil {
		respond(w, 500, nil, "uci show failed")
		return
	}
	respond(w, 200, map[string]interface{}{"values": parseUciShow(string(out))}, "uci get success")
}

// rawCommit runs `uci commit <config>` plus the system reload side-effects
// the legacy uciCommit endpoint had.
func rawCommit(cfg string) error {
	if err := uciCommand("-q", "commit", cfg).Run(); err != nil {
		return err
	}
	if cfg == "system" {
		runBounded(10*time.Second, initDir()+"/system", "reload")
		if name, err := uciCommand("-q", "get", "system.@system[0].hostname").Output(); err == nil && len(name) > 0 {
			runBounded(5*time.Second, "hostname", strings.TrimSpace(string(name)))
		}
	}
	return nil
}

// rawPutCommands builds the uci batch directives for a section write.
// Scalar values become `set`; JSON arrays become `delete` + `add_list` per
// element (UCI list semantics — `set` on an existing list would silently
// collapse it to a single-value option).
func rawPutCommands(cfg, section, stype string, values map[string]interface{}) ([]string, error) {
	commands := []string{}
	if stype != "" {
		if !uciOptRe.MatchString(stype) {
			return nil, fmt.Errorf("invalid section type")
		}
		commands = append(commands, fmt.Sprintf("set %s.%s=%s", cfg, section, stype))
	}
	for k, v := range values {
		if !uciOptRe.MatchString(k) {
			return nil, fmt.Errorf("invalid option name")
		}
		if list, ok := v.([]interface{}); ok {
			commands = append(commands, fmt.Sprintf("delete %s.%s.%s", cfg, section, k))
			for _, item := range list {
				val := fmt.Sprintf("%v", item)
				if strings.ContainsAny(val, "\r\n'\"") {
					return nil, fmt.Errorf("invalid list value")
				}
				commands = append(commands, fmt.Sprintf("add_list %s.%s.%s='%s'", cfg, section, k, val))
			}
			continue
		}
		val := fmt.Sprintf("%v", v)
		// uci has no escape syntax inside quotes — reject quote chars
		// outright and always single-quote the value so whitespace or
		// shell-looking content can't reshape the batch directive
		if strings.ContainsAny(val, "\r\n'\"") {
			return nil, fmt.Errorf("invalid option value")
		}
		commands = append(commands, fmt.Sprintf("set %s.%s.%s='%s'", cfg, section, k, val))
	}
	if len(commands) == 0 {
		return nil, fmt.Errorf("nothing to write")
	}
	return commands, nil
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
	commands, err := rawPutCommands(cfg, req.Section, req.Type, req.Values)
	if err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	cmd := uciCommand("-q", "batch")
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
	cmd := uciCommand("-q", "batch")
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
			sec := sections[section].(map[string]interface{})
			k := parts[2]
			// uci lists emit one line per element — keep them all so the
			// expert view can round-trip without collapsing the list
			if prev, exists := sec[k]; exists {
				switch l := prev.(type) {
				case []interface{}:
					sec[k] = append(l, value)
				default:
					sec[k] = []interface{}{prev, value}
				}
			} else {
				sec[k] = value
			}
		}
	}
	return sections
}
