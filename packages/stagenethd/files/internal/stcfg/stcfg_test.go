package stcfg

import (
	"testing"

	"stageneth/internal/ucifile"
)

func TestParse(t *testing.T) {
	f := ucifile.Parse(`
config stageneth 'globals'
	option enabled '1'
	option schema_version '2'
	option trunk 'eth1'

config service 'mgmt'
	option vlan_id '10'
	option description 'Admin'
	list alt_ip '10.0.0.1/24'
	list alt_ip '10.0.0.2/24'

config binding 'sw0p1'
	option iface 'eth0'
	option service 'mgmt'

config forwarding 'fwd1'
	option src 'mgmt'
	option dst 'guest'
	option enabled '0'

config logging 'remote'
	option host '10.0.0.5'

config unrelated 'x'
	option foo 'bar'
`)
	c := Parse(f)
	if c.SchemaVersion != "2" {
		t.Errorf("schema_version = %q", c.SchemaVersion)
	}
	if c.Globals["trunk"] != "eth1" {
		t.Errorf("trunk = %q", c.Globals["trunk"])
	}
	if c.Services["mgmt"]["vlan_id"] != "10" {
		t.Errorf("mgmt vlan = %q", c.Services["mgmt"]["vlan_id"])
	}
	if got := c.Services["mgmt"]["alt_ip"]; got != "10.0.0.1/24 10.0.0.2/24" {
		t.Errorf("list join = %q", got)
	}
	if c.Bindings["sw0p1"]["service"] != "mgmt" {
		t.Error("binding parse failed")
	}
	if c.Forwardings["fwd1"]["enabled"] != "0" {
		t.Error("forwarding parse failed")
	}
	if c.Logging["remote"]["host"] != "10.0.0.5" {
		t.Error("logging parse failed")
	}
	if len(c.Bindings) != 1 || len(c.Services) != 1 {
		t.Error("unrelated section leaked into typed maps")
	}
	if got := c.ServiceNames(); len(got) != 1 || got[0] != "mgmt" {
		t.Errorf("ServiceNames = %v", got)
	}
	if got := c.SortedBindings(); len(got) != 1 || got[0]["iface"] != "eth0" {
		t.Errorf("SortedBindings = %v", got)
	}
}

func TestSortedOrder(t *testing.T) {
	f := ucifile.Parse(`
config binding 'z'
	option iface 'e1'
config binding 'a'
	option iface 'e2'
`)
	c := Parse(f)
	got := c.SortedBindings()
	if len(got) != 2 || got[0]["iface"] != "e2" || got[1]["iface"] != "e1" {
		t.Errorf("SortedBindings order = %v", got)
	}
}
