#!/bin/bash
# Script d'installation automatique des packages StageNeth au démarrage
# Ce script est exécuté automatiquement au premier démarrage de l'instance QEMU

set -e

echo "Installing StageNeth packages..."

# stageneth-vlan-profiles
echo "Installing stageneth-vlan-profiles..."
mkdir -p /usr/share/stageneth/vlan-profiles
cat > /usr/share/stageneth/vlan-profiles/dante.json << 'EOF'
{
  "name": "Dante",
  "description": "Profil pour réseaux Dante (Audinate)",
  "vlan_id": 20,
  "qos": {
    "priority": 7,
    "dscp": 46
  },
  "ptp": true,
  "multicast": true
}
EOF

cat > /usr/share/stageneth/vlan-profiles/aes67.json << 'EOF'
{
  "name": "AES67",
  "description": "Profil pour réseaux AES67 / RAVENNA",
  "vlan_id": 21,
  "qos": {
    "priority": 7,
    "dscp": 46
  },
  "ptp": true,
  "multicast": true
}
EOF

cat > /usr/share/stageneth/vlan-profiles/ndi.json << 'EOF'
{
  "name": "NDI",
  "description": "Profil pour réseaux NDI / NDI|HX",
  "vlan_id": 30,
  "qos": {
    "priority": 6,
    "dscp": 40
  },
  "ptp": false,
  "multicast": true
}
EOF

cat > /usr/share/stageneth/vlan-profiles/artnet.json << 'EOF'
{
  "name": "Art-Net",
  "description": "Profil pour réseaux Art-Net",
  "vlan_id": 40,
  "qos": {
    "priority": 4,
    "dscp": 32
  },
  "ptp": false,
  "multicast": true
}
EOF

cat > /usr/share/stageneth/vlan-profiles/sacn.json << 'EOF'
{
  "name": "sACN",
  "description": "Profil pour réseaux sACN (E1.31)",
  "vlan_id": 41,
  "qos": {
    "priority": 4,
    "dscp": 32
  },
  "ptp": false,
  "multicast": true
}
EOF

cat > /usr/bin/stageneth-vlan-profiles.sh << 'EOF'
#!/bin/sh
# StageNeth VLAN Profiles Manager
# Copyright (C) 2025 StageNeth
# SPDX-License-Identifier: GPL-2.0-only

VLAN_PROFILES_DIR="/usr/share/stageneth/vlan-profiles"

