package api

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

	// record names look like Name._type._proto.domain. — strip the
	// type+domain suffix and unescape \032 so the UI shows the bare
	// instance name, not the whole DNS record
	instanceName := func(rec string) string {
		n := strings.TrimSuffix(rec, ".")
		for _, suf := range []string{"." + serviceType + "." + domain, "." + serviceType, "." + domain} {
			if strings.HasSuffix(n, suf) {
				n = strings.TrimSuffix(n, suf)
				break
			}
		}
		return strings.Trim(strings.ReplaceAll(n, "\\032", " "), "\"")
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if m := srvRe.FindStringSubmatch(line); m != nil {
			name := instanceName(m[1])
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
			name := instanceName(m[1])
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
			name := instanceName(m[2])
			if entries[name] == nil {
				entries[name] = map[string]interface{}{"name": name, "type": serviceType, "domain": domain}
			}
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}

	// bound the per-hostname dns-sd resolves — each can take up to 2s and
	// a hostile mDNS advertiser controls the entry count
	resolved := 0
	for _, entry := range entries {
		if resolved >= 32 {
			break
		}
		if hostname, ok := entry["hostname"].(string); ok && hostname != "" {
			if ip, ok := ips[hostname]; ok {
				entry["ip"] = ip
			} else {
				resolved++
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

// dnsSd runs a bounded dns-sd query. busybox `timeout` kills the browse
// after `duration` seconds — no shell, so nothing from the wire or the
// request can reach a command line.
func dnsSd(duration int, args ...string) []byte {
	argv := append([]string{"timeout", strconv.Itoa(duration), "dns-sd"}, args...)
	out, _ := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	return out
}

var mdnsTokenRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func mdnsResolve(serviceType, domain string, duration int) (map[string]map[string]interface{}, []string) {
	if !mdnsTokenRe.MatchString(serviceType) || !mdnsTokenRe.MatchString(domain) {
		return nil, nil
	}
	out := dnsSd(duration, "-Z", serviceType, domain)
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

	out := dnsSd(req.Duration, "-B", req.Service, req.Domain)

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
		// cap the service-type sweep — every iteration is a timed
		// subprocess and the advertiser controls the type count
		if len(services) > 16 {
			services = services[:16]
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
	if req.Port < 1 || req.Port > 65535 {
		respond(w, 400, nil, "invalid port")
		return
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
	errWalkFull := fmt.Errorf("walk truncated at 2000 entries")
	err := g.Walk(req.OID, func(pdu gosnmp.SnmpPDU) error {
		if len(results) >= 2000 {
			return errWalkFull
		}
		results = append(results, map[string]interface{}{
			"oid":   pdu.Name,
			"type":  pdu.Type.String(),
			"value": fmt.Sprintf("%v", pdu.Value),
		})
		return nil
	})
	if err == errWalkFull {
		err = nil // truncation is not a failure
	}
	if err != nil {
		respond(w, 500, nil, "snmp walk failed: "+err.Error())
		return
	}
	respond(w, 200, results, "snmp walk success")
}
