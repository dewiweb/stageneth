package main

import (
	json "encoding/json"
	fmt "fmt"
	http "net/http"
	exec "os/exec"
	strings "strings"
)

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

		commands = append(commands, fmt.Sprintf("delete %s.%s", req.Config, req.Section))
	} else {
		commands = append(commands, fmt.Sprintf("set %s.%s=%s", req.Config, req.Section, req.Type))
		for k, v := range req.Values {
			commands = append(commands, fmt.Sprintf("set %s.%s.%s=%v", req.Config, req.Section, k, v))
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
	if req.Config == "system" {
		exec.Command("/etc/init.d/system", "reload").Run()
		if name, err := exec.Command("uci", "-q", "get", "system.@system[0].hostname").Output(); err == nil && len(name) > 0 {
			exec.Command("hostname", strings.TrimSpace(string(name))).Run()
		}
	}
	respond(w, 200, nil, "uci commit success")
}
