// Package engine merges rendered desired-state sections into UCI config
// files using a persisted manifest of previously generated sections,
// writes files atomically, and reports which configs changed.
package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"stageneth/internal/render"
	"stageneth/internal/stcfg"
	"stageneth/internal/ucifile"
)

// managedConfig describes one UCI config file StageNeth writes to.
type managedConfig struct {
	name      string // config name / file name in ConfigDir
	render    func(*stcfg.Config, map[string]bool) []ucifile.Section
	fullOwner bool // true: file is fully rendered (no merge)
}

var managed = []managedConfig{
	{"network", render.Network, false},
	{"firewall", func(st *stcfg.Config, _ map[string]bool) []ucifile.Section { return render.Firewall(st) }, false},
	{"dhcp", func(st *stcfg.Config, _ map[string]bool) []ucifile.Section { return render.Dhcp(st) }, false},
	{"rsyslog", func(st *stcfg.Config, _ map[string]bool) []ucifile.Section { return render.Rsyslog(st) }, false},
	{"umdns", func(st *stcfg.Config, _ map[string]bool) []ucifile.Section { return render.Umdns(st) }, true},
	{"mdns_repeater", func(st *stcfg.Config, _ map[string]bool) []ucifile.Section { return render.MdnsRepeater(st) }, true},
}

const rsyslogConfPath = "/etc/rsyslog.conf"
const collectorBegin = "### STAGENETH COLLECTOR BEGIN ###"
const collectorEnd = "### STAGENETH COLLECTOR END ###"

var collectorBlockRe = regexp.MustCompile(`(?s)` + regexp.QuoteMeta(collectorBegin) + `.*` + regexp.QuoteMeta(collectorEnd) + `\n?`)

// Engine computes and applies config plans.
type Engine struct {
	ConfigDir    string          // directory holding UCI files ("/etc/config")
	ManifestPath string          // path to the generated-sections manifest
	RsyslogConf  string          // path to /etc/rsyslog.conf
	Avail        map[string]bool // kernel interfaces (nil → read sysfs)
}

// New returns an Engine for the live system. STAGENETH_STATE_DIR and
// STAGENETH_RSYSLOG_CONF relocate state for tests/recovery mounts.
func New() *Engine {
	state := os.Getenv("STAGENETH_STATE_DIR")
	if state == "" {
		state = "/etc/stageneth"
	}
	rs := rsyslogConfPath
	if p := os.Getenv("STAGENETH_RSYSLOG_CONF"); p != "" {
		rs = p
	}
	return &Engine{
		ConfigDir:    ucifile.ConfigDir,
		ManifestPath: filepath.Join(state, "manifest.json"),
		RsyslogConf:  rs,
	}
}

type manifest map[string][]string

// loadManifest returns the persisted manifest and whether it existed.
func (e *Engine) loadManifest() (manifest, bool) {
	m := manifest{}
	b, err := os.ReadFile(e.ManifestPath)
	if err != nil {
		return m, false
	}
	json.Unmarshal(b, &m)
	return m, true
}

var legacyPrefixes = []string{"svc_", "br_", "zone_", "allow_dhcp_", "allow_dns_", "allow_ntp_", "allow_syslog_"}

// legacyManaged reconstructs which sections the pre-stagenethd orchestrator
// generated, for one-time migration when no manifest exists yet.
func legacyManaged(name string, st *stcfg.Config) bool {
	for _, p := range legacyPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	if strings.Contains(name, "_to_") {
		src := strings.SplitN(name, "_to_", 2)[0]
		if _, ok := st.Services[src]; ok || src == "mgmt" {
			return true
		}
	}
	return false
}

