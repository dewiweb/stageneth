// Package render computes the desired managed UCI sections for each
// OpenWrt config from the StageNeth source of truth. All functions are
// pure: same input, same output, no side effects.
package render

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"stageneth/internal/stcfg"
	"stageneth/internal/ucifile"
)

func opt(name, value string) ucifile.Option {
	return ucifile.Option{Name: name, Value: value}
}

func list(name, value string) ucifile.Option {
	return ucifile.Option{Name: name, Value: value, List: true}
}

func sec(typ, name string, opts ...ucifile.Option) ucifile.Section {
	return ucifile.Section{Type: typ, Name: name, Options: opts}
}

// AvailableInterfaces returns the kernel interface names from sysfs.
func AvailableInterfaces() map[string]bool {
	res := map[string]bool{}
	entries, _ := os.ReadDir("/sys/class/net")
	for _, e := range entries {
		info, err := os.Stat(filepath.Join("/sys/class/net", e.Name()))
		if err == nil && info.IsDir() {
			res[e.Name()] = true
		}
	}
	return res
}

func fallbackIface(iface string, avail map[string]bool) string {
	if avail[iface] {
		return iface
	}
	for _, cand := range []string{"eth1", "eth0", "eth2"} {
		if avail[cand] {
			return cand
		}
	}
	return "eth0"
}

func validVlanID(s string) string {
	id, err := strconv.Atoi(s)
	if err != nil || id < 1 || id > 4094 {
		return "1"
	}
	return strconv.Itoa(id)
}

// Trunk returns the configured trunk interface with fallback applied.
func Trunk(st *stcfg.Config, avail map[string]bool) string {
	trunk := st.Globals["trunk"]
	if trunk == "" {
		trunk = "eth1"
	}
	return trunk
}

// SwitchVendor returns the configured switch vendor profile.
func SwitchVendor(st *stcfg.Config) string {
	v := st.Globals["switch_vendor"]
	if v == "" {
		v = "generic"
	}
	return v
}

// servicePorts computes the bridge port list for a service.
func servicePorts(st *stcfg.Config, svcName string, svc map[string]string, trunk string, avail map[string]bool) []string {
	vlanID := validVlanID(svc["vlan_id"])
	ports := []string{}
	seen := map[string]bool{}
	// dedupe: two bindings on the same iface+service must not emit
	// duplicate ports (or duplicate device sections downstream)
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			ports = append(ports, p)
		}
	}
	for _, bcfg := range st.SortedBindings() {
		if bcfg["service"] == svcName {
			iface := bcfg["interface"]
			if iface == "" {
				iface = trunk
			}
			iface = fallbackIface(iface, avail)
			if svc["untagged"] == "1" {
				add(iface)
			} else {
				add(iface + "." + vlanID)
			}
		}
	}
	if len(ports) == 0 {
		iface := fallbackIface(trunk, avail)
		if svc["untagged"] == "1" {
			ports = []string{iface}
		} else {
			ports = []string{iface + "." + vlanID}
		}
	}
	return ports
}

// Network renders managed sections for /etc/config/network.
func Network(st *stcfg.Config, avail map[string]bool) []ucifile.Section {
	trunk := Trunk(st, avail)
	vendor := SwitchVendor(st)
	var out []ucifile.Section
	for _, svcName := range st.ServiceNames() {
		svc := st.Services[svcName]
		vlanID := validVlanID(svc["vlan_id"])
		ifname := "svc_" + svcName
		bridge := "br_" + svcName
		ports := servicePorts(st, svcName, svc, trunk, avail)

		ipaddr := svc["ipaddr"]
		if ipaddr == "" {
			ipaddr = fmt.Sprintf("10.%s.0.1", vlanID)
		}
		netmask := svc["netmask"]
		if netmask == "" {
			netmask = "255.255.255.0"
		}
		mtu := svc["mtu"]
		if mtu == "" {
			mtu = "1500"
			if svcName == "st2110" {
				mtu = "9000"
			}
		}

		ifOpts := []ucifile.Option{
			opt("device", bridge),
			opt("proto", "static"),
			opt("ipaddr", ipaddr),
			opt("netmask", netmask),
		}
		if mtu != "1500" {
			ifOpts = append(ifOpts, opt("mtu", mtu))
		}
		out = append(out, sec("interface", ifname, ifOpts...))

		for _, port := range ports {
			if !strings.Contains(port, ".") {
				continue
			}
			vlanDev := strings.ReplaceAll(port, ".", "_")
			parts := strings.SplitN(port, ".", 2)
			out = append(out, sec("device", vlanDev,
				opt("name", port),
				opt("type", "8021q"),
				opt("vid", parts[1]),
				opt("ifname", parts[0]),
				opt("mtu", mtu),
			))
		}

		brOpts := []ucifile.Option{
			opt("name", bridge),
			opt("type", "bridge"),
			opt("ports", strings.Join(ports, " ")),
			opt("mtu", mtu),
		}
		if svc["multicast"] == "1" {
			brOpts = append(brOpts, opt("igmp_snooping", "1"))
			if vendor == "generic" {
				brOpts = append(brOpts, opt("multicast_querier", "1"))
			}
		}
		out = append(out, sec("device", bridge, brOpts...))
	}
	return out
}

