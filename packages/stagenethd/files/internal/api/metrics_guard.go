package api

import (
	http "net/http"
	netip "net/netip"
	strings "strings"

	"stageneth/internal/ucifile"
)

// /api/metrics is public (Prometheus scraper convention), so its exposure
// is bounded by an IP allowlist instead of a session. Default: loopback
// and private ranges only — suitable for a management LAN. Override with
// `option metrics_allow '<CIDR/IP> ...'` in the daemon config's main
// section; the option replaces the defaults entirely.
var metricsACL = []*netip.Prefix{
	mustPrefix("127.0.0.0/8"), mustPrefix("::1/128"),
	mustPrefix("10.0.0.0/8"), mustPrefix("172.16.0.0/12"),
	mustPrefix("192.168.0.0/16"), mustPrefix("169.254.0.0/16"),
	mustPrefix("fc00::/7"), mustPrefix("fe80::/10"),
}

var metricsCfgPath = ""

func mustPrefix(s string) *netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		panic("bad builtin prefix: " + s)
	}
	return &p
}

// InitMetrics reads the optional metrics_allow option from the daemon
// config and replaces the default allowlist.
func InitMetrics(daemonConfigPath string) {
	metricsCfgPath = daemonConfigPath
	f, err := ucifile.Read(daemonConfigPath)
	if err != nil {
		return // missing/corrupt config: keep defaults
	}
	for _, s := range f.Sections {
		if s.Type != "main" {
			continue
		}
		opt := s.Get("metrics_allow")
		if opt == "" {
			return
		}
		var acl []*netip.Prefix
		for _, tok := range strings.Fields(opt) {
			if p, err := netip.ParsePrefix(tok); err == nil {
				acl = append(acl, &p)
				continue
			}
			if a, err := netip.ParseAddr(tok); err == nil {
				p := netip.PrefixFrom(a, a.BitLen())
				acl = append(acl, &p)
			}
			// unparsable tokens are ignored rather than failing closed
		}
		if len(acl) > 0 {
			metricsACL = acl
		}
		return
	}
}

func metricsAllowed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		// RemoteAddr is empty on unix sockets; X-Real-IP (set by nginx) is
		// the only meaningful source here.
		if ip == "" {
			respond(w, 403, nil, "metrics not allowed from this address")
			return
		}
		addr, err := netip.ParseAddr(strings.Split(ip, "%")[0])
		if err != nil {
			respond(w, 403, nil, "metrics not allowed from this address")
			return
		}
		for _, p := range metricsACL {
			if p.Contains(addr) {
				next(w, r)
				return
			}
		}
		respond(w, 403, nil, "metrics not allowed from this address")
	}
}

// health is a public liveness probe: process up and mux reachable.
func health(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, map[string]interface{}{"ok": true}, "ok")
}
