package api

import (
	http "net/http"
)

// Register installs all API routes on mux.
//
// Everything lives under /api/v1 except /api/metrics (Prometheus scraper
// convention), which also has a /api/v1/metrics alias.
//
// Public: session lifecycle, firstboot status, wizard.
// operator role: all read endpoints + apply/preview + own password.
// tech role: everything else (config mutation, restore, probes, user admin,
// raw UCI editing on the allowlisted configs).
func Register(mux *http.ServeMux) {
	// Prometheus scrape endpoint (unversioned by convention),
	// IP-allowlisted since it carries no session.
	mux.HandleFunc("GET /api/metrics", metricsAllowed(metricsPrometheus))

	// liveness probe
	mux.HandleFunc("GET /api/v1/health", health)

	// session lifecycle (self-guarded by method)
	mux.HandleFunc("/api/v1/session", sessionDispatch)
	mux.HandleFunc("GET /api/v1/firstboot", firstbootStatus)
	mux.HandleFunc("POST /api/v1/wizard", wizard)
	mux.HandleFunc("POST /api/v1/wizard-skip", wizardSkip)

	// any authenticated session
	mux.HandleFunc("POST /api/v1/password", requireAuth(changePassword))

	// operator + tech: read endpoints, apply, preview, events
	mux.HandleFunc("GET /api/v1/monitoring", requireAuth(monitoringSummary))
	mux.HandleFunc("GET /api/v1/metrics", requireAuth(metricsPrometheus))
	mux.HandleFunc("GET /api/v1/alerts", requireAuth(alertsList))
	mux.HandleFunc("GET /api/v1/logs", requireAuth(logsGet))
	mux.HandleFunc("GET /api/v1/interfaces", requireAuth(networkInterfaces))
	mux.HandleFunc("GET /api/v1/ntp", requireAuth(ntpGet))
	mux.HandleFunc("GET /api/v1/time", requireAuth(timeGet))
	mux.HandleFunc("GET /api/v1/ping", requireAuth(pingFromRouter))
	mux.HandleFunc("GET /api/v1/presets", requireAuth(presetsList))
	mux.HandleFunc("POST /api/v1/apply", requireAuth(applyNetwork))
	mux.HandleFunc("GET /api/v1/apply/preview", requireAuth(previewNetwork))
	mux.HandleFunc("GET /api/v1/state", requireAuth(v1State))
	mux.HandleFunc("GET /api/v1/events", requireAuth(v1Events))

	// tech only: mutations and privileged reads
	mux.HandleFunc("/api/v1/users", requireTech(usersDispatch))
	mux.HandleFunc("PUT /api/v1/globals", requireTech(v1SetGlobals))
	mux.HandleFunc("PUT /api/v1/services/{name}", requireTech(v1UpsertSection("service", v1ValidateService)))
	mux.HandleFunc("DELETE /api/v1/services/{name}", requireTech(v1DeleteSection("service")))
	mux.HandleFunc("PUT /api/v1/bindings/{name}", requireTech(v1UpsertSection("binding", v1ValidateBinding)))
	mux.HandleFunc("DELETE /api/v1/bindings/{name}", requireTech(v1DeleteSection("binding")))
	mux.HandleFunc("PUT /api/v1/forwardings/{name}", requireTech(v1UpsertSection("forwarding", v1ValidateForwarding)))
	mux.HandleFunc("DELETE /api/v1/forwardings/{name}", requireTech(v1DeleteSection("forwarding")))
	mux.HandleFunc("PUT /api/v1/logging/{name}", requireTech(v1SetLogging))
	mux.HandleFunc("PUT /api/v1/ntp", requireTech(ntpSet))
	mux.HandleFunc("POST /api/v1/presets/{name}", requireTech(presetApply))
	mux.HandleFunc("POST /api/v1/network/reload", requireTech(networkReload))
	mux.HandleFunc("POST /api/v1/snmp/walk", requireTech(snmpWalk))
	mux.HandleFunc("POST /api/v1/mdns/discover", requireTech(mdnsDiscover))
	// firstboot is one-way: the wizard refuses once any account exists,
	// so a "reset" flag would only confuse. Preset re-application lives
	// at POST /api/v1/presets/{name}/apply.
	mux.HandleFunc("GET /api/v1/backup", requireTech(backupConfig))
	mux.HandleFunc("POST /api/v1/restore", requireTech(restoreConfig))

	// expert-mode raw UCI editing, allowlisted configs only
	mux.HandleFunc("GET /api/v1/raw/{config}", requireTech(rawGet))
	mux.HandleFunc("PUT /api/v1/raw/{config}", requireTech(rawPut))
	mux.HandleFunc("DELETE /api/v1/raw/{config}", requireTech(rawDelete))
}

func usersDispatch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		usersList(w, r)
	case http.MethodPost, http.MethodPut:
		usersSet(w, r)
	case http.MethodDelete:
		usersDelete(w, r)
	default:
		respond(w, 405, nil, "method not allowed")
	}
}
