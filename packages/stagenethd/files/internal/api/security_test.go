package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stageneth/internal/auth"
)

func testSession(t *testing.T, user, role string) *http.Cookie {
	t.Helper()
	if sessions == nil {
		Init(auth.NewUsers(filepath.Join(t.TempDir(), "stagenethd")), auth.NewStore())
	}
	id, err := sessions.Create(user, role)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: auth.CookieName, Value: id}
}

func okHandler(w http.ResponseWriter, r *http.Request) { respond(w, 200, nil, "ok") }

func TestRequireAuth(t *testing.T) {
	testSession(t, "a", "tech")
	rr := httptest.NewRecorder()
	requireAuth(okHandler)(rr, httptest.NewRequest("GET", "/x", nil))
	if rr.Code != 401 {
		t.Errorf("unauthenticated got %d", rr.Code)
	}
	req := httptest.NewRequest("GET", "/x", nil)
	req.AddCookie(testSession(t, "a", "operator"))
	rr = httptest.NewRecorder()
	requireAuth(okHandler)(rr, req)
	if rr.Code != 200 {
		t.Errorf("authenticated got %d", rr.Code)
	}
}

func TestRequireTech(t *testing.T) {
	for _, tc := range []struct {
		role string
		want int
	}{
		{"operator", 403},
		{"tech", 200},
	} {
		req := httptest.NewRequest("GET", "/x", nil)
		req.AddCookie(testSession(t, "u-"+tc.role, tc.role))
		rr := httptest.NewRecorder()
		requireTech(okHandler)(rr, req)
		if rr.Code != tc.want {
			t.Errorf("role %s: got %d want %d", tc.role, rr.Code, tc.want)
		}
	}
}

func TestRawAllowlist(t *testing.T) {
	for _, cfg := range []string{"stageneth", "dropbear", "cron", "stagenethd", "../passwd"} {
		for _, h := range []http.HandlerFunc{rawGet, rawPut, rawDelete} {
			req := httptest.NewRequest("GET", "/api/v1/raw/"+cfg, nil)
			req.SetPathValue("config", cfg)
			rr := httptest.NewRecorder()
			h(rr, req)
			if rr.Code != 400 {
				t.Errorf("raw config %q got %d, want 400", cfg, rr.Code)
			}
		}
	}
}

func TestRawPutInjection(t *testing.T) {
	for _, body := range []string{
		`{"section":"x'evil","type":"iface","values":{"proto":"static"}}`,
		`{"section":"ok","type":"iface","values":{"ipaddr":"1.2.3.4\nset other.x=y"}}`,
		`{"section":"ok","type":"iface","values":{"bad name":"v"}}`,
		`{"section":"ok","type":"iface","values":{"desc":"a' delete network.lan"}}`,
		`{"section":"ok","type":"iface","values":{"desc":"a\"b"}}`,
	} {
		req := httptest.NewRequest("PUT", "/api/v1/raw/network", strings.NewReader(body))
		req.SetPathValue("config", "network")
		rr := httptest.NewRecorder()
		rawPut(rr, req)
		if rr.Code != 400 {
			t.Errorf("injection body accepted: %s -> %d", body, rr.Code)
		}
	}
}

func TestMetricsACL(t *testing.T) {
	for _, tc := range []struct {
		ip   string
		want int
	}{
		{"10.0.0.5", 200},
		{"192.168.1.1", 200},
		{"127.0.0.1", 200},
		{"8.8.8.8", 403},
		{"1.2.3.4", 403},
		{"not-an-ip", 403},
		{"", 403},
	} {
		req := httptest.NewRequest("GET", "/api/metrics", nil)
		req.Header.Set("X-Real-IP", tc.ip)
		rr := httptest.NewRecorder()
		metricsAllowed(okHandler)(rr, req)
		if rr.Code != tc.want {
			t.Errorf("ip %q: got %d want %d", tc.ip, rr.Code, tc.want)
		}
	}
}

