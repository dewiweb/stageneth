package main

import (
	context "context"
	json "encoding/json"
	fmt "fmt"
	http "net/http"
	exec "os/exec"
	regexp "regexp"
	strconv "strconv"
	strings "strings"
	time "time"

	gosnmp "github.com/gosnmp/gosnmp"
)

func mdnsParseZone(out []byte, serviceType, domain string) ([]string, map[string]map[string]interface{}) {
	names := []string{}
	seen := map[string]bool{}
	entries := map[string]map[string]interface{}{}
	ips := map[string]string{}
	srvRe := regexp.MustCompile(`^([^\s]+)\s+SRV\s+\S+\s+\S+\s+(\d+)\s+(\S+)\.?`)
	txtRe := regexp.MustCompile(`^([^\s]+)\s+TXT\s+(.+)$`)
	ipRe := regexp.MustCompile(`^([^\s]+)\s+A\s+(\d+\.\d+\.\d+\.\d+)`)
	ptrRe := regexp.MustCompile(`^([^\s]+)\s+PTR\s+(\S+)`)

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if m := srvRe.FindStringSubmatch(line); m != nil {
			name := strings.Trim(strings.ReplaceAll(strings.TrimSuffix(strings.TrimSuffix(m[1], "."), "."+serviceType), "\\032", " "), "\"")
			if entries[name] == nil {
				entries[name] = map[string]interface{}{"name": name, "type": serviceType, "domain": domain}
			}
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
			entries[name]["hostname"] = strings.TrimSuffix(m[3], ".")
			if port, err := strconv.Atoi(m[2]); err == nil {
				entries[name]["port"] = port
			}
			continue
		}
		if m := txtRe.FindStringSubmatch(line); m != nil {
			name := strings.Trim(strings.ReplaceAll(strings.TrimSuffix(strings.TrimSuffix(m[1], "."), "."+serviceType), "\\032", " "), "\"")
			if entries[name] == nil {
				entries[name] = map[string]interface{}{"name": name, "type": serviceType, "domain": domain}
			}
			txt := []string{}
			for _, q := range regexp.MustCompile(`"([^"]*)"`).FindAllStringSubmatch(m[2], -1) {
				txt = append(txt, q[1])
			}
			entries[name]["txt"] = txt
			continue
		}
		if m := ipRe.FindStringSubmatch(line); m != nil {
			ips[strings.TrimSuffix(m[1], ".")] = m[2]
			continue
		}
		if m := ptrRe.FindStringSubmatch(line); m != nil {
			name := strings.Trim(strings.ReplaceAll(strings.TrimSuffix(strings.TrimSuffix(m[2], "."), "."+serviceType), "\\032", " "), "\"")
			if entries[name] == nil {
				entries[name] = map[string]interface{}{"name": name, "type": serviceType, "domain": domain}
			}
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}

	for _, entry := range entries {
		if hostname, ok := entry["hostname"].(string); ok && hostname != "" {
			if ip, ok := ips[hostname]; ok {
				entry["ip"] = ip
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				ipOut, _ := exec.CommandContext(ctx, "dns-sd", "-G", "v4", hostname+".").CombinedOutput()
				cancel()
				if m := regexp.MustCompile(`\b(\d+\.\d+\.\d+\.\d+)\b`).FindSubmatch(ipOut); m != nil {
					entry["ip"] = string(m[1])
				}
			}
		}
	}

	return names, entries
}
func mdnsResolve(serviceType, domain string, duration int) (map[string]map[string]interface{}, []string) {
	cmd := fmt.Sprintf("dns-sd -Z %s %s & PID=$!; sleep %d; kill $PID 2>/dev/null; wait $PID 2>/dev/null", serviceType, domain, duration)
	out, _ := exec.Command("sh", "-c", cmd).CombinedOutput()
	names, entries := mdnsParseZone(out, serviceType, domain)
	return entries, names
}
func mdnsDiscover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Service  string `json:"service"`
		Domain   string `json:"domain"`
		Duration int    `json:"duration"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		req.Service = ""
		req.Domain = ""
		req.Duration = 0
	}
	if req.Service == "" {
		req.Service = "_services._dns-sd._udp"
	}
	if req.Domain == "" {
		req.Domain = "local"
	}
	if req.Duration <= 0 || req.Duration > 10 {
		req.Duration = 3
	}
	if matched, _ := regexp.MatchString("^[A-Za-z0-9_.-]+$", req.Service); !matched {
		respond(w, 400, nil, "invalid service name")
		return
	}
	if matched, _ := regexp.MatchString("^[A-Za-z0-9_.-]+$", req.Domain); !matched {
		respond(w, 400, nil, "invalid domain")
		return
	}

	cmd := fmt.Sprintf("dns-sd -B %s %s & PID=$!; sleep %d; kill $PID 2>/dev/null; wait $PID 2>/dev/null", req.Service, req.Domain, req.Duration)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil && len(out) == 0 {
		respond(w, 500, nil, "mdns command failed: "+err.Error())
		return
	}

	if req.Service == "_services._dns-sd._udp" {
		seen := map[string]bool{}
		services := []string{}
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.Contains(line, " Add ") {
				continue
			}
			f := strings.Fields(line)
			if len(f) < 2 {
				continue
			}
			domainTok := f[len(f)-2]
			nameTok := f[len(f)-1]
			transport := strings.TrimSuffix(domainTok, ".local.")
			transport = strings.TrimSuffix(transport, ".local")
			if transport == "" || !strings.HasPrefix(transport, "_") {
				continue
			}
			serviceType := nameTok + "." + transport
			if serviceType == "_dns-sd._udp" || serviceType == "_services._dns-sd._udp" {
				continue
			}
			if !seen[serviceType] {
				seen[serviceType] = true
				services = append(services, serviceType)
			}
		}

		results := []map[string]interface{}{}
		resolveDuration := req.Duration
		if resolveDuration > 3 {
			resolveDuration = 2
		} else if resolveDuration > 1 {
			resolveDuration = 1
		}
		for _, serviceType := range services {
			entries, names := mdnsResolve(serviceType, req.Domain, resolveDuration)
			for _, name := range names {
				if e, ok := entries[name]; ok {
					if h, ok := e["hostname"].(string); ok && h != "" {
						results = append(results, e)
					}
				}
			}
		}
		respond(w, 200, results, "mdns discover success")
		return
	}

	names, entries := mdnsParseZone(out, req.Service, req.Domain)
	results := []map[string]interface{}{}
	for _, name := range names {
		if e, ok := entries[name]; ok {
			if h, ok := e["hostname"].(string); ok && h != "" {
				results = append(results, e)
			}
		}
	}
	respond(w, 200, results, "mdns discover success")
}
func parseAuthProtocol(p string) gosnmp.SnmpV3AuthProtocol {
	switch strings.ToUpper(p) {
	case "SHA":
		return gosnmp.SHA
	case "SHA224":
		return gosnmp.SHA224
	case "SHA256":
		return gosnmp.SHA256
	case "SHA384":
		return gosnmp.SHA384
	case "SHA512":
		return gosnmp.SHA512
	default:
		return gosnmp.MD5
	}
}
func parsePrivProtocol(p string) gosnmp.SnmpV3PrivProtocol {
	switch strings.ToUpper(p) {
	case "DES":
		return gosnmp.DES
	case "AES192":
		return gosnmp.AES192
	case "AES256":
		return gosnmp.AES256
	case "AES192C":
		return gosnmp.AES192C
	case "AES256C":
		return gosnmp.AES256C
	default:
		return gosnmp.AES
	}
}
func parseV3MsgFlags(level string) gosnmp.SnmpV3MsgFlags {
	var flags gosnmp.SnmpV3MsgFlags
	switch strings.ToLower(level) {
	case "authnopriv":
		flags = gosnmp.AuthNoPriv
	case "authpriv":
		flags = gosnmp.AuthPriv
	default:
		flags = gosnmp.NoAuthNoPriv
	}
	return flags | gosnmp.Reportable
}
func snmpWalk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host          string `json:"host"`
		Port          int    `json:"port"`
		Community     string `json:"community"`
		OID           string `json:"oid"`
		Version       string `json:"version"`
		Username      string `json:"username"`
		AuthProtocol  string `json:"authProtocol"`
		AuthPass      string `json:"authPass"`
		PrivProtocol  string `json:"privProtocol"`
		PrivPass      string `json:"privPass"`
		SecurityLevel string `json:"securityLevel"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Host == "" || req.OID == "" {
		respond(w, 400, nil, "invalid request: host and oid required")
		return
	}
	if req.Port == 0 {
		req.Port = 161
	}
	if req.Version == "" {
		req.Version = "v2c"
	}

	g := &gosnmp.GoSNMP{
		Target:             req.Host,
		Port:               uint16(req.Port),
		Timeout:            5 * time.Second,
		Retries:            1,
		ExponentialTimeout: false,
		MaxRepetitions:     10,
	}
	if req.Version == "v3" {
		if req.Username == "" {
			respond(w, 400, nil, "invalid request: username required for v3")
			return
		}
		if req.SecurityLevel == "" {
			req.SecurityLevel = "AuthNoPriv"
		}
		g.Version = gosnmp.Version3
		g.SecurityModel = gosnmp.UserSecurityModel
		g.MsgFlags = parseV3MsgFlags(req.SecurityLevel)
		g.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 req.Username,
			AuthenticationProtocol:   parseAuthProtocol(req.AuthProtocol),
			PrivacyProtocol:          parsePrivProtocol(req.PrivProtocol),
			AuthenticationPassphrase: req.AuthPass,
			PrivacyPassphrase:        req.PrivPass,
		}
	} else {
		if req.Community == "" {
			req.Community = "public"
		}
		g.Community = req.Community
		g.Version = gosnmp.Version2c
	}
	if err := g.Connect(); err != nil {
		respond(w, 500, nil, "snmp connect failed: "+err.Error())
		return
	}
	defer g.Conn.Close()

	results := []map[string]interface{}{}
	err := g.Walk(req.OID, func(pdu gosnmp.SnmpPDU) error {
		results = append(results, map[string]interface{}{
			"oid":   pdu.Name,
			"type":  pdu.Type.String(),
			"value": fmt.Sprintf("%v", pdu.Value),
		})
		return nil
	})
	if err != nil {
		respond(w, 500, nil, "snmp walk failed: "+err.Error())
		return
	}
	respond(w, 200, results, "snmp walk success")
}
