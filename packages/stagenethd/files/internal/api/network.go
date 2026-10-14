package api

import (
	context "context"
	json "encoding/json"
	fmt "fmt"
	"net"
	http "net/http"
	os "os"
	exec "os/exec"
	"path/filepath"
	regexp "regexp"
	sort "sort"
	"stageneth/internal/auth"
	"stageneth/internal/ucifile"
	strings "strings"
	time "time"
)

// stagenethdBin is the engine binary the API shells out to for
// apply/preview — overridable for tests and non-standard installs.
var stagenethdBin = func() string {
	if b := os.Getenv("STAGENETHD_BIN"); b != "" {
		return b
	}
	return "/usr/sbin/stagenethd"
}()

func applyNetwork(w http.ResponseWriter, r *http.Request) {
	// hard bound — a hung apply would otherwise pin this handler and,
	// via the engine flock, every subsequent apply
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, stagenethdBin, "apply").CombinedOutput()
	if err != nil {
		code := 500
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 3 {
			code = 409
		}
		audit("apply", "failed: "+firstLine(string(out)), r)
		respond(w, code, map[string]string{"log": string(out)}, "apply failed")
		return
	}
	audit("apply", "ok: "+firstLine(string(out)), r)
	respond(w, 200, map[string]string{"log": string(out)}, "apply success")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
