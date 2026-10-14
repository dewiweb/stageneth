package api

import (
	json "encoding/json"
	fmt "fmt"
	net "net"
	http "net/http"
	os "os"
	"path/filepath"
	regexp "regexp"
	sort "sort"
	strconv "strconv"
	strings "strings"
	sync "sync"
	time "time"

	"stageneth/internal/stcfg"
	"stageneth/internal/ucifile"
)

// StagenethConfig is the source-of-truth config file path.
var StagenethConfig = filepath.Join(ucifile.ConfigDir, "stageneth")

// stagenethFileMu serializes read-modify-write cycles on the stageneth
// config file — concurrent PUTs would otherwise lose each other's edits.
var stagenethFileMu sync.Mutex

// --- validation ---------------------------------------------------------------

var (
	nameRe  = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)
	ifaceRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,15}$`)
	hostRe  = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,253}$`)
	dirRe   = regexp.MustCompile(`^/[A-Za-z0-9_/.-]{0,127}$`)
)

func validBool(s string) bool { return s == "0" || s == "1" }

func intRange(s string, lo, hi int, emptyOK bool) bool {
	if s == "" {
		return emptyOK
	}
	n, err := strconv.Atoi(s)
	return err == nil && n >= lo && n <= hi
}

func ipv4(s string) bool { return net.ParseIP(s) != nil && net.ParseIP(s).To4() != nil }

func ipv4OrEmpty(s string) bool { return s == "" || ipv4(s) }

func saneText(s string) bool {
	if len(s) > 64 {
		return false
	}
	for _, r := range s {
		if r < 0x20 {
			return false
		}
	}
	return true
}

// serviceField validators cover every option the render layer consumes
// plus ptp/description which the UI exposes.
var serviceFields = map[string]func(string) bool{
	"vlan_id":       func(s string) bool { return intRange(s, 1, 4094, false) },
	"ipaddr":        ipv4OrEmpty,
	"netmask":       ipv4OrEmpty,
	"mtu":           func(s string) bool { return intRange(s, 68, 9700, true) },
	"dscp":          func(s string) bool { return intRange(s, 0, 63, true) },
	"priority":      func(s string) bool { return intRange(s, 0, 7, true) },
	"ptp":           validBool,
	"multicast":     validBool,
	"untagged":      validBool,
	"igmp_snooping": validBool,
	"enabled":       validBool,
	"description":   saneText,
}

var globalsFields = map[string]func(string) bool{
	"switch_vendor":    func(s string) bool { return nameRe.MatchString(s) },
	"trunk":            func(s string) bool { return s == "" || ifaceRe.MatchString(s) },
	"ptp_timestamping": func(s string) bool { return s == "" || s == "software" || s == "hardware" },
	"show_lock":        validBool,
	"enabled":          validBool,
}

func validateFields(fields map[string]func(string) bool, vals map[string]string) error {
	for k, v := range vals {
		vf, ok := fields[k]
		if !ok {
			return fmt.Errorf("unknown option %q", k)
		}
		if !vf(v) {
			return fmt.Errorf("invalid value for %q", k)
		}
	}
	return nil
}

// --- stageneth file access -----------------------------------------------------

func loadStageneth() (*ucifile.File, *stcfg.Config, error) {
	f, err := ucifile.Read(StagenethConfig)
	if err != nil {
		if os.IsNotExist(err) {
			f = &ucifile.File{}
		} else {
			return nil, nil, err
		}
	}
	return f, stcfg.Parse(f), nil
}

func saveStageneth(f *ucifile.File) error {
	tmp := StagenethConfig + ".tmp"
	w, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if err := f.Write(w); err != nil {
		w.Close()
		return err
	}
	if err := w.Sync(); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, StagenethConfig); err != nil {
		return err
	}
	// fsync the dir so the rename survives a power cut
	if d, err := os.Open(filepath.Dir(StagenethConfig)); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

func findSection(f *ucifile.File, typ, name string) *ucifile.Section {
	for i := range f.Sections {
		if f.Sections[i].Type == typ && f.Sections[i].Name == name {
			return &f.Sections[i]
		}
	}
	return nil
}

