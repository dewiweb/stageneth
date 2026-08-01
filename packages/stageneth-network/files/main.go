package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type UciData struct {
	Sections map[string]string
	Values   map[string]map[string]string
}

func uciShow(config string) UciData {
	data := UciData{Sections: map[string]string{}, Values: map[string]map[string]string{}}
	out, _ := exec.Command("uci", "-q", "show", config).Output()
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := parts[0]
		value := strings.Trim(parts[1], "'\"")
		kp := strings.Split(key, ".")
		if len(kp) == 2 {
			data.Sections[kp[1]] = value
		} else if len(kp) == 3 {
			section := kp[1]
			option := kp[2]
			if data.Values[section] == nil {
				data.Values[section] = map[string]string{}
			}
			data.Values[section][option] = value
		}
	}
	return data
}

func availableInterfaces() map[string]bool {
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

func qset(section, option, value string) string {
	return fmt.Sprintf("set %s.%s=%s", section, option, value)
}

func addList(section, option, value string) string {
	return fmt.Sprintf("add_list %s.%s=%s", section, option, value)
}

func del(section, option string) string {
	if option == "" {
		return fmt.Sprintf("delete %s", section)
	}
	return fmt.Sprintf("delete %s.%s", section, option)
}

func generateNetwork(svcName string, svc map[string]string, bindings UciData, trunk string, switchVendor string, avail map[string]bool) []string {
	vlanID := validVlanID(svc["vlan_id"])
	ifname := "svc_" + svcName
	bridge := "br_" + svcName
	ports := []string{}
	for _, bcfg := range bindings.Values {
		if bcfg["service"] == svcName {
			iface := bcfg["interface"]
			if iface == "" {
				iface = trunk
			}
			iface = fallbackIface(iface, avail)
			if svc["untagged"] == "1" {
				ports = append(ports, iface)
			} else {
				ports = append(ports, iface+"."+vlanID)
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

	var cmds []string
	cmds = append(cmds, fmt.Sprintf("set network.%s=interface", ifname))
	cmds = append(cmds, qset("network."+ifname, "device", bridge))
	cmds = append(cmds, qset("network."+ifname, "proto", "static"))
	cmds = append(cmds, qset("network."+ifname, "ipaddr", ipaddr))
	cmds = append(cmds, qset("network."+ifname, "netmask", netmask))
	if mtu != "1500" {
		cmds = append(cmds, qset("network."+ifname, "mtu", mtu))
	}
	for _, port := range ports {
		if strings.Contains(port, ".") {
			vlanDev := strings.ReplaceAll(port, ".", "_")
			parts := strings.SplitN(port, ".", 2)
			parent := parts[0]
			vid := parts[1]
			cmds = append(cmds, fmt.Sprintf("set network.%s=device", vlanDev))
			cmds = append(cmds, qset("network."+vlanDev, "name", port))
			cmds = append(cmds, qset("network."+vlanDev, "type", "8021q"))
			cmds = append(cmds, qset("network."+vlanDev, "vid", vid))
			cmds = append(cmds, qset("network."+vlanDev, "ifname", parent))
			cmds = append(cmds, qset("network."+vlanDev, "mtu", mtu))
		}
	}
	cmds = append(cmds, fmt.Sprintf("set network.%s=device", bridge))
	cmds = append(cmds, qset("network."+bridge, "name", bridge))
	cmds = append(cmds, qset("network."+bridge, "type", "bridge"))
	cmds = append(cmds, qset("network."+bridge, "ports", strings.Join(ports, " ")))
	cmds = append(cmds, qset("network."+bridge, "mtu", mtu))
	if svc["multicast"] == "1" {
		cmds = append(cmds, qset("network."+bridge, "igmp_snooping", "1"))
		if switchVendor == "generic" {
			cmds = append(cmds, qset("network."+bridge, "multicast_querier", "1"))
		}
	}
	return cmds
}

func generateFirewall(svcName string, svc map[string]string) []string {
	zone := "zone_" + svcName
	ifname := "svc_" + svcName
	input := "REJECT"
	if svcName == "mgmt" {
		input = "ACCEPT"
	}
	cmds := []string{
		fmt.Sprintf("set firewall.%s=zone", zone),
		qset("firewall."+zone, "name", svcName),
		qset("firewall."+zone, "network", ifname),
		qset("firewall."+zone, "input", input),
		qset("firewall."+zone, "output", "ACCEPT"),
		qset("firewall."+zone, "forward", "REJECT"),
	}
	for _, kind := range []string{"dhcp", "dns", "ntp"} {
		var port string
		switch kind {
		case "dhcp":
			port = "67"
		case "dns":
			port = "53"
		case "ntp":
			port = "123"
		}
		proto := "udp"
		if kind == "dns" {
			proto = "tcp udp"
		}
		rule := fmt.Sprintf("allow_%s_%s", kind, svcName)
		cmds = append(cmds, fmt.Sprintf("set firewall.%s=rule", rule))
		cmds = append(cmds, qset("firewall."+rule, "name", fmt.Sprintf("Allow-%s-%s", strings.ToUpper(kind), svcName)))
		cmds = append(cmds, qset("firewall."+rule, "src", svcName))
		cmds = append(cmds, qset("firewall."+rule, "proto", proto))
		cmds = append(cmds, qset("firewall."+rule, "dest_port", port))
		cmds = append(cmds, qset("firewall."+rule, "target", "ACCEPT"))
	}
	return cmds
}

func generateForwardings(forwardings UciData) []string {
	var cmds []string
	for name, fcfg := range forwardings.Values {
		cmds = append(cmds, fmt.Sprintf("set firewall.%s=forwarding", name))
		cmds = append(cmds, qset("firewall."+name, "src", fcfg["src"]))
		cmds = append(cmds, qset("firewall."+name, "dest", fcfg["dest"]))
		cmds = append(cmds, qset("firewall."+name, "enabled", fcfg["enabled"]))
	}
	return cmds
}

func generateDhcp(svcName string, svc map[string]string) []string {
	ifname := "svc_" + svcName
	ipaddr := svc["ipaddr"]
	if ipaddr == "" {
		vlanID := validVlanID(svc["vlan_id"])
		ipaddr = fmt.Sprintf("10.%s.0.1", vlanID)
	}
	return []string{
		fmt.Sprintf("set dhcp.%s=dhcp", ifname),
		qset("dhcp."+ifname, "interface", ifname),
		qset("dhcp."+ifname, "start", "101"),
		qset("dhcp."+ifname, "limit", "154"),
		qset("dhcp."+ifname, "leasetime", "12h"),
		addList("dhcp."+ifname, "dhcp_option", "42,"+ipaddr),
		del("dhcp."+ifname, "interface_name"),
		addList("dhcp."+ifname, "interface_name", ifname),
	}
}

func generateQos(svcName string, svc map[string]string) []string {
	dscp := svc["dscp"]
	if dscp == "" {
		dscp = "0"
	}
	priority := svc["priority"]
	if priority == "" {
		priority = "0"
	}
	return []string{
		qset("stageneth."+svcName, "dscp", dscp),
		qset("stageneth."+svcName, "priority", priority),
	}
}

func tuneNics() {
	avail := availableInterfaces()
	for iface := range avail {
		if strings.HasPrefix(iface, "lo") ||
			strings.HasPrefix(iface, "br-") ||
			strings.HasPrefix(iface, "bond") ||
			strings.HasPrefix(iface, "docker") ||
			strings.HasPrefix(iface, "veth") ||
			strings.HasPrefix(iface, "wg") ||
			strings.HasPrefix(iface, "tun") ||
			strings.Contains(iface, ".") {
			continue
		}
		exec.Command("ethtool", "--set-eee", iface, "eee", "disabled").Run()
		exec.Command("ethtool", "-G", iface, "rx", "4096", "tx", "4096").Run()
		exec.Command("ethtool", "-C", iface, "rx-usecs", "0", "tx-usecs", "0").Run()
		exec.Command("ethtool", "-K", iface, "gro", "off", "tso", "off", "lro", "off", "gso", "off").Run()
	}
}

func generateMdnsRepeater(services map[string]map[string]string) []string {
	cmds := []string{
		"set mdns_repeater.main=mdns_repeater",
		qset("mdns_repeater.main", "enabled", "1"),
		del("mdns_repeater.main", "interface"),
	}
	var names []string
	for n := range services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "guest" {
			continue
		}
		cmds = append(cmds, addList("mdns_repeater.main", "interface", "br_"+name))
	}
	return cmds
}

func generateUmdns(services map[string]map[string]string) []string {
	cmds := []string{
		"set umdns.umdns=umdns",
		qset("umdns.umdns", "enabled", "1"),
		del("umdns.umdns", "network"),
	}
	var names []string
	for n := range services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "guest" {
			continue
		}
		cmds = append(cmds, addList("umdns.umdns", "network", "svc_"+name))
	}
	return cmds
}

func batchCommands(cmds []string) {
	for _, cmd := range cmds {
		parts := strings.SplitN(cmd, " ", 2)
		action := parts[0]
		var arg string
		if len(parts) > 1 {
			arg = parts[1]
		}
		c := exec.Command("uci", "-q", action, arg)
		if err := c.Run(); err != nil && action != "delete" {
			fmt.Fprintf(os.Stderr, "uci command failed: %s (%v)\n", cmd, err)
			os.Exit(1)
		}
	}
}

func run(name string, args ...string) {
	exec.Command(name, args...).Run()
}

func apply() {
	st := uciShow("stageneth")
	services := map[string]map[string]string{}
	for s, t := range st.Sections {
		if t == "service" {
			if v, ok := st.Values[s]; ok {
				services[s] = v
			} else {
				services[s] = map[string]string{}
			}
		}
	}
	bindings := UciData{Values: map[string]map[string]string{}}
	for s, t := range st.Sections {
		if t == "binding" {
			if v, ok := st.Values[s]; ok {
				bindings.Values[s] = v
			}
		}
	}
	forwardings := UciData{Values: map[string]map[string]string{}}
	for s, t := range st.Sections {
		if t == "forwarding" {
			if v, ok := st.Values[s]; ok {
				forwardings.Values[s] = v
			}
		}
	}

	serviceNames := []string{}
	for n := range services {
		serviceNames = append(serviceNames, n)
	}
	sort.Strings(serviceNames)
	forwardingNames := map[string]bool{}
	for n := range forwardings.Values {
		forwardingNames[n] = true
	}

	var commands []string

	cleanup := []struct {
		cfg    string
		prefix string
	}{
		{"network", "svc_"},
		{"network", "br_"},
		{"dhcp", "svc_"},
		{"firewall", "zone_"},
	}
	for _, c := range cleanup {
		existing := uciShow(c.cfg)
		for section := range existing.Sections {
			if strings.HasPrefix(section, c.prefix) {
				svc := strings.TrimPrefix(section, c.prefix)
				if _, ok := services[svc]; !ok {
					commands = append(commands, del(c.cfg+"."+section, ""))
				}
			}
		}
	}
	existingFw := uciShow("firewall")
	for section := range existingFw.Sections {
		if strings.Contains(section, "_to_") && !forwardingNames[section] {
			commands = append(commands, del("firewall."+section, ""))
		}
		for _, prefix := range []string{"allow_dhcp_", "allow_dns_", "allow_ntp_"} {
			if strings.HasPrefix(section, prefix) {
				svc := strings.TrimPrefix(section, prefix)
				if _, ok := services[svc]; !ok {
					commands = append(commands, del("firewall."+section, ""))
				}
			}
		}
	}

	trunk := st.Values["globals"]["trunk"]
	if trunk == "" {
		trunk = "eth1"
	}
	switchVendor := st.Values["globals"]["switch_vendor"]
	if switchVendor == "" {
		switchVendor = "generic"
	}
	avail := availableInterfaces()
	for _, svcName := range serviceNames {
		svc := services[svcName]
		commands = append(commands, generateNetwork(svcName, svc, bindings, trunk, switchVendor, avail)...)
		commands = append(commands, generateFirewall(svcName, svc)...)
		commands = append(commands, generateDhcp(svcName, svc)...)
		commands = append(commands, generateQos(svcName, svc)...)
	}

	commands = append(commands, generateForwardings(forwardings)...)
	for _, s := range serviceNames {
		if s == "mgmt" || s == "guest" {
			continue
		}
		fwd := "mgmt_to_" + s
		commands = append(commands, fmt.Sprintf("set firewall.%s=forwarding", fwd))
		commands = append(commands, qset("firewall."+fwd, "src", "mgmt"))
		commands = append(commands, qset("firewall."+fwd, "dest", s))
		commands = append(commands, qset("firewall."+fwd, "enabled", "1"))
	}
	commands = append(commands, generateUmdns(services)...)
	commands = append(commands, generateMdnsRepeater(services)...)

	batchCommands(commands)
	run("uci", "-q", "commit", "network")
	run("uci", "-q", "commit", "firewall")
	run("uci", "-q", "commit", "dhcp")
	run("uci", "-q", "commit", "stageneth")
	run("uci", "-q", "commit", "umdns")
	run("uci", "-q", "commit", "mdns_repeater")

	run("uci", "-q", "delete", "dhcp.bb")
	run("uci", "-q", "commit", "dhcp")

	run("/etc/init.d/network", "reload")
	tuneNics()
	run("/etc/init.d/firewall", "reload")
	run("/etc/init.d/umdns", "enable")
	run("/etc/init.d/umdns", "restart")
	run("/etc/init.d/mdns-repeater", "enable")
	run("/etc/init.d/mdns-repeater", "restart")
	run("/etc/init.d/stageneth-ptp", "enable")
	run("/etc/init.d/stageneth-ptp", "restart")
	run("/etc/init.d/dnsmasq", "stop")
	run("sleep", "1")
	run("/etc/init.d/dnsmasq", "start")
	fmt.Println("StageNeth network configuration applied.")
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "apply" {
		fmt.Println("Usage: stageneth-network apply")
		os.Exit(1)
	}
	apply()
}