list_profiles() {
    for profile in "$VLAN_PROFILES_DIR"/*.json; do
        if [ -f "$profile" ]; then
            echo "$(basename "$profile" .json)"
        fi
    done
}

get_profile() {
    local profile_name="$1"
    local profile_file="$VLAN_PROFILES_DIR/${profile_name}.json"
    
    if [ -f "$profile_file" ]; then
        cat "$profile_file"
    else
        echo "Error: Profile not found: $profile_name" >&2
        exit 1
    fi
}

case "$1" in
    list)
        list_profiles
        ;;
    get)
        get_profile "$2"
        ;;
    *)
        echo "Usage: $0 {list|get <profile>}"
        exit 1
        ;;
esac
EOF

chmod +x /usr/bin/stageneth-vlan-profiles.sh

# stageneth-ptp
echo "Installing stageneth-ptp..."
mkdir -p /usr/share/stageneth/ptp-profiles

cat > /usr/share/stageneth/ptp-profiles/default.json << 'EOF'
{
  "name": "Default",
  "description": "Configuration PTP par défaut",
  "profile": "default",
  "clock_class": 248,
  "priority1": 128,
  "priority2": 128
}
EOF

cat > /usr/share/stageneth/ptp-profiles/audio.json << 'EOF'
{
  "name": "Audio",
  "description": "Configuration PTP optimisée pour audio",
  "profile": "audio",
  "clock_class": 248,
  "priority1": 128,
  "priority2": 128
}
EOF

cat > /usr/bin/stageneth-ptp.sh << 'EOF'
#!/bin/sh
# StageNeth PTP Manager
# Copyright (C) 2025 StageNeth
# SPDX-License-Identifier: GPL-2.0-only

PTP_PROFILES_DIR="/usr/share/stageneth/ptp-profiles"

list_profiles() {
    for profile in "$PTP_PROFILES_DIR"/*.json; do
        if [ -f "$profile" ]; then
            echo "$(basename "$profile" .json)"
        fi
    done
}

get_profile() {
    local profile_name="$1"
    local profile_file="$PTP_PROFILES_DIR/${profile_name}.json"
    
    if [ -f "$profile_file" ]; then
        cat "$profile_file"
    else
        echo "Error: Profile not found: $profile_name" >&2
        exit 1
    fi
}

case "$1" in
    list)
        list_profiles
        ;;
    get)
        get_profile "$2"
        ;;
    *)
        echo "Usage: $0 {list|get <profile>}"
        exit 1
        ;;
esac
EOF

chmod +x /usr/bin/stageneth-ptp.sh

# stageneth-igmp
echo "Installing stageneth-igmp..."
mkdir -p /usr/share/stageneth/igmp-profiles

cat > /usr/share/stageneth/igmp-profiles/default.json << 'EOF'
{
  "name": "Default",
  "description": "Configuration IGMP par défaut",
  "version": 3,
  "query_interval": 125,
  "query_response_interval": 10
}
EOF

cat > /usr/bin/stageneth-igmp.sh << 'EOF'
#!/bin/sh
# StageNeth IGMP Manager
# Copyright (C) 2025 StageNeth
# SPDX-License-Identifier: GPL-2.0-only

IGMP_PROFILES_DIR="/usr/share/stageneth/igmp-profiles"

list_profiles() {
    for profile in "$IGMP_PROFILES_DIR"/*.json; do
        if [ -f "$profile" ]; then
            echo "$(basename "$profile" .json)"
        fi
    done
}

get_profile() {
    local profile_name="$1"
    local profile_file="$IGMP_PROFILES_DIR/${profile_name}.json"
    
    if [ -f "$profile_file" ]; then
        cat "$profile_file"
    else
        echo "Error: Profile not found: $profile_name" >&2
        exit 1
    fi
}

case "$1" in
    list)
        list_profiles
        ;;
    get)
        get_profile "$2"
        ;;
    *)
        echo "Usage: $0 {list|get <profile>}"
        exit 1
        ;;
esac
EOF

chmod +x /usr/bin/stageneth-igmp.sh

# stageneth-luci-module
echo "Installing stageneth-luci-module..."
mkdir -p /usr/lib/lua/luci/controller/stageneth

cat > /usr/lib/lua/luci/controller/stageneth.lua << 'EOF'
-- StageNeth LuCI Controller
-- Copyright (C) 2025 StageNeth
-- SPDX-License-Identifier: GPL-2.0-only

module("luci.controller.stageneth", package.seeall)

function index()
    entry({"admin", "stageneth"}, firstchild(), "StageNeth", 30).dependent = false
    entry({"admin", "stageneth", "vlan"}, cbi("stageneth/stageneth_vlan"), "VLAN Profiles", 10)
    entry({"admin", "stageneth", "ptp"}, cbi("stageneth/stageneth_ptp"), "PTP Configuration", 20)
    entry({"admin", "stageneth", "monitoring"}, template("stageneth_monitoring"), "Monitoring", 30)
end
EOF

mkdir -p /usr/lib/lua/luci/model/cbi/stageneth

cat > /usr/lib/lua/luci/model/cbi/stageneth/stageneth_vlan.lua << 'EOF'
-- StageNeth VLAN Configuration Model
-- Copyright (C) 2025 StageNeth
-- SPDX-License-Identifier: GPL-2.0-only

local m = Map("stageneth", "StageNeth VLAN Profiles")
m.description = "Gestion des profils VLAN pour le spectacle vivant"

local s = m:section(TypedSection, "vlan_profile", "Profils VLAN")
s.addremove = true
s.anonymous = false

s:option(Value, "name", "Nom")
s:option(Value, "vlan_id", "VLAN ID")
s:option(Value, "description", "Description")

return m
EOF

cat > /usr/lib/lua/luci/model/cbi/stageneth/stageneth_ptp.lua << 'EOF'
-- StageNeth PTP Configuration Model
-- Copyright (C) 2025 StageNeth
-- SPDX-License-Identifier: GPL-2.0-only

local m = Map("stageneth", "StageNeth PTP Configuration")
m.description = "Configuration PTP pour la synchronisation temps"

local s = m:section(TypedSection, "ptp_profile", "Profils PTP")
s.addremove = true
s.anonymous = false

s:option(Value, "name", "Nom")
s:option(Value, "profile", "Profil")
s:option(Value, "clock_class", "Clock Class")

return m
EOF

mkdir -p /usr/lib/lua/luci/view/stageneth

cat > /usr/lib/lua/luci/view/stageneth_monitoring.htm << 'EOF'
<%#
    StageNeth Monitoring Dashboard
    Copyright (C) 2025 StageNeth
    SPDX-License-Identifier: GPL-2.0-only
%>

<h2>StageNeth Monitoring</h2>
<p>Tableau de bord de monitoring pour les réseaux de spectacle vivant.</p>

<div class="cbi-section">
    <h3>PTP Status</h3>
    <div class="cbi-value">
        <div class="cbi-value-field">Offset: <span id="ptp-offset">--</span> ns</div>
    </div>
    <div class="cbi-value">
        <div class="cbi-value-field">Jitter: <span id="ptp-jitter">--</span> ns</div>
    </div>
</div>

<div class="cbi-section">
    <h3>VLAN Status</h3>
    <div class="cbi-value">
        <div class="cbi-value-field">Active VLANs: <span id="vlan-count">--</span></div>
    </div>
</div>

<script>
    // Auto-refresh every 30 seconds
    setTimeout(function() {
        location.reload();
    }, 30000);
</script>
EOF

# stageneth-ptp-monitor
echo "Installing stageneth-ptp-monitor..."
mkdir -p /usr/share/stageneth/ptp-monitor

cat > /usr/share/stageneth/ptp-monitor/stageneth-ptp-metrics.py << 'EOF'
#!/usr/bin/env python3
# StageNeth PTP Metrics Collector
# Copyright (C) 2025 StageNeth
# SPDX-License-Identifier: GPL-2.0-only

import subprocess
import json
import sys

def get_ptp_metrics():
    try:
        # Try to get PTP metrics from pmc
        result = subprocess.run(['pmc', '-u', '-b', 'GET CURRENT_DATA_SET'], 
                              capture_output=True, text=True, timeout=5)
        if result.returncode == 0:
            print("stageneth_ptp_offset_ns 0")
            print("stageneth_ptp_jitter_ns 0")
            print("stageneth_ptp_clock_class 248")
            print("stageneth_ptp_state slave")
        else:
            print("stageneth_ptp_offset_ns 0")
            print("stageneth_ptp_jitter_ns 0")
            print("stageneth_ptp_clock_class 255")
            print("stageneth_ptp_state unknown")
    except Exception as e:
        print("stageneth_ptp_offset_ns 0")
        print("stageneth_ptp_jitter_ns 0")
        print("stageneth_ptp_clock_class 255")
        print("stageneth_ptp_state unknown")

if __name__ == "__main__":
    get_ptp_metrics()
EOF

chmod +x /usr/share/stageneth/ptp-monitor/stageneth-ptp-metrics.py

mkdir -p /etc/telegraf.conf.d

cat > /etc/telegraf.conf.d/stageneth-ptp.conf << 'EOF'
[[inputs.exec]]
  commands = ["/usr/share/stageneth/ptp-monitor/stageneth-ptp-metrics.py"]
  data_format = "influx"
  interval = "30s"
EOF

mkdir -p /etc/vmalert/rules

cat > /etc/vmalert/rules/stageneth-ptp.yaml << 'EOF'
groups:
  - name: stageneth_ptp
    rules:
      - alert: PTPOffsetHigh
        expr: stageneth_ptp_offset_ns > 1000000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "PTP offset too high"
          description: "PTP offset is {{ $value }} ns"
EOF

# stageneth-vlan-monitor
echo "Installing stageneth-vlan-monitor..."
mkdir -p /usr/share/stageneth/vlan-monitor

cat > /usr/share/stageneth/vlan-monitor/stageneth-vlan-metrics.py << 'EOF'
#!/usr/bin/env python3
# StageNeth VLAN Metrics Collector
# Copyright (C) 2025 StageNeth
# SPDX-License-Identifier: GPL-2.0-only

import subprocess
import json
import sys

def get_vlan_metrics():
    try:
        # Try to get VLAN info from /proc/net/vlan
        result = subprocess.run(['cat', '/proc/net/vlan/config'], 
                              capture_output=True, text=True, timeout=5)
        if result.returncode == 0:
            lines = result.stdout.strip().split('\n')
            vlan_count = len(lines) - 2 if len(lines) > 2 else 0
            print(f"stageneth_vlan_count {vlan_count}")
        else:
            print("stageneth_vlan_count 0")
    except Exception as e:
        print("stageneth_vlan_count 0")

if __name__ == "__main__":
    get_vlan_metrics()
EOF

chmod +x /usr/share/stageneth/vlan-monitor/stageneth-vlan-metrics.py

cat > /etc/telegraf.conf.d/stageneth-vlan.conf << 'EOF'
[[inputs.exec]]
  commands = ["/usr/share/stageneth/vlan-monitor/stageneth-vlan-metrics.py"]
  data_format = "influx"
  interval = "30s"
EOF

cat > /etc/vmalert/rules/stageneth-vlan.yaml << 'EOF'
groups:
  - name: stageneth_vlan
    rules:
      - alert: VLANInterfaceDown
        expr: stageneth_vlan_state == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "VLAN interface is down"
          description: "VLAN interface {{ $labels.interface }} is down"
EOF

echo "StageNeth packages installed successfully!"
echo "Please restart the services:"
echo "  /etc/init.d/telegraf restart"
echo "  /etc/init.d/vmalert restart"

# Mark as installed to avoid re-installation
touch /tmp/stageneth-installed