// replaceSectionOptions writes vals as the section's complete option set,
// in sorted order so file output is deterministic.
func replaceSectionOptions(s *ucifile.Section, vals map[string]string) {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	s.Options = nil
	for _, k := range keys {
		s.Options = append(s.Options, ucifile.Option{Name: k, Value: vals[k]})
	}
}

// --- handlers ------------------------------------------------------------------

func decodeBody(r *http.Request, dst interface{}) bool {
	return json.NewDecoder(r.Body).Decode(dst) == nil
}

// v1State returns the typed stageneth configuration.
func v1State(w http.ResponseWriter, r *http.Request) {
	_, st, err := loadStageneth()
	if err != nil {
		respond(w, 500, nil, "config load failed")
		return
	}
	respond(w, 200, st, "ok")
}

func v1SetGlobals(w http.ResponseWriter, r *http.Request) {
	var vals map[string]string
	if !decodeBody(r, &vals) || len(vals) == 0 {
		respond(w, 400, nil, "invalid body")
		return
	}
	if err := validateFields(globalsFields, vals); err != nil {
		respond(w, 400, nil, err.Error())
		return
	}
	stagenethFileMu.Lock()
	defer stagenethFileMu.Unlock()
	f, _, err := loadStageneth()
	if err != nil {
		respond(w, 500, nil, "config load failed")
		return
	}
	s := findSection(f, "stageneth", "globals")
	if s == nil {
		f.Sections = append([]ucifile.Section{{Type: "stageneth", Name: "globals"}}, f.Sections...)
		s = &f.Sections[0]
	}
	for k, v := range vals {
		s.Set(k, v)
	}
	if err := saveStageneth(f); err != nil {
		respond(w, 500, nil, "config save failed")
		return
	}
	audit("v1_globals", "", r)
	respond(w, 200, nil, "globals saved")
}

// v1UpsertSection handles PUT /api/v1/{services|bindings|forwardings}/{name}.
func v1UpsertSection(sectionType string, extraValidate func(vals map[string]string, st *stcfg.Config) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !nameRe.MatchString(name) {
			respond(w, 400, nil, "invalid name")
			return
		}
		var vals map[string]string
		if !decodeBody(r, &vals) {
			respond(w, 400, nil, "invalid body")
			return
		}
		stagenethFileMu.Lock()
		defer stagenethFileMu.Unlock()
		f, st, err := loadStageneth()
		if err != nil {
			respond(w, 500, nil, "config load failed")
			return
		}
		if extraValidate != nil {
			if err := extraValidate(vals, st); err != nil {
				respond(w, 400, nil, err.Error())
				return
			}
		}
		// one vlan_id = one broadcast domain — two services sharing a
		// vlan would render colliding trunk subinterfaces
		if sectionType == "service" {
			if vid := vals["vlan_id"]; vid != "" {
				for other, sv := range st.Services {
					if other != name && sv["vlan_id"] == vid {
						respond(w, 409, nil, "vlan_id "+vid+" already used by service "+other)
						return
					}
				}
			}
		}
		s := findSection(f, sectionType, name)
		if s == nil {
			f.Sections = append(f.Sections, ucifile.Section{Type: sectionType, Name: name})
			s = &f.Sections[len(f.Sections)-1]
		}
		replaceSectionOptions(s, vals)
		if err := saveStageneth(f); err != nil {
			respond(w, 500, nil, "config save failed")
			return
		}
		audit("v1_set_"+sectionType, name, r)
		respond(w, 200, nil, sectionType+" saved")
	}
}

func v1DeleteSection(sectionType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if sectionType == "service" && name == "mgmt" {
			respond(w, 400, nil, "the mgmt service cannot be deleted")
			return
		}
		stagenethFileMu.Lock()
		defer stagenethFileMu.Unlock()
		f, st, err := loadStageneth()
		if err != nil {
			respond(w, 500, nil, "config load failed")
			return
		}
		if sectionType == "service" {
			// bindings referencing this service would dangle — silently
			// dead today, resurrected if the name is ever reused
			for bn, bv := range st.Bindings {
				if bv["service"] == name {
					respond(w, 409, nil, "service "+name+" still bound by "+bn)
					return
				}
			}
		}
		out := f.Sections[:0]
		found := false
		for _, s := range f.Sections {
			if s.Type == sectionType && s.Name == name {
				found = true
				continue
			}
			out = append(out, s)
		}
		if !found {
			respond(w, 404, nil, "section not found")
			return
		}
		f.Sections = out
		if err := saveStageneth(f); err != nil {
			respond(w, 500, nil, "config save failed")
			return
		}
		audit("v1_delete_"+sectionType, name, r)
		respond(w, 200, nil, sectionType+" deleted")
	}
}