func TestLoginLimiter(t *testing.T) {
	loginLimiter.Lock()
	loginLimiter.fails = map[string][]time.Time{}
	loginLimiter.Unlock()
	ip := "10.0.0.9"
	for i := 0; i < loginMaxFails; i++ {
		if loginThrottled(ip) {
			t.Fatalf("throttled after %d fails", i)
		}
		loginFailed(ip)
	}
	if !loginThrottled(ip) {
		t.Fatal("not throttled at limit")
	}
	// a different IP is unaffected
	if loginThrottled("10.0.0.10") {
		t.Fatal("unrelated IP throttled")
	}
	// stale entries are pruned
	loginLimiter.Lock()
	loginLimiter.fails[ip] = []time.Time{time.Now().Add(-loginFailWindow - time.Minute)}
	loginLimiter.Unlock()
	if loginThrottled(ip) {
		t.Fatal("stale fails still throttle")
	}
	loginLimiter.Lock()
	if _, exists := loginLimiter.fails[ip]; exists {
		t.Fatal("stale ip key not evicted")
	}
	loginLimiter.Unlock()
	// spoofed X-Forwarded-For must not defeat the limiter
	req := httptest.NewRequest("POST", "/api/v1/session", nil)
	req.Header.Set("X-Real-IP", "10.0.0.1")
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := clientIP(req); got != "10.0.0.1" {
		t.Fatalf("clientIP trusted spoofed XFF: %q", got)
	}
}

func TestParseUciShow(t *testing.T) {
	out := `network.loopback=interface
network.loopback.device='lo'
network.loopback.proto='static'
network.lan=interface
network.lan.ipaddr='192.168.1.1'
garbage line no equals`
	m := parseUciShow(out)
	lb, ok := m["loopback"].(map[string]interface{})
	if !ok {
		t.Fatal("loopback missing")
	}
	if lb[".type"] != "interface" || lb["proto"] != "static" {
		t.Errorf("loopback = %v", lb)
	}
	if m["lan"].(map[string]interface{})["ipaddr"] != "192.168.1.1" {
		t.Error("lan ipaddr wrong")
	}
}

func TestMdnsParseZone(t *testing.T) {
	// dns-sd -Z escapes spaces in instance names as \032
	zone := `; comment
Stage\032Mixer._dante._tcp.local SRV 0 0 5000 mixer.local.
Stage\032Mixer._dante._tcp.local TXT "version=1"
mixer.local. A 192.168.20.5
_services._dns-sd._udp.local. PTR Stage\032Mixer._dante._tcp.local.
`
	names, entries := mdnsParseZone([]byte(zone), "_dante._tcp", "local")
	if len(names) != 1 || names[0] != "Stage Mixer" {
		t.Fatalf("names = %v", names)
	}
	e := entries["Stage Mixer"]
	if e["port"] != 5000 || e["hostname"] != "mixer.local" || e["ip"] != "192.168.20.5" {
		t.Errorf("entry = %+v", e)
	}
	txt, _ := e["txt"].([]string)
	if len(txt) != 1 || txt[0] != "version=1" {
		t.Errorf("txt = %v", e["txt"])
	}
}

func TestEscLabel(t *testing.T) {
	if got := escLabel(`a"b\c`); got != `a\"b\\c` {
		t.Errorf("escLabel = %q", got)
	}
	if got := escLabel("x\ny"); got != `x\ny` {
		t.Errorf("escLabel newline = %q", got)
	}
}

func TestParseLogTime(t *testing.T) {
	if _, ok := parseLogTime("Dec  5 14:22:01 router log: something"); !ok {
		t.Error("syslog ts not parsed")
	}
	if _, ok := parseLogTime(""); ok {
		t.Error("empty line parsed")
	}
}

// Fuzz parsers that consume untrusted-ish input — uci show output,
// mDNS zone text (network-derived), syslog lines. Panic-freedom is the
// property under test.
func FuzzParseUciShow(f *testing.F) {
	f.Add("network.lan=interface\nnetwork.lan.ipaddr='192.168.1.1'\n")
	f.Add("a.b.c='x'\n\nweird==line\nx.y")
	f.Fuzz(func(t *testing.T, in string) {
		parseUciShow(in)
	})
}

func FuzzMdnsParseZone(f *testing.F) {
	f.Add("Name._dante._tcp.local SRV 0 0 5000 host.local.\nhost.local. A 10.0.0.1\n")
	f.Fuzz(func(t *testing.T, in string) {
		mdnsParseZone([]byte(in), "_dante._tcp", "local")
	})
}