func (e *Engine) saveManifest(m manifest) error {
	if err := os.MkdirAll(filepath.Dir(e.ManifestPath), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(e.ManifestPath, b, 0644)
}

// writeFileAtomic writes via tmp+fsync+rename and fsyncs the directory —
// durable across a power cut, which matters on an appliance that can be
// unplugged mid-apply.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	// fsync the parent dir so the rename itself is durable
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

// lock serializes concurrent applies (API request, procd trigger, CLI)
// via an advisory flock on <manifest dir>/apply.lock.
func (e *Engine) lock() (*os.File, error) {
	p := filepath.Join(filepath.Dir(e.ManifestPath), "apply.lock")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// SectionChange describes a section-level difference.
type FileChange struct {
	Config   string
	Path     string
	Changed  bool
	Added    []string // desired sections not previously present
	Removed  []string // generated sections no longer desired
	Modified []string // sections present in both but differing
	OldText  string
	NewText  string
}

// Plan is the full set of file changes to apply.
type Plan struct {
	Changes         []FileChange
	RsyslogConfDiff bool // collector block in /etc/rsyslog.conf changed
}

func sectionKey(s ucifile.Section) string {
	if s.Name != "" {
		return s.Name
	}
	return "@" + s.Type
}

func fmtOpts(opts []ucifile.Option) string {
	var parts []string
	for _, o := range opts {
		kw := "option"
		if o.List {
			kw = "list"
		}
		parts = append(parts, fmt.Sprintf("%s %s=%q", kw, o.Name, o.Value))
	}
	return strings.Join(parts, ";")
}

// merge builds the new file: passthrough sections (existing, minus stale
// generated, minus names colliding with desired) followed by desired.
func merge(existing *ucifile.File, desired []ucifile.Section, prevManaged map[string]bool) *ucifile.File {
	desiredNames := map[string]bool{}
	for _, s := range desired {
		desiredNames[sectionKey(s)] = true
	}
	out := &ucifile.File{}
	for _, s := range existing.Sections {
		key := sectionKey(s)
		if prevManaged[key] || desiredNames[key] {
			continue
		}
		out.Sections = append(out.Sections, s)
	}
	out.Sections = append(out.Sections, desired...)
	return out
}

// Plan computes what apply would change, without writing anything.
func (e *Engine) Plan(st *stcfg.Config) (*Plan, error) {
	avail := e.Avail
	if avail == nil {
		avail = render.AvailableInterfaces()
	}
	mf, mfExists := e.loadManifest()
	plan := &Plan{}
	for _, mc := range managed {
		path := filepath.Join(e.ConfigDir, mc.name)
		old := ""
		if b, err := os.ReadFile(path); err == nil {
			old = string(b)
		}
		existing := ucifile.Parse(old)
		desired := mc.render(st, avail)

		var merged *ucifile.File
		if mc.fullOwner {
			merged = &ucifile.File{Sections: desired}
		} else {
			prev := map[string]bool{}
			if mfExists {
				for _, n := range mf[mc.name] {
					prev[n] = true
				}
			} else {
				// No manifest yet: one-time cleanup of sections generated
				// by the pre-stagenethd orchestrator.
				for _, s := range existing.Sections {
					if s.Name != "" && legacyManaged(s.Name, st) {
						prev[s.Name] = true
					}
				}
			}
			merged = merge(existing, desired, prev)
		}
		newText := merged.Text()

		fc := FileChange{Config: mc.name, Path: path, OldText: old, NewText: newText}
		fc.Changed = strings.TrimSpace(old) != strings.TrimSpace(newText)

		oldSections := map[string]ucifile.Section{}
		oldManaged := map[string]bool{}
		for _, n := range mf[mc.name] {
			oldManaged[n] = true
		}
		for _, s := range existing.Sections {
			oldSections[sectionKey(s)] = s
		}
		desiredSet := map[string]ucifile.Section{}
		for _, s := range desired {
			k := sectionKey(s)
			desiredSet[k] = s
			if _, ok := oldSections[k]; !ok {
				fc.Added = append(fc.Added, k)
			} else if fmtOpts(oldSections[k].Options) != fmtOpts(s.Options) {
				fc.Modified = append(fc.Modified, k)
			}
		}
		for _, n := range mf[mc.name] {
			if _, ok := desiredSet[n]; !ok {
				fc.Removed = append(fc.Removed, n)
			}
		}
		if mc.fullOwner {
			for _, s := range existing.Sections {
				k := sectionKey(s)
				if _, ok := desiredSet[k]; !ok && !contains(fc.Added, k) {
					fc.Removed = append(fc.Removed, k)
				}
			}
		}
		plan.Changes = append(plan.Changes, fc)
	}

	// Collector block in /etc/rsyslog.conf lives outside ConfigDir.
	confPath := e.RsyslogConf
	if confPath == "" {
		confPath = rsyslogConfPath
	}
	old, _ := os.ReadFile(confPath)
	newText := e.collectorConfText(string(old), st)
	plan.RsyslogConfDiff = newText != string(old)

	return plan, nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func (e *Engine) collectorConfText(content string, st *stcfg.Config) string {
	desired := collectorBlockRe.ReplaceAllString(content, "")
	if block := render.CollectorBlock(st); block != "" {
		if desired != "" && !strings.HasSuffix(desired, "\n") {
			desired += "\n"
		}
		desired += collectorBegin + "\n" + block + "\n" + collectorEnd + "\n"
	}
	return desired
}

// Snapshot captures the pre-apply contents of every file the engine may
// write, so a failed apply or reload can be rolled back.
type Snapshot struct {
	files map[string]fileBackup
}

type fileBackup struct {
	existed bool
	content []byte
}

// snapshot reads the current contents of all managed paths.
func (e *Engine) snapshot() *Snapshot {
	s := &Snapshot{files: map[string]fileBackup{}}
	paths := make([]string, 0, len(managed)+1)
	for _, mc := range managed {
		paths = append(paths, filepath.Join(e.ConfigDir, mc.name))
	}
	confPath := e.RsyslogConf
	if confPath == "" {
		confPath = rsyslogConfPath
	}
	paths = append(paths, confPath)
	for _, p := range paths {
		b, err := os.ReadFile(p)
		s.files[p] = fileBackup{existed: err == nil, content: b}
	}
	return s
}

// Restore writes back the snapshotted file contents (and removes files that
// did not exist at snapshot time). Best-effort: errors are aggregated.
func (e *Engine) Restore(s *Snapshot) error {
	var errs []string
	for p, b := range s.files {
		var err error
		if b.existed {
			err = writeFileAtomic(p, b.content, 0644)
		} else {
			err = os.Remove(p)
			if os.IsNotExist(err) {
				err = nil
			}
		}
		if err != nil {
			errs = append(errs, p+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("rollback partial: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (e *Engine) lastGoodDir() string {
	return filepath.Join(filepath.Dir(e.ManifestPath), "last-good")
}

// SaveLastGood persists a snapshot under <manifest dir>/last-good so the
// last working configuration survives process exit — the console escape
// hatch after an apply that severs management access. Serialized with
// Apply/Rollback so concurrent applies can't interleave with the write.
func (e *Engine) SaveLastGood(s *Snapshot) error {
	lk, err := e.lock()
	if err != nil {
		return fmt.Errorf("apply lock: %w", err)
	}
	defer lk.Close()
	dir := e.lastGoodDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	type lastGoodEntry struct {
		Path    string `json:"path"`
		Existed bool   `json:"existed"`
	}
	index := map[string]lastGoodEntry{}
	for p, b := range s.files {
		base := filepath.Base(p)
		index[base] = lastGoodEntry{Path: p, Existed: b.existed}
		if !b.existed {
			continue
		}
		if err := writeFileAtomic(filepath.Join(dir, base), b.content, 0600); err != nil {
			return err
		}
	}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, en := range entries {
			if en.Name() != "index.json" {
				e, ok := index[en.Name()]
				if !ok || !e.Existed {
					os.Remove(filepath.Join(dir, en.Name()))
				}
			}
		}
	}
	jb, _ := json.Marshal(index)
	return writeFileAtomic(filepath.Join(dir, "index.json"), jb, 0600)
}

// Rollback restores the persisted last-good snapshot. Returns the live
// paths that were rewritten. Serialized with Apply: a rollback racing
// an in-flight apply would interleave file writes.
func (e *Engine) Rollback() ([]string, error) {
	lk, err := e.lock()
	if err != nil {
		return nil, fmt.Errorf("apply lock: %w", err)
	}
	defer lk.Close()
	dir := e.lastGoodDir()
	jb, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("no last-good snapshot: %w", err)
	}
	var index map[string]struct {
		Path    string `json:"path"`
		Existed bool   `json:"existed"`
	}
	if err := json.Unmarshal(jb, &index); err != nil {
		return nil, fmt.Errorf("corrupt last-good index: %w", err)
	}
	var restored []string
	var errs []string
	for base, ent := range index {
		if !ent.Existed {
			// apply created this file — rollback removes it
			if err := os.Remove(ent.Path); err != nil && !os.IsNotExist(err) {
				errs = append(errs, ent.Path+": remove")
				continue
			}
			restored = append(restored, ent.Path)
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, base))
		if err != nil {
			errs = append(errs, base+": read")
			continue
		}
		if err := writeFileAtomic(ent.Path, b, 0644); err != nil {
			errs = append(errs, ent.Path+": write")
			continue
		}
		restored = append(restored, ent.Path)
	}
	if len(errs) > 0 {
		return restored, fmt.Errorf("partial rollback: %s", strings.Join(errs, "; "))
	}
	return restored, nil
}

// Apply writes all changed files atomically, updates the manifest, and
// returns the list of config names that changed plus a Snapshot the caller
// can Restore() if service reloads fail. On a write error the files are
// rolled back automatically before returning.
func (e *Engine) Apply(st *stcfg.Config) ([]string, *Snapshot, error) {
	lk, err := e.lock()
	if err != nil {
		return nil, nil, fmt.Errorf("apply lock: %w", err)
	}
	defer lk.Close()
	plan, err := e.Plan(st)
	if err != nil {
		return nil, nil, err
	}
	snap := e.snapshot()
	var changed []string
	mf := manifest{}
	avail := e.Avail
	if avail == nil {
		avail = render.AvailableInterfaces()
	}
	for i, mc := range managed {
		fc := plan.Changes[i]
		desiredNames := []string{}
		for _, s := range mc.render(st, avail) {
			if s.Name != "" {
				desiredNames = append(desiredNames, s.Name)
			}
		}
		mf[mc.name] = desiredNames
		if !fc.Changed {
			continue
		}
		if err := writeFileAtomic(fc.Path, []byte(fc.NewText), 0644); err != nil {
			rerr := e.Restore(snap)
			return nil, nil, fmt.Errorf("write %s: %w (rollback: %v)", fc.Path, err, rerr)
		}
		changed = append(changed, mc.name)
	}
	if plan.RsyslogConfDiff {
		confPath := e.RsyslogConf
		if confPath == "" {
			confPath = rsyslogConfPath
		}
		old, _ := os.ReadFile(confPath)
		if err := writeFileAtomic(confPath, []byte(e.collectorConfText(string(old), st)), 0644); err != nil {
			rerr := e.Restore(snap)
			return nil, nil, fmt.Errorf("write %s: %w (rollback: %v)", confPath, err, rerr)
		} else if !contains(changed, "rsyslog") {
			changed = append(changed, "rsyslog")
		}
	}
	// the manifest must record every generated section — if it can't be
	// written, the configs we just wrote would become untracked "user"
	// sections next apply, so roll the whole thing back
	if err := e.saveManifest(mf); err != nil {
		rerr := e.Restore(snap)
		return nil, nil, fmt.Errorf("manifest write: %w (rollback: %v)", err, rerr)
	}
	return changed, snap, nil
}
