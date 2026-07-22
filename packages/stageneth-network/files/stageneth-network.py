#!/usr/bin/python3
#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

import os
import subprocess
import sys
import re


def uci_show(config):
    """Return a dict of UCI section/options for a given config."""
    result = subprocess.run(
        ['uci', '-q', 'show', config],
        capture_output=True,
        text=True
    )
    data = {'sections': {}, 'values': {}}
    if result.returncode != 0:
        return data

    sections = {}
    for line in result.stdout.splitlines():
        if '=' not in line:
            continue
        key, value = line.split('=', 1)
        # key like stageneth.dante=service or stageneth.dante.vlan_id=20
        parts = key.split('.')
        if len(parts) == 2:
            # section declaration: config.section=type
            config_name, section = parts
            sections[section] = value.strip("'")
        elif len(parts) == 3:
            config_name, section, option = parts
            sections.setdefault(section, '')
            if section not in data['values']:
                data['values'][section] = {}
            data['values'][section][option] = value.strip("'\"")
    data['sections'] = sections
    return data


def available_interfaces():
    return set(os.listdir('/sys/class/net'))


def fallback_iface(iface, available):
    if iface in available:
        return iface
    for cand in ['eth1', 'eth0', 'eth2']:
        if cand in available:
            return cand
    return 'eth0'


def batch_commands(commands):
    """Run a list of uci set commands one by one."""
    if not commands:
        return
    for cmd in commands:
        action, arg = cmd.split(' ', 1)
        # delete is allowed to fail when the option is absent
        subprocess.run(['uci', '-q', action, arg], check=(action != 'delete'))


def qset(section, option, value):
    """Return a uci set command (no shell quoting needed)."""
    return f"set {section}.{option}={value}"


def generate_network(svc_name, svc, bindings):
    """Generate UCI commands for network config for a service."""
    available = available_interfaces()
    vlan_id = svc.get('vlan_id', '1')
    ifname = f"svc_{svc_name}"
    bridge = f"br_{svc_name}"
    ports = []
    for bname, bcfg in bindings.items():
        if bcfg.get('service') == svc_name:
            iface = bcfg.get('interface', 'eth0')
            iface = fallback_iface(iface, available)
            if svc.get('untagged') == '1':
                ports.append(iface)
            else:
                ports.append(f"{iface}.{vlan_id}")
    if not ports:
        iface = fallback_iface('eth0', available)
        if svc.get('untagged') == '1':
            ports = [iface]
        else:
            ports = [f"{iface}.{vlan_id}"]

    ipaddr = f"10.{vlan_id}.0.1"
    cmds = [
        f"set network.{ifname}=interface",
        qset(f"network.{ifname}", 'device', bridge),
        qset(f"network.{ifname}", 'proto', 'static'),
        qset(f"network.{ifname}", 'ipaddr', ipaddr),
        qset(f"network.{ifname}", 'netmask', '255.255.255.0'),
    ]
    for port in ports:
        if '.' in port:
            vlan_dev = port.replace('.', '_')
            parent, vid = port.rsplit('.', 1)
            cmds.extend([
                f"set network.{vlan_dev}=device",
                qset(f"network.{vlan_dev}", 'name', port),
                qset(f"network.{vlan_dev}", 'type', '8021q'),
                qset(f"network.{vlan_dev}", 'vid', vid),
                qset(f"network.{vlan_dev}", 'ifname', parent),
            ])
    cmds.extend([
        f"set network.{bridge}=device",
        qset(f"network.{bridge}", 'name', bridge),
        qset(f"network.{bridge}", 'type', 'bridge'),
        qset(f"network.{bridge}", 'ports', ' '.join(ports)),
    ])
    if svc.get('multicast') == '1':
        cmds.append(qset(f"network.{bridge}", 'igmp_snooping', '1'))
    return cmds


def generate_firewall(svc_name, svc):
    """Generate UCI commands for firewall zone for a service."""
    zone = f"zone_{svc_name}"
    ifname = f"svc_{svc_name}"
    input_policy = 'ACCEPT' if svc.get('multicast') == '1' else 'REJECT'
    forward_policy = 'REJECT'
    dhcp_rule = f"allow_dhcp_{svc_name}"
    dns_rule = f"allow_dns_{svc_name}"
    ntp_rule = f"allow_ntp_{svc_name}"
    cmds = [
        f"set firewall.{zone}=zone",
        qset(f"firewall.{zone}", 'name', svc_name),
        qset(f"firewall.{zone}", 'network', ifname),
        qset(f"firewall.{zone}", 'input', input_policy),
        qset(f"firewall.{zone}", 'output', 'ACCEPT'),
        qset(f"firewall.{zone}", 'forward', forward_policy),
        f"set firewall.{dhcp_rule}=rule",
        qset(f"firewall.{dhcp_rule}", 'name', f"Allow-DHCP-{svc_name}"),
        qset(f"firewall.{dhcp_rule}", 'src', svc_name),
        qset(f"firewall.{dhcp_rule}", 'proto', 'udp'),
        qset(f"firewall.{dhcp_rule}", 'dest_port', '67'),
        qset(f"firewall.{dhcp_rule}", 'target', 'ACCEPT'),
        f"set firewall.{dns_rule}=rule",
        qset(f"firewall.{dns_rule}", 'name', f"Allow-DNS-{svc_name}"),
        qset(f"firewall.{dns_rule}", 'src', svc_name),
        qset(f"firewall.{dns_rule}", 'proto', 'tcp udp'),
        qset(f"firewall.{dns_rule}", 'dest_port', '53'),
        qset(f"firewall.{dns_rule}", 'target', 'ACCEPT'),
        f"set firewall.{ntp_rule}=rule",
        qset(f"firewall.{ntp_rule}", 'name', f"Allow-NTP-{svc_name}"),
        qset(f"firewall.{ntp_rule}", 'src', svc_name),
        qset(f"firewall.{ntp_rule}", 'proto', 'udp'),
        qset(f"firewall.{ntp_rule}", 'dest_port', '123'),
        qset(f"firewall.{ntp_rule}", 'target', 'ACCEPT'),
    ]
    return cmds


