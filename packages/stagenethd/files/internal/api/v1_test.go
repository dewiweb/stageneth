package api

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stageneth/internal/mon"
)

func v1Env(t *testing.T) string {
	dir := t.TempDir()
	old := StagenethConfig
	StagenethConfig = filepath.Join(dir, "stageneth")
	t.Cleanup(func() { StagenethConfig = old })
	os.WriteFile(StagenethConfig, []byte(`config stageneth 'globals'
	option enabled '1'
	option schema_version '2'
	option trunk 'eth1'

config service 'mgmt'
	option vlan_id '10'
`), 0644)
	return StagenethConfig
}

func TestV1ServiceUpsert(t *testing.T) {
	v1Env(t)
	h := v1UpsertSection("service", v1ValidateService)

	req := httptest.NewRequest("PUT", "/api/v1/services/dante", strings.NewReader(`{"vlan_id":"20","ipaddr":"10.20.0.1","netmask":"255.255.255.0","ptp":"1","multicast":"1"}`))
	req.SetPathValue("name", "dante")
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != 200 {
		t.Fatalf("upsert: %d %s", rr.Code, rr.Body.String())
	}
	b, _ := os.ReadFile(StagenethConfig)
	s := string(b)
	if !strings.Contains(s, "config service 'dante'") || !strings.Contains(s, "option vlan_id '20'") {
		t.Error("service not written:\n" + s)
	}
	// update existing
	req = httptest.NewRequest("PUT", "/api/v1/services/dante", strings.NewReader(`{"vlan_id":"21"}`))
	req.SetPathValue("name", "dante")
	rr = httptest.NewRecorder()
	h(rr, req)
	b, _ = os.ReadFile(StagenethConfig)
	if !strings.Contains(string(b), "option vlan_id '21'") || strings.Contains(string(b), "ptp") {
		t.Error("replace semantics failed:\n" + string(b))
	}
}

func TestV1ServiceValidation(t *testing.T) {
	v1Env(t)
	h := v1UpsertSection("service", v1ValidateService)
	cases := []string{
		`{"vlan_id":"99999"}`,          // out of range
		`{"vlan_id":"20","bogus":"1"}`, // unknown option
		`{"vlan_id":"20","ipaddr":"not-an-ip"}`,
		`{"vlan_id":"20","description":"bad\nline"}`,
	}
	for _, c := range cases {
		req := httptest.NewRequest("PUT", "/api/v1/services/x", strings.NewReader(c))
		req.SetPathValue("name", "x")
		rr := httptest.NewRecorder()
		h(rr, req)
		if rr.Code != 400 {
			t.Errorf("accepted %s -> %d", c, rr.Code)
		}
	}
	b, _ := os.ReadFile(StagenethConfig)
	if strings.Contains(string(b), "config service 'x'") {
		t.Error("invalid write persisted")
	}
}

func TestV1DeleteGuards(t *testing.T) {
	v1Env(t)
	// mgmt is protected
	req := httptest.NewRequest("DELETE", "/api/v1/services/mgmt", nil)
	req.SetPathValue("name", "mgmt")
	rr := httptest.NewRecorder()
	v1DeleteSection("service")(rr, req)
	if rr.Code != 400 {
		t.Error("mgmt deletion allowed")
	}
	// service with a binding cannot be deleted (dangling binding)
	req = httptest.NewRequest("PUT", "/api/v1/services/tmp", strings.NewReader(`{"vlan_id":"30"}`))
	req.SetPathValue("name", "tmp")
	v1UpsertSection("service", v1ValidateService)(httptest.NewRecorder(), req)
	req = httptest.NewRequest("PUT", "/api/v1/bindings/b1", strings.NewReader(`{"interface":"eth0","service":"tmp"}`))
	req.SetPathValue("name", "b1")
	v1UpsertSection("binding", v1ValidateBinding)(httptest.NewRecorder(), req)
	req = httptest.NewRequest("DELETE", "/api/v1/services/tmp", nil)
	req.SetPathValue("name", "tmp")
	rr = httptest.NewRecorder()
	v1DeleteSection("service")(rr, req)
	if rr.Code != 409 {
		t.Errorf("delete with bound service: got %d, want 409", rr.Code)
	}
	// delete the binding, then the service goes
	req = httptest.NewRequest("DELETE", "/api/v1/bindings/b1", nil)
	req.SetPathValue("name", "b1")
	v1DeleteSection("binding")(httptest.NewRecorder(), req)
	req = httptest.NewRequest("DELETE", "/api/v1/services/tmp", nil)
	req.SetPathValue("name", "tmp")
	rr = httptest.NewRecorder()
	v1DeleteSection("service")(rr, req)
	if rr.Code != 200 {
		t.Fatal("delete failed")
	}
	b, _ := os.ReadFile(StagenethConfig)
	if strings.Contains(string(b), "'tmp'") {
		t.Error("section not deleted")
	}
}

func TestV1ForwardingValidation(t *testing.T) {
	v1Env(t)
	h := v1UpsertSection("forwarding", v1ValidateForwarding)
	put := func(body string) int {
		req := httptest.NewRequest("PUT", "/api/v1/forwardings/f1", strings.NewReader(body))
		req.SetPathValue("name", "f1")
		rr := httptest.NewRecorder()
		h(rr, req)
		return rr.Code
	}
	if c := put(`{}`); c != 400 {
		t.Errorf("empty forwarding accepted: %d", c)
	}
	if c := put(`{"src":"mgmt","dest":"ghost"}`); c != 400 {
		t.Errorf("forwarding to unknown service accepted: %d", c)
	}
	if c := put(`{"src":"mgmt","dest":"mgmt","enabled":"1"}`); c != 200 {
		t.Errorf("valid forwarding rejected: %d", c)
	}
}

