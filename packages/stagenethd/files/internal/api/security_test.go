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
