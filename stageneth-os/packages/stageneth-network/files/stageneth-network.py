#!/usr/bin/python3
#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

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


def batch_commands(commands):
    """Run a list of uci commands through batch."""
    if not commands:
        return
    input_text = '\n'.join(commands) + '\n'
    subprocess.run(['uci', '-q', 'batch'], input=input_text, text=True, check=True)


def qset(section, option, value):
    """Return a quoted uci set command."""
    return f"set {section}.{option}='{value}'"


def generate_network(svc_name, svc, bindings):
    """Generate UCI commands for network config for a service."""
    vlan_id = svc.get('vlan_id', '1')
    ifname = f"svc-{svc_name}"
    bridge = f"br-{svc_name}"
    ports = []
    for bname, bcfg in bindings.items():
        if bcfg.get('service') == svc_name:
            iface = bcfg.get('interface', 'eth0')
            ports.append(f"{iface}.{vlan_id}")
    if not ports:
        ports = [f"eth0.{vlan_id}"]

    cmds = [
        f"set network.{ifname}=interface",
        qset(f"network.{ifname}", 'device', bridge),
        qset(f"network.{ifname}", 'proto', 'none'),
        f"set network.{bridge}=device",
        qset(f"network.{bridge}", 'name', bridge),
        qset(f"network.{bridge}", 'type', 'bridge'),
        qset(f"network.{bridge}", 'ports', ' '.join(ports)),
    ]
    return cmds


def generate_firewall(svc_name, svc):
    """Generate UCI commands for firewall zone for a service."""
    zone = f"zone_{svc_name}"
    ifname = f"svc-{svc_name}"
    input_policy = 'ACCEPT' if svc.get('multicast') == '1' else 'REJECT'
    forward_policy = 'REJECT'
    return [
        f"set firewall.{zone}=zone",
        qset(f"firewall.{zone}", 'name', svc_name),
        qset(f"firewall.{zone}", 'network', ifname),
        qset(f"firewall.{zone}", 'input', input_policy),
        qset(f"firewall.{zone}", 'output', 'ACCEPT'),
        qset(f"firewall.{zone}", 'forward', forward_policy),
    ]


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

    commands = []
    for svc_name, svc in services.items():
        commands.extend(generate_network(svc_name, svc, bindings))
        commands.extend(generate_firewall(svc_name, svc))
        commands.extend(generate_qos(svc_name, svc))

    commands.extend(generate_forwardings(forwardings))

    batch_commands(commands)
    subprocess.run(['uci', '-q', 'commit', 'network'], check=True)
    subprocess.run(['uci', '-q', 'commit', 'firewall'], check=True)
    subprocess.run(['uci', '-q', 'commit', 'stageneth'], check=True)

    # Reload services
    subprocess.run(['/etc/init.d/network', 'reload'], check=False)
    subprocess.run(['/etc/init.d/firewall', 'reload'], check=False)
    print('StageNeth network configuration applied.')


def main():
    if len(sys.argv) < 2 or sys.argv[1] != 'apply':
        print('Usage: stageneth-network.py apply')
        sys.exit(1)
    apply()


if __name__ == '__main__':
    main()