func TestV1GlobalsAndLogging(t *testing.T) {
	v1Env(t)
	req := httptest.NewRequest("PUT", "/api/v1/globals", strings.NewReader(`{"show_lock":"1","trunk":"eth2"}`))
	rr := httptest.NewRecorder()
	v1SetGlobals(rr, req)
	if rr.Code != 200 {
		t.Fatalf("globals: %d %s", rr.Code, rr.Body.String())
	}
	b, _ := os.ReadFile(StagenethConfig)
	if !strings.Contains(string(b), "option show_lock '1'") {
		t.Error("globals not merged:\n" + string(b))
	}
	// bad field rejected
	req = httptest.NewRequest("PUT", "/api/v1/globals", strings.NewReader(`{"schema_version":"9"}`))
	rr = httptest.NewRecorder()
	v1SetGlobals(rr, req)
	if rr.Code != 400 {
		t.Error("schema_version writable")
	}
	// logging validation
	req = httptest.NewRequest("PUT", "/api/v1/logging/remote", strings.NewReader(`{"enabled":"1","host":"10.0.0.5","port":"1514","protocol":"tcp"}`))
	req.SetPathValue("name", "remote")
	rr = httptest.NewRecorder()
	v1SetLogging(rr, req)
	if rr.Code != 200 {
		t.Fatalf("logging: %d %s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest("PUT", "/api/v1/logging/remote", strings.NewReader(`{"host":"a\"b"}`))
	req.SetPathValue("name", "remote")
	rr = httptest.NewRecorder()
	v1SetLogging(rr, req)
	if rr.Code != 400 {
		t.Error("host injection accepted")
	}
}

func TestV1BindingValidation(t *testing.T) {
	v1Env(t)
	h := v1UpsertSection("binding", v1ValidateBinding)
	// unknown service rejected
	req := httptest.NewRequest("PUT", "/api/v1/bindings/b1", strings.NewReader(`{"interface":"eth0","service":"ghost"}`))
	req.SetPathValue("name", "b1")
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != 400 {
		t.Error("binding to unknown service accepted")
	}
	// valid binding accepted
	req = httptest.NewRequest("PUT", "/api/v1/bindings/b1", strings.NewReader(`{"interface":"eth0","service":"mgmt"}`))
	req.SetPathValue("name", "b1")
	rr = httptest.NewRecorder()
	h(rr, req)
	if rr.Code != 200 {
		t.Fatalf("binding: %d %s", rr.Code, rr.Body.String())
	}
}

func TestV1EventsStreamsSnapshot(t *testing.T) {
	InitMonitor(mon.NewCollector(time.Hour, time.Hour))
	t.Cleanup(func() { InitMonitor(nil) })
	old := sseTick
	sseTick = 10 * time.Millisecond
	t.Cleanup(func() { sseTick = old })

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/v1/events", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { v1Events(rr, req); close(done) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done
	// regression: collected_at is int64 — a float64-only accessor left the
	// "changed" predicate permanently false and SSE never pushed a frame
	if !strings.Contains(rr.Body.String(), "data: ") {
		t.Fatalf("no SSE frame pushed: %q", rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %q", ct)
	}
}

func TestMapFloatNumericKinds(t *testing.T) {
	m := map[string]interface{}{"f": 1.5, "i64": int64(7), "u64": uint64(8), "i": 9}
	for k, want := range map[string]float64{"f": 1.5, "i64": 7, "u64": 8, "i": 9} {
		if got := mapFloat(m, k); got != want {
			t.Errorf("mapFloat(%s) = %v, want %v", k, got, want)
		}
	}
}

func TestRawPutCommands(t *testing.T) {
	cmds, err := rawPutCommands("dhcp", "lan", "", map[string]interface{}{
		"server": []interface{}{"8.8.8.8", "1.1.1.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmds, "\n")
	if !strings.Contains(joined, "delete dhcp.lan.server") ||
		!strings.Contains(joined, "add_list dhcp.lan.server='8.8.8.8'") ||
		!strings.Contains(joined, "add_list dhcp.lan.server='1.1.1.1'") {
		t.Fatalf("list commands wrong: %v", cmds)
	}
	if _, err := rawPutCommands("dhcp", "lan", "", map[string]interface{}{".type": "x"}); err == nil {
		t.Error("dot-prefixed option name accepted — produces 'set dhcp.lan..type='")
	}
	if _, err := rawPutCommands("dhcp", "lan", "", map[string]interface{}{"x": []interface{}{"a'b"}}); err == nil {
		t.Error("list value with quote accepted")
	}
	if _, err := rawPutCommands("dhcp", "lan", "@bad[0]", nil); err == nil {
		t.Error("anonymous-ref style accepted as section type")
	}
}

func TestParseUciShowLists(t *testing.T) {
	out := "dhcp.lan=dhcp\ndhcp.lan.server='8.8.8.8'\ndhcp.lan.server='1.1.1.1'\ndhcp.lan.leasetime='12h'\n"
	v := parseUciShow(out)
	lan := v["lan"].(map[string]interface{})
	list, ok := lan["server"].([]interface{})
	if !ok || len(list) != 2 || list[0] != "8.8.8.8" || list[1] != "1.1.1.1" {
		t.Fatalf("list not preserved: %v", lan["server"])
	}
	if lan["leasetime"] != "12h" {
		t.Fatalf("scalar corrupted: %v", lan["leasetime"])
	}
}
