package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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
