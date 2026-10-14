package ucifile

import "testing"

const sample = `# comment line
config interface 'lan'
	option device 'br-lan'
	option proto 'static'
	list ports 'eth0'
	list ports 'eth1'
	option hostname 'my host'

config device
	option name 'br-lan'
`

func TestRoundTrip(t *testing.T) {
	f := Parse(sample)
	if len(f.Sections) != 2 {
		t.Fatalf("sections: %d", len(f.Sections))
	}
	lan := f.Get("lan")
	if lan == nil || lan.Type != "interface" {
		t.Fatal("lan section not found")
	}
	if lan.Get("hostname") != "my host" {
		t.Errorf("quoted value: %q", lan.Get("hostname"))
	}
	lists := 0
	for _, o := range lan.Options {
		if o.Name == "ports" && o.List {
			lists++
		}
	}
	if lists != 2 {
		t.Errorf("list items: %d", lists)
	}
	if f.Sections[1].Name != "" || f.Sections[1].Type != "device" {
		t.Error("anonymous section broken")
	}

	// emit → reparse → equal
	f2 := Parse(f.Text())
	if f2.Text() != f.Text() {
		t.Fatalf("re-parse diverged:\n%s\n---\n%s", f2.Text(), f.Text())
	}
}

func TestQuoteEdgeCases(t *testing.T) {
	s := Section{Type: "x", Name: "y", Options: []Option{
		{Name: "plain", Value: "abc"},
		{Name: "spaces", Value: "a b c"},
		{Name: "sq", Value: "it's"},
		{Name: "empty", Value: ""},
	}}
	f := &File{Sections: []Section{s}}
	f2 := Parse(f.Text())
	s2 := f2.Sections[0]
	if s2.Get("plain") != "abc" || s2.Get("spaces") != "a b c" || s2.Get("sq") != "it's" || s2.Get("empty") != "" {
		t.Errorf("round-trip values wrong:\n%s", f.Text())
	}
}

func TestGetSet(t *testing.T) {
	f := Parse("config interface 'lan'\n\toption proto 'static'\n")
	s := f.Get("lan")
	if s.Get("proto") != "static" || s.Get("missing") != "" {
		t.Error("get failed")
	}
	s.Set("proto", "dhcp")
	s.Set("ipaddr", "10.0.0.1")
	if s.Get("proto") != "dhcp" || s.Get("ipaddr") != "10.0.0.1" {
		t.Error("set failed")
	}
	if idx := f.Index("lan"); idx != 0 {
		t.Error("index failed")
	}
}