// CollectorInfo reports whether the syslog collector is enabled and its settings.
func CollectorInfo(st *stcfg.Config) (enabled bool, port, proto, dir string) {
	collector := st.Logging["collector"]
	enabled = collector != nil && collector["enabled"] == "1"
	port, proto, dir = "514", "udp", "/var/log/remote"
	if enabled {
		if p := collector["port"]; validSyslogPort(p) {
			port = p
		}
		if collector["protocol"] == "tcp" || collector["protocol"] == "both" {
			proto = collector["protocol"]
		}
		if d := collector["dir"]; d != "" && syslogDirRe.MatchString(d) && !strings.Contains(d, "..") {
			dir = d
		}
	}
	return enabled, port, proto, dir
}

// Firewall renders managed sections for /etc/config/firewall.
func Firewall(st *stcfg.Config) []ucifile.Section {
	var out []ucifile.Section
	colEnabled, colPort, colProto, _ := CollectorInfo(st)
	for _, svcName := range st.ServiceNames() {
		zone := "zone_" + svcName
		ifname := "svc_" + svcName
		input := "REJECT"
		if svcName == "mgmt" {
			input = "ACCEPT"
		}
		out = append(out, sec("zone", zone,
			opt("name", svcName),
			opt("network", ifname),
			opt("input", input),
			opt("output", "ACCEPT"),
			opt("forward", "REJECT"),
		))
		for _, kind := range []string{"dhcp", "dns", "ntp"} {
			var port, proto string
			switch kind {
			case "dhcp":
				port, proto = "67", "udp"
			case "dns":
				port, proto = "53", "tcp udp"
			case "ntp":
				port, proto = "123", "udp"
			}
			rule := fmt.Sprintf("allow_%s_%s", kind, svcName)
			out = append(out, sec("rule", rule,
				opt("name", fmt.Sprintf("Allow-%s-%s", strings.ToUpper(kind), svcName)),
				opt("src", svcName),
				opt("proto", proto),
				opt("dest_port", port),
				opt("target", "ACCEPT"),
			))
		}
		if colEnabled && svcName != "mgmt" {
			proto := "udp"
			if colProto == "tcp" {
				proto = "tcp"
			} else if colProto == "both" {
				proto = "tcp udp"
			}
			rule := "allow_syslog_" + svcName
			out = append(out, sec("rule", rule,
				opt("name", fmt.Sprintf("Allow-SYSLOG-%s", svcName)),
				opt("src", svcName),
				opt("proto", proto),
				opt("dest_port", colPort),
				opt("target", "ACCEPT"),
			))
		}
	}

	fwdNames := make([]string, 0, len(st.Forwardings))
	for n := range st.Forwardings {
		fwdNames = append(fwdNames, n)
	}
	sort.Strings(fwdNames)
	for _, name := range fwdNames {
		fcfg := st.Forwardings[name]
		// disabled forwardings are omitted entirely rather than emitted
		// with option enabled '0' — fw4's forwarding schema does not
		// reliably honor it, and an absent section is unambiguous
		if fcfg["enabled"] != "1" {
			continue
		}
		out = append(out, sec("forwarding", name,
			opt("src", fcfg["src"]),
			opt("dest", fcfg["dest"]),
		))
	}
	for _, s := range st.ServiceNames() {
		if s == "mgmt" || s == "guest" {
			continue
		}
		fwd := "mgmt_to_" + s
		out = append(out, sec("forwarding", fwd,
			opt("src", "mgmt"),
			opt("dest", s),
		))
	}
	return out
}

