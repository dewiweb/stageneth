package render_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"stageneth/internal/render"
	"stageneth/internal/stcfg"
	"stageneth/internal/ucifile"
)

// Golden test: parse a representative /etc/config/stageneth, render every
// managed output, and compare against testdata/golden.txt.
// Regenerate with: UPDATE_GOLDEN=1 go test ./internal/render/
// The golden file IS the review surface for generated config — a diff in
// it means a diff in what lands on the router.

var testAvail = map[string]bool{
	"eth0": true, "eth1": true, "eth2": true, "eth3": true, "lo": true,
}

func serialize(secs []ucifile.Section) string {
	f := &ucifile.File{Sections: secs}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		panic(err)
	}
	return buf.String()
}

func renderAll(st *stcfg.Config) string {
	out := ""
	for _, cfg := range []struct {
		name string
		secs []ucifile.Section
	}{
		{"network", render.Network(st, testAvail)},
		{"firewall", render.Firewall(st)},
		{"dhcp", render.Dhcp(st)},
		{"rsyslog", render.Rsyslog(st)},
		{"umdns", render.Umdns(st)},
		{"mdns_repeater", render.MdnsRepeater(st)},
	} {
		out += fmt.Sprintf("### %s\n%s", cfg.name, serialize(cfg.secs))
	}
	out += "### rsyslog.conf collector block\n" + render.CollectorBlock(st) + "\n"
	return out
}

const fullConfig = `
config stageneth 'globals'
	option schema_version '2'
	option switch_vendor 'generic'
	option trunk 'eth1'
	option firstboot_done '1'

config service 'mgmt'
	option vlan_id '10'
	option ipaddr '192.168.10.1'
	option netmask '255.255.255.0'
	option untagged '1'
	option dhcp '1'

config service 'dante'
	option vlan_id '20'
	option ipaddr '10.20.0.1'
	option netmask '255.255.255.0'
	option multicast '1'
	option dhcp '1'

config service 'st2110'
	option vlan_id '30'
	option ipaddr '10.30.0.1'
	option netmask '255.255.255.0'
	option mtu '9000'
	option multicast '1'
	option ptp_timestamping '1'

config service 'aes67'
	option vlan_id '40'
	option ipaddr '10.40.0.1'
	option netmask '255.255.255.0'
	option multicast '1'

config binding 'b_dante_e2'
	option service 'dante'
	option interface 'eth2'

config binding 'b_st_e3'
	option service 'st2110'
	option interface 'eth3'

config binding 'b_st_trunk'
	option service 'st2110'
	option interface 'eth1'

config forwarding 'dante_to_aes67'
	option src 'dante'
	option dest 'aes67'
	option enabled '1'

config forwarding 'guest_to_mgmt_off'
	option src 'guest'
	option dest 'mgmt'
	option enabled '0'

config logging 'remote'
	option enabled '1'
	option host 'logs.example.local'
	option port '1514'
	option protocol 'tcp'

config logging 'collector'
	option enabled '1'
	option port '514'
	option protocol 'both'
	option dir '/var/log/remote'
`

const minimalConfig = `
config stageneth 'globals'
	option schema_version '2'

config service 'mgmt'
	option vlan_id '1'
	option untagged '1'
`

func TestGoldenRender(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cfg    string
		golden string
	}{
		{"full", fullConfig, "golden_full.txt"},
		{"minimal", minimalConfig, "golden_minimal.txt"},
	} {
		st := stcfg.Parse(ucifile.Parse(tc.cfg))
		got := renderAll(st)
		path := filepath.Join("testdata", tc.golden)
		if os.Getenv("UPDATE_GOLDEN") == "1" {
			if err := os.WriteFile(path, []byte(got), 0644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: golden missing (%v) — regenerate with UPDATE_GOLDEN=1", tc.name, err)
		}
		if got != string(want) {
			t.Fatalf("%s: render output diverged from %s — inspect the diff, then regenerate with UPDATE_GOLDEN=1 if intended\n--- got ---\n%s", tc.name, tc.golden, got)
		}
	}
}