func previewNetwork(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, stagenethdBin, "preview").CombinedOutput()
	if err != nil {
		respond(w, 500, map[string]string{"log": string(out)}, "preview failed")
		return
	}
	respond(w, 200, map[string]string{"diff": string(out)}, "preview")
}
func networkReload(w http.ResponseWriter, r *http.Request) {
	for _, svc := range []string{"network", "firewall", "dnsmasq"} {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		err := exec.CommandContext(ctx, initDir()+"/"+svc, "reload").Run()
		cancel()
		if err != nil {
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
func pingFromRouter(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if net.ParseIP(ip) == nil {
		respond(w, 400, nil, "invalid ip")
		return
	}
	out, err := outBounded(5*time.Second, "ping", "-c", "1", "-W", "2", ip)
	respond(w, 200, map[string]interface{}{"ok": err == nil, "output": string(out)}, "ping done")
}

var backupPackages = []string{"stageneth", "stagenethd", "network", "dhcp", "firewall", "system"}

func backupConfig(w http.ResponseWriter, r *http.Request) {
	var out strings.Builder
	out.WriteString("# StageNeth configuration backup\n")
	fmt.Fprintf(&out, "# packages: %s\n", strings.Join(backupPackages, ", "))
	for _, pkg := range backupPackages {
		part, err := uciCommand("export", pkg).Output()
		if err != nil {
			fmt.Fprintf(&out, "\n# --- package %s export failed ---\n", pkg)
			continue
		}
		fmt.Fprintf(&out, "\n# --- package: %s ---\n", pkg)
		out.Write(part)
	}
	respond(w, 200, map[string]string{"data": out.String()}, "backup ready")
}

func uciImport(pkg, data string) error {
	tmp, err := os.CreateTemp("", "stageneth-restore-*.cfg")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if _, err := uciCommand("import", "-f", tmpName, pkg).CombinedOutput(); err != nil {
		return err
	}
	_, _ = uciCommand("commit", pkg).CombinedOutput()
	return nil
}

func restoreConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&body); err != nil {
		respond(w, 400, nil, "invalid body")
		return
	}
	re := regexp.MustCompile(`(?m)^package '([^']+)'$`)
	matches := re.FindAllStringSubmatchIndex(body.Data, -1)
	if matches == nil {
		respond(w, 400, nil, "no package found in backup")
		return
	}
	allowed := map[string]bool{}
	for _, p := range backupPackages {
		allowed[p] = true
	}
	var failed []string
	type pkgBlock struct{ name, data string }
	var blocks []pkgBlock
	for i, m := range matches {
		pkg := body.Data[m[2]:m[3]]
		start := m[0]
		end := len(body.Data)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		if !allowed[pkg] {
			failed = append(failed, pkg+": not a backup package")
			continue
		}
		blocks = append(blocks, pkgBlock{pkg, body.Data[start:end]})
	}
	if len(failed) > 0 {
		respond(w, 400, nil, "restore rejected: "+strings.Join(failed, ", "))
		return
	}
	// snapshot current state of every package we're about to touch so a
	// mid-restore failure rolls back instead of leaving a mixed config
	prev := map[string]string{}
	for _, b := range blocks {
		out, _ := uciCommand("export", b.name).Output()
		prev[b.name] = string(out)
	}
	for _, b := range blocks {
		if err := uciImport(b.name, b.data); err != nil {
			for p, snap := range prev {
				if snap == "" {
					_ = os.Remove(filepath.Join(ucifile.ConfigDir, p)) // package didn't exist before
				} else {
					_ = uciImport(p, snap) // export output includes the package header
				}
			}
			respond(w, 500, nil, "restore failed for "+b.name+", rolled back")
			return
		}
	}
	respond(w, 200, nil, "config restored")
}

// firstboot is one-way: once an account exists the wizard refuses to
// run again (an unauthenticated re-run could overwrite the admin
// password). Re-provisioning is done via the presets endpoint.
func markFirstbootDone() bool {
	stagenethFileMu.Lock()
	defer stagenethFileMu.Unlock()
	f, _, err := loadStageneth()
	if err != nil {
		return false
	}
	s := findSection(f, "stageneth", "globals")
	if s == nil {
		f.Sections = append([]ucifile.Section{{Type: "stageneth", Name: "globals",
			Options: []ucifile.Option{{Name: "firstboot_done", Value: "1"}}}}, f.Sections...)
	} else {
		s.Set("firstboot_done", "1")
	}
	return saveStageneth(f) == nil
}

func firstbootDone() bool {
	f, _, err := loadStageneth()
	if err != nil {
		return false
	}
	if s := findSection(f, "stageneth", "globals"); s != nil {
		return s.Get("firstboot_done") == "1"
	}
	return false
}
func listServices() []string {
	f, _, err := loadStageneth()
	if err != nil {
		return []string{}
	}
	names := []string{}
	for _, s := range f.Sections {
		if s.Type == "service" {
			names = append(names, s.Name)
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

// setConsolePassword provisions the system root password so SSH/serial
// console matches the credential created in the wizard. This is
// provisioning, not authentication — the API never reads /etc/shadow.
func setConsolePassword(password string) {
	for _, args := range [][]string{{"-a", "sha512"}, {}} {
		cmd := boundedCmd(10*time.Second, "chpasswd", args...)
		cmd.Stdin = strings.NewReader("root:" + password + "\n")
		if err := cmd.Run(); err == nil {
			return
		}
	}
	// last resort: interactive passwd fed twice on stdin
	cmd := boundedCmd(10*time.Second, "passwd", "root")
	cmd.Stdin = strings.NewReader(password + "\n" + password + "\n")
	cmd.Run()
}

// grantSession creates a session cookie after firstboot provisioning.
func grantSession(w http.ResponseWriter, user, role string) bool {
	id, err := sessions.Create(user, role)
	if err != nil {
		respond(w, 500, nil, "session error")
		return false
	}
	auth.SetCookie(w, id, 12*time.Hour)
	return true
}

func wizard(w http.ResponseWriter, r *http.Request) {
	if firstbootDone() || usersDB.HasUsers() {
		respond(w, 403, nil, "wizard already completed")
		return
	}
	var req struct {
		Password string   `json:"password"`
		Preset   string   `json:"preset"`
		Services []string `json:"services"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Password == "" {
		respond(w, 400, nil, "password required")
		return
	}
	if err := usersDB.Set("admin", "tech", req.Password); err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	// mark firstboot done BEFORE anything else can fail — while done is
	// false an unauthenticated retry could overwrite the admin password
	if !markFirstbootDone() {
		respond(w, 500, nil, "config save failed")
		return
	}
	setConsolePassword(req.Password)
	if req.Preset == "" {
		req.Preset = "base"
	}
	out, err := applyPresetToUci(req.Preset, req.Services)
	if err != nil {
		respond(w, 500, map[string]string{"log": out}, "network apply failed")
		return
	}
	if !grantSession(w, "admin", "tech") {
		return
	}
	respond(w, 200, map[string]string{"user": "admin", "role": "tech"}, "wizard complete")
}

func wizardSkip(w http.ResponseWriter, r *http.Request) {
	if firstbootDone() || usersDB.HasUsers() {
		respond(w, 403, nil, "wizard already completed")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Password == "" {
		respond(w, 400, nil, "password required")
		return
	}
	if err := usersDB.Set("admin", "tech", req.Password); err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	if !markFirstbootDone() {
		respond(w, 500, nil, "config save failed")
		return
	}
	setConsolePassword(req.Password)
	if !grantSession(w, "admin", "tech") {
		return
	}
	respond(w, 200, map[string]string{"user": "admin", "role": "tech"}, "wizard skipped")
}
