package main

import (
	fmt "fmt"
	log "log"
	http "net/http"
	os "os"
)

func main() {
	secret = os.Getenv("STAGENETH_SECRET")
	if secret == "" {
		secret = "change-me"
	}
	bind := os.Getenv("STAGENETH_BIND")
	if bind == "" {
		bind = "127.0.0.1"
	}
	port := os.Getenv("STAGENETH_PORT")
	if port == "" {
		port = "8090"
	}

	http.HandleFunc("/api/login", login)
	http.HandleFunc("/api/firstboot", firstbootStatus)
	http.HandleFunc("/api/wizard", wizard)
	http.HandleFunc("/api/wizard-skip", wizardSkip)
	http.HandleFunc("/api/change-password", auth(changePassword))
	http.HandleFunc("/api/ubus/call", auth(ubusCall))
	http.HandleFunc("/api/uci/get", auth(uciGet))
	http.HandleFunc("/api/uci/set", auth(uciSet))
	http.HandleFunc("/api/uci/commit", auth(uciCommit))
	http.HandleFunc("/api/stageneth/apply", auth(applyNetwork))
	http.HandleFunc("/api/stageneth/presets", auth(presetsList))
	http.HandleFunc("/api/stageneth/preset-apply", auth(presetApply))
	http.HandleFunc("/api/network/reload", auth(networkReload))
	http.HandleFunc("/api/network/interfaces", auth(networkInterfaces))
	http.HandleFunc("/api/monitoring/summary", auth(monitoringSummary))
	http.HandleFunc("/api/metrics", auth(metricsPrometheus))
	http.HandleFunc("/api/snmp/walk", auth(snmpWalk))
	http.HandleFunc("/api/mdns/discover", auth(mdnsDiscover))
	http.HandleFunc("/api/ntp", auth(ntpGet))
	http.HandleFunc("/api/ntp/set", auth(ntpSet))
	http.HandleFunc("/api/time", auth(timeGet))
	http.HandleFunc("/api/logs", auth(logsGet))
	http.HandleFunc("/api/ping", auth(pingFromRouter))
	http.HandleFunc("/api/backup", auth(backupConfig))
	http.HandleFunc("/api/restore", auth(restoreConfig))

	addr := fmt.Sprintf("%s:%s", bind, port)
	fmt.Println("StageNeth API listening on", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
