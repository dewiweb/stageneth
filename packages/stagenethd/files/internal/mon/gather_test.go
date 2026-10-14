package mon

import "testing"

func TestParseMdbOutput(t *testing.T) {
	out := `dev br_dante port eth1.20 grp 239.69.10.1 temp vid 20
dev br_dante port eth1.20 grp 224.0.0.251 temp vid 20
dev br_mgmt port eth1.10 grp 224.0.0.1 permanent vid 10`
	m := parseMdbOutput(out)
	dante, ok := m["br_dante"].([]map[string]string)
	if !ok || len(dante) != 2 {
		t.Fatalf("br_dante entries = %v", m["br_dante"])
	}
	if dante[0]["group"] != "239.69.10.1" || dante[0]["port"] != "eth1.20" || dante[0]["vid"] != "20" {
		t.Errorf("entry = %+v", dante[0])
	}
	mgmt, ok := m["br_mgmt"].([]map[string]string)
	if !ok || len(mgmt) != 1 {
		t.Fatalf("br_mgmt entries = %v", m["br_mgmt"])
	}
	// malformed lines are skipped
	if got := parseMdbOutput("garbage\n\nshort dev x\n"); len(got) != 0 {
		t.Errorf("malformed input = %v", got)
	}
}

func TestParseMemInfoFields(t *testing.T) {
	// parseMemInfo reads /proc/meminfo on the test host — verify shape only
	m := parseMemInfo()
	tot, _ := m["total_kb"].(uint64)
	if tot == 0 {
		t.Skip("no meminfo")
	}
	if up, _ := m["used_percent"].(float64); up < 0 || up > 100 {
		t.Errorf("used_percent = %v", up)
	}
}

func TestParseNetDevLive(t *testing.T) {
	for _, iface := range parseNetDev() {
		if iface["name"] == "lo" {
			t.Error("lo should be filtered")
		}
		if _, ok := iface["rx_bytes"]; !ok {
			t.Errorf("missing rx_bytes: %v", iface)
		}
	}
}

func TestParseIGMPSample(t *testing.T) {
	// live read — just assert the shape
	m := parseIGMPSnooping()
	for name, v := range m {
		if name == "" || (v != true && v != false) {
			t.Errorf("bad entry %q=%v", name, v)
		}
	}
}