def generate_forwardings(forwardings):
    """Generate inter-VLAN forwarding rules."""
    cmds = []
    for fname, fcfg in forwardings.items():
        cmds.extend([
            f"set firewall.{fname}=forwarding",
            qset(f"firewall.{fname}", 'src', fcfg.get('src', '')),
            qset(f"firewall.{fname}", 'dest', fcfg.get('dest', '')),
            qset(f"firewall.{fname}", 'enabled', fcfg.get('enabled', '1')),
        ])
    return cmds


def generate_dhcp(svc_name, svc):
    """Generate a DHCP pool for a service VLAN (.101-.254 dynamic, .2-.100 static)."""
    ifname = f"svc_{svc_name}"
    return [
        f"set dhcp.{ifname}=dhcp",
        qset(f"dhcp.{ifname}", 'interface', ifname),
        qset(f"dhcp.{ifname}", 'start', '101'),
        qset(f"dhcp.{ifname}", 'limit', '154'),
        qset(f"dhcp.{ifname}", 'leasetime', '12h'),
        f"delete dhcp.{ifname}.interface_name",
        f"add_list dhcp.{ifname}.interface_name={ifname}",
    ]


def generate_qos(svc_name, svc):
    """Generate QoS/qosify rules for a service."""
    # Minimal: store DSCP and priority for later use by qosify/tc scripts.
    return [
        qset(f"stageneth.{svc_name}", 'dscp', svc.get('dscp', '0')),
        qset(f"stageneth.{svc_name}", 'priority', svc.get('priority', '0')),
    ]


def apply():
    st = uci_show('stageneth')
    sections = st['sections']
    values = st['values']

    services = {s: values.get(s, {}) for s, t in sections.items() if t == 'service'}
    bindings = {s: values.get(s, {}) for s, t in sections.items() if t == 'binding'}
    forwardings = {s: values.get(s, {}) for s, t in sections.items() if t == 'forwarding'}

    service_names = set(services.keys())
    forwarding_names = set(forwardings.keys())

    commands = []
    # Clean up network/dhcp/firewall sections for services no longer in stageneth
    for cfg, prefix in [
        ('network', 'svc_'),
        ('network', 'br-'),
        ('dhcp', 'svc_'),
        ('firewall', 'zone_'),
    ]:
        existing = uci_show(cfg)
        for section in existing['sections']:
            if section.startswith(prefix):
                svc = section[len(prefix):]
                if svc not in service_names:
                    commands.append(f"delete {cfg}.{section}")
    # Clean up stale stageneth forwardings and per-service DHCP/DNS rules
    existing_fw = uci_show('firewall')
    for section in existing_fw['sections']:
        if '_to_' in section and section not in forwarding_names:
            commands.append(f"delete firewall.{section}")
        for prefix in ['allow_dhcp_', 'allow_dns_', 'allow_ntp_']:
            if section.startswith(prefix):
                svc = section[len(prefix):]
                if svc not in service_names:
                    commands.append(f"delete firewall.{section}")

    for svc_name, svc in services.items():
        commands.extend(generate_network(svc_name, svc, bindings))
        commands.extend(generate_firewall(svc_name, svc))
        commands.extend(generate_dhcp(svc_name, svc))
        commands.extend(generate_qos(svc_name, svc))

    commands.extend(generate_forwardings(forwardings))

    batch_commands(commands)
    subprocess.run(['uci', '-q', 'commit', 'network'], check=True)
    subprocess.run(['uci', '-q', 'commit', 'firewall'], check=True)
    subprocess.run(['uci', '-q', 'commit', 'dhcp'], check=True)
    subprocess.run(['uci', '-q', 'commit', 'stageneth'], check=True)

    # Remove stale second dnsmasq instance and reload services
    subprocess.run(['uci', '-q', 'delete', 'dhcp.bb'], check=False)
    subprocess.run(['uci', '-q', 'commit', 'dhcp'], check=False)
    subprocess.run(['/etc/init.d/network', 'reload'], check=False)
    subprocess.run(['/etc/init.d/firewall', 'reload'], check=False)
    subprocess.run(['/etc/init.d/dnsmasq', 'stop'], check=False)
    subprocess.run(['sleep', '1'], check=False)
    subprocess.run(['/etc/init.d/dnsmasq', 'start'], check=False)
    print('StageNeth network configuration applied.')


def main():
    if len(sys.argv) < 2 or sys.argv[1] != 'apply':
        print('Usage: stageneth-network.py apply')
        sys.exit(1)
    apply()


if __name__ == '__main__':
    main()