func v1ValidateService(vals map[string]string, st *stcfg.Config) error {
	if len(vals) == 0 {
		return fmt.Errorf("invalid body")
	}
	if err := validateFields(serviceFields, vals); err != nil {
		return err
	}
	return nil
}

func v1ValidateBinding(vals map[string]string, st *stcfg.Config) error {
	if len(vals) == 0 {
		return fmt.Errorf("invalid body")
	}
	for k, v := range vals {
		switch k {
		case "interface":
			if v != "" && !ifaceRe.MatchString(v) {
				return fmt.Errorf("invalid interface %q", v)
			}
		case "service":
			if _, ok := st.Services[v]; !ok {
				return fmt.Errorf("unknown service %q", v)
			}
		default:
			return fmt.Errorf("unknown option %q", k)
		}
	}
	return nil
}

func v1ValidateForwarding(vals map[string]string, st *stcfg.Config) error {
	if len(vals) == 0 {
		return fmt.Errorf("invalid body")
	}
	for k, v := range vals {
		switch k {
		case "src", "dest":
			if _, ok := st.Services[v]; !ok {
				return fmt.Errorf("unknown service %q for %s", v, k)
			}
		case "enabled":
			if !validBool(v) {
				return fmt.Errorf("invalid enabled %q", v)
			}
		default:
			return fmt.Errorf("unknown option %q", k)
		}
	}
	return nil
}

func v1SetLogging(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name != "remote" && name != "collector" {
		respond(w, 400, nil, "unknown logging section")
		return
	}
	var vals map[string]string
	if !decodeBody(r, &vals) {
		respond(w, 400, nil, "invalid body")
		return
	}
	for k, v := range vals {
		var ok bool
		switch k {
		case "enabled":
			ok = validBool(v)
		case "host":
			ok = v == "" || hostRe.MatchString(v)
		case "port":
			ok = intRange(v, 1, 65535, true)
		case "protocol":
			ok = v == "udp" || v == "tcp" || (name == "collector" && v == "both")
		case "dir":
			ok = name == "collector" && dirRe.MatchString(v) && !strings.Contains(v, "..")
		default:
			respond(w, 400, nil, "unknown option "+strconv.Quote(k))
			return
		}
		if !ok {
			respond(w, 400, nil, "invalid value for "+strconv.Quote(k))
			return
		}
	}
	stagenethFileMu.Lock()
	defer stagenethFileMu.Unlock()
	f, _, err := loadStageneth()
	if err != nil {
		respond(w, 500, nil, "config load failed")
		return
	}
	s := findSection(f, "logging", name)
	if s == nil {
		f.Sections = append(f.Sections, ucifile.Section{Type: "logging", Name: name})
		s = &f.Sections[len(f.Sections)-1]
	}
	replaceSectionOptions(s, vals)
	if err := saveStageneth(f); err != nil {
		respond(w, 500, nil, "config save failed")
		return
	}
	audit("v1_logging", name, r)
	respond(w, 200, nil, "logging saved")
}

// --- SSE -----------------------------------------------------------------------

// sseTick is the push cadence for v1Events; a var so tests can shorten it.
var sseTick = 5 * time.Second

// v1Events streams monitoring snapshots as Server-Sent Events.
func v1Events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		respond(w, 500, nil, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	tick := time.NewTicker(sseTick)
	defer tick.Stop()
	var lastTS float64
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			snap := snapshot()
			if snap == nil {
				continue
			}
			// only push when a new sample was collected
			if ts := mapFloat(snap, "collected_at"); ts == lastTS {
				continue
			} else {
				lastTS = ts
			}
			b, err := json.Marshal(snap)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}