// Dhcp renders managed sections for /etc/config/dhcp.
func Dhcp(st *stcfg.Config) []ucifile.Section {
	var out []ucifile.Section
	for _, svcName := range st.ServiceNames() {
		svc := st.Services[svcName]
		ifname := "svc_" + svcName
		ipaddr := svc["ipaddr"]
		if ipaddr == "" {
			ipaddr = fmt.Sprintf("10.%s.0.1", validVlanID(svc["vlan_id"]))
		}
		out = append(out, sec("dhcp", ifname,
			opt("interface", ifname),
			opt("start", "101"),
			opt("limit", "154"),
			opt("leasetime", "12h"),
			list("dhcp_option", "42,"+ipaddr),
			list("interface_name", ifname),
		))
	}
	return out
}

var syslogHostRe = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
var syslogDirRe = regexp.MustCompile(`^[A-Za-z0-9_/.-]+$`)

func validSyslogPort(s string) bool {
	p, err := strconv.Atoi(s)
	return err == nil && p >= 1 && p <= 65535
}

// Rsyslog renders managed sections for /etc/config/rsyslog
// (forwarder and collector inputs via the rsyslog UCI schema).
func Rsyslog(st *stcfg.Config) []ucifile.Section {
	var out []ucifile.Section

	remote := st.Logging["remote"]
	if remote != nil && remote["enabled"] == "1" && remote["host"] != "" &&
		syslogHostRe.MatchString(remote["host"]) {
		port := remote["port"]
		if !validSyslogPort(port) {
			port = "514"
		}
		proto := remote["protocol"]
		if proto != "udp" && proto != "tcp" {
			proto = "udp"
		}
		out = append(out, sec("forwarder", "stageneth_remote",
			opt("source", "*.*"),
			opt("target", remote["host"]),
			opt("port", port),
			opt("protocol", proto),
		))
	}

	colEnabled, colPort, colProto, _ := CollectorInfo(st)
	if colEnabled {
		opts := []ucifile.Option{}
		if colProto != "tcp" {
			opts = append(opts, opt("udp_input", "1"), opt("udp_input_port", colPort))
		}
		if colProto != "udp" {
			opts = append(opts, opt("tcp_input", "1"), opt("tcp_input_port", colPort))
		}
		out = append(out, sec("rsyslog", "syslog", opts...))
	}
	return out
}

// CollectorBlock returns the managed block for /etc/rsyslog.conf writing
// remote logs to dir/<hostname>.log, or "" when the collector is disabled.
func CollectorBlock(st *stcfg.Config) string {
	enabled, _, _, dir := CollectorInfo(st)
	if !enabled {
		return ""
	}
	return fmt.Sprintf(`template(name="stagenethRemoteFile" type="string" string="%s/%%HOSTNAME%%.log")
if ($fromhost-ip != "127.0.0.1" and $fromhost-ip != "::1") then {
	action(type="omfile" dynaFile="stagenethRemoteFile")
	stop
}`, dir)
}

// Umdns renders the full /etc/config/umdns file.
func Umdns(st *stcfg.Config) []ucifile.Section {
	// jail '1' keeps stock OpenWrt sandboxing — a fullOwner render must
	// not silently drop the packaged hardening
	opts := []ucifile.Option{opt("enabled", "1"), opt("jail", "1")}
	for _, name := range st.ServiceNames() {
		if name == "guest" {
			continue
		}
		opts = append(opts, list("network", "svc_"+name))
	}
	return []ucifile.Section{sec("umdns", "umdns", opts...)}
}

// MdnsRepeater renders the full /etc/config/mdns_repeater file.
func MdnsRepeater(st *stcfg.Config) []ucifile.Section {
	opts := []ucifile.Option{opt("enabled", "1")}
	for _, name := range st.ServiceNames() {
		if name == "guest" {
			continue
		}
		opts = append(opts, list("interface", "br_"+name))
	}
	return []ucifile.Section{sec("mdns_repeater", "main", opts...)}
}
