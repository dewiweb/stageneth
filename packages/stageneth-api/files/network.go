package main

import (
	json "encoding/json"
	fmt "fmt"
	http "net/http"
	os "os"
	exec "os/exec"
	regexp "regexp"
	sort "sort"
	strings "strings"
)

func applyNetwork(w http.ResponseWriter, r *http.Request) {
	out, err := exec.Command("/usr/sbin/stageneth-network", "apply").CombinedOutput()
	if err != nil {
		respond(w, 500, map[string]string{"log": string(out)}, "apply failed")
		return
	}
	respond(w, 200, map[string]string{"log": string(out)}, "apply success")
}
func networkReload(w http.ResponseWriter, r *http.Request) {
	for _, svc := range []string{"network", "firewall", "dnsmasq"} {
		cmd := exec.Command("/etc/init.d/"+svc, "reload")
		if err := cmd.Run(); err != nil {
			respond(w, 500, nil, svc+" reload failed")
			return
		}
	}
	respond(w, 200, nil, "network reloaded")
}
func networkInterfaces(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		respond(w, 500, nil, "failed to read network interfaces")
		return
	}
	interfaces := []map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		mac := readFileTrim("/sys/class/net/" + name + "/address")
		interfaces = append(interfaces, map[string]string{"name": name, "mac": mac})
	}
	respond(w, 200, map[string]interface{}{"interfaces": interfaces}, "interfaces listed")
}
func readFileTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
func parseInterfaceAddrs() map[string]string {
	addrs := map[string]string{}
	b, err := exec.Command("ip", "-o", "-4", "addr", "show").Output()
	if err != nil {
		return addrs
	}
	re := regexp.MustCompile(`(?m)^\d+:\s+(\S+)\s+.*\s+inet\s+([0-9.]+)/`)
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		addrs[m[1]] = m[2]
	}
	return addrs
}
func pingFromRouter(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		respond(w, 400, nil, "missing ip")
		return
	}
	out, err := exec.Command("ping", "-c", "1", "-W", "2", ip).CombinedOutput()
	respond(w, 200, map[string]interface{}{"ok": err == nil, "output": string(out)}, "ping done")
}

var backupPackages = []string{"stageneth", "stageneth-api", "network", "dhcp", "firewall", "system"}

func backupConfig(w http.ResponseWriter, r *http.Request) {
	var out strings.Builder
	out.WriteString("# StageNeth configuration backup\n")
	fmt.Fprintf(&out, "# packages: %s\n", strings.Join(backupPackages, ", "))
	for _, pkg := range backupPackages {
		part, err := exec.Command("uci", "export", pkg).Output()
		if err != nil {
			fmt.Fprintf(&out, "\n# --- package %s export failed ---\n", pkg)
			continue
		}
		fmt.Fprintf(&out, "\n# --- package: %s ---\n", pkg)
		out.Write(part)
	}
	respond(w, 200, map[string]string{"data": out.String()}, "backup ready")
}

func restoreConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respond(w, 400, nil, "invalid body")
		return
	}
	re := regexp.MustCompile(`(?m)^package '([^']+)'$`)
	matches := re.FindAllStringSubmatchIndex(body.Data, -1)
	if matches == nil {
		respond(w, 400, nil, "no package found in backup")
		return
	}
	var failed []string
	for i, m := range matches {
		pkg := body.Data[m[2]:m[3]]
		start := m[0]
		end := len(body.Data)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		block := body.Data[start:end]
		tmp := "/tmp/stageneth-restore-" + pkg + ".cfg"
		if err := os.WriteFile(tmp, []byte(block), 0600); err != nil {
			failed = append(failed, pkg+": write temp")
			continue
		}
		if _, err := exec.Command("sh", "-c", "uci import "+pkg+" < "+tmp).CombinedOutput(); err != nil {
			failed = append(failed, pkg+": uci import")
			continue
		}
		_, _ = exec.Command("uci", "commit", pkg).CombinedOutput()
	}
	if len(failed) > 0 {
		respond(w, 500, nil, "restore failed for "+strings.Join(failed, ", "))
		return
	}
	respond(w, 200, nil, "config restored")
}

func firstbootDone() bool {
	out, _ := exec.Command("uci", "-q", "get", "stageneth.globals.firstboot_done").Output()
	return strings.TrimSpace(string(out)) == "1"
}
func listServices() []string {
	out, err := exec.Command("uci", "-q", "show", "stageneth").Output()
	if err != nil {
		return []string{}
	}
	sections := parseUciShow(string(out))
	names := []string{}
	for s, data := range sections {
		m, ok := data.(map[string]interface{})
		if !ok {
			continue
		}
		if t, ok := m[".type"].(string); ok && t == "service" {
			names = append(names, s)
		}
	}
	sort.Strings(names)
	return names
}
func firstbootStatus(w http.ResponseWriter, r *http.Request) {
	presets := []map[string]interface{}{}
	for _, p := range stagenethPresets {
		svcs := []string{}
		for _, s := range p.Services {
			svcs = append(svcs, s.Name)
		}
		presets = append(presets, map[string]interface{}{
			"name":        p.Name,
			"label":       p.Label,
			"description": p.Description,
			"services":    svcs,
		})
	}
	respond(w, 200, map[string]interface{}{"firstboot_done": firstbootDone(), "services": listServices(), "presets": presets}, "ok")
}
func wizard(w http.ResponseWriter, r *http.Request) {
	if firstbootDone() {
		respond(w, 403, nil, "wizard already completed")
		return
	}
	var req struct {
		Password string   `json:"password"`
		Preset   string   `json:"preset"`
		Services []string `json:"services"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Password == "" {
		respond(w, 400, nil, "invalid wizard request")
		return
	}
	hash, err := opensslHash(req.Password)
	if err != nil {
		respond(w, 500, nil, "hash error")
		return
	}
	if err := exec.Command("sed", "-i", fmt.Sprintf("s|^root:[^:]*:|root:%s:|", hash), "/etc/shadow").Run(); err != nil {
		respond(w, 500, nil, "shadow update error")
		return
	}
	if req.Preset == "" {
		req.Preset = "base"
	}
	out, err := applyPresetToUci(req.Preset, req.Services)
	if err != nil {
		respond(w, 500, map[string]string{"log": out}, "network apply failed")
		return
	}
	exec.Command("uci", "-q", "set", "stageneth.globals=stageneth").Run()
	exec.Command("uci", "-q", "set", "stageneth.globals.firstboot_done=1").Run()
	exec.Command("uci", "-q", "commit", "stageneth").Run()
	respond(w, 200, map[string]string{"token": token("root")}, "wizard complete")
}

func wizardSkip(w http.ResponseWriter, r *http.Request) {
	if firstbootDone() {
		respond(w, 403, nil, "wizard already completed")
		return
	}
	exec.Command("uci", "-q", "set", "stageneth.globals=stageneth").Run()
	exec.Command("uci", "-q", "set", "stageneth.globals.firstboot_done=1").Run()
	exec.Command("uci", "-q", "commit", "stageneth").Run()
	respond(w, 200, map[string]string{"token": token("root")}, "wizard skipped")
}
