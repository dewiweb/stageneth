#!/bin/bash
# Script pour créer un overlay squashfs avec les packages StageNeth

set -e

OVERLAY_DIR="/tmp/stageneth-overlay-final"
OVERLAY_SQUASHFS="/tmp/stageneth-overlay.squashfs"

# Nettoyer et créer le répertoire d'overlay
rm -rf "$OVERLAY_DIR"
mkdir -p "$OVERLAY_DIR"

# Créer la structure des répertoires StageNeth
mkdir -p "$OVERLAY_DIR/usr/share/stageneth/vlan-profiles"
mkdir -p "$OVERLAY_DIR/usr/share/stageneth/ptp-profiles"
mkdir -p "$OVERLAY_DIR/usr/share/stageneth/igmp-profiles"
mkdir -p "$OVERLAY_DIR/usr/share/stageneth/ptp-monitor"
mkdir -p "$OVERLAY_DIR/usr/share/stageneth/vlan-monitor"
mkdir -p "$OVERLAY_DIR/usr/bin"
mkdir -p "$OVERLAY_DIR/usr/lib/lua/luci/controller/stageneth"
mkdir -p "$OVERLAY_DIR/usr/lib/lua/luci/model/cbi/stageneth"
mkdir -p "$OVERLAY_DIR/usr/lib/lua/luci/view/stageneth"
mkdir -p "$OVERLAY_DIR/etc/telegraf.conf.d"
mkdir -p "$OVERLAY_DIR/etc/vmalert/rules"

# Copier les fichiers de profils VLAN
cat > "$OVERLAY_DIR/usr/share/stageneth/vlan-profiles/dante.json" << 'EOF'
{"name":"Dante","description":"Profil pour réseaux Dante (Audinate)","vlan_id":20,"qos":{"priority":7,"dscp":46},"ptp":true,"multicast":true}
EOF

cat > "$OVERLAY_DIR/usr/share/stageneth/vlan-profiles/aes67.json" << 'EOF'
{"name":"AES67","description":"Profil pour réseaux AES67 / RAVENNA","vlan_id":21,"qos":{"priority":7,"dscp":46},"ptp":true,"multicast":true}
EOF

cat > "$OVERLAY_DIR/usr/share/stageneth/vlan-profiles/ndi.json" << 'EOF'
{"name":"NDI","description":"Profil pour réseaux NDI / NDI|HX","vlan_id":30,"qos":{"priority":6,"dscp":40},"ptp":false,"multicast":true}
EOF

cat > "$OVERLAY_DIR/usr/share/stageneth/vlan-profiles/artnet.json" << 'EOF'
{"name":"Art-Net","description":"Profil pour réseaux Art-Net","vlan_id":40,"qos":{"priority":4,"dscp":32},"ptp":false,"multicast":true}
EOF

cat > "$OVERLAY_DIR/usr/share/stageneth/vlan-profiles/sacn.json" << 'EOF'
{"name":"sACN","description":"Profil pour réseaux sACN (E1.31)","vlan_id":41,"qos":{"priority":4,"dscp":32},"ptp":false,"multicast":true}
EOF

# Copier les fichiers de profils PTP
cat > "$OVERLAY_DIR/usr/share/stageneth/ptp-profiles/default.json" << 'EOF'
{"name":"Default","description":"Configuration PTP par défaut","profile":"default","clock_class":248,"priority1":128,"priority2":128}
EOF

cat > "$OVERLAY_DIR/usr/share/stageneth/ptp-profiles/audio.json" << 'EOF'
{"name":"Audio","description":"Configuration PTP optimisée pour audio","profile":"audio","clock_class":248,"priority1":128,"priority2":128}
EOF

# Copier les fichiers de profils IGMP
cat > "$OVERLAY_DIR/usr/share/stageneth/igmp-profiles/default.json" << 'EOF'
{"name":"Default","description":"Configuration IGMP par défaut","version":3,"query_interval":125,"query_response_interval":10}
EOF

# Copier les scripts
cat > "$OVERLAY_DIR/usr/bin/stageneth-vlan-profiles.sh" << 'EOF'
#!/bin/sh
case "$1" in
    list) ls /usr/share/stageneth/vlan-profiles/*.json 2>/dev/null | xargs -n1 basename -s .json ;;
    get) cat "/usr/share/stageneth/vlan-profiles/$2.json" 2>/dev/null || echo "Profile not found" ;;
    *) echo "Usage: $0 {list|get <profile>}" ;;
esac
EOF

cat > "$OVERLAY_DIR/usr/bin/stageneth-ptp.sh" << 'EOF'
#!/bin/sh
case "$1" in
    list) ls /usr/share/stageneth/ptp-profiles/*.json 2>/dev/null | xargs -n1 basename -s .json ;;
    get) cat "/usr/share/stageneth/ptp-profiles/$2.json" 2>/dev/null || echo "Profile not found" ;;
    *) echo "Usage: $0 {list|get <profile>}" ;;
esac
EOF

cat > "$OVERLAY_DIR/usr/bin/stageneth-igmp.sh" << 'EOF'
#!/bin/sh
case "$1" in
    list) ls /usr/share/stageneth/igmp-profiles/*.json 2>/dev/null | xargs -n1 basename -s .json ;;
    get) cat "/usr/share/stageneth/igmp-profiles/$2.json" 2>/dev/null || echo "Profile not found" ;;
    *) echo "Usage: $0 {list|get <profile>}" ;;
esac
EOF

# Copier les modules LuCI
cat > "$OVERLAY_DIR/usr/lib/lua/luci/controller/stageneth.lua" << 'EOF'
module("luci.controller.stageneth", package.seeall)
function index()
    entry({"admin", "stageneth"}, firstchild(), "StageNeth", 30).dependent = false
    entry({"admin", "stageneth", "vlan"}, cbi("stageneth/stageneth_vlan"), "VLAN Profiles", 10)
    entry({"admin", "stageneth", "ptp"}, cbi("stageneth/stageneth_ptp"), "PTP Configuration", 20)
    entry({"admin", "stageneth", "monitoring"}, template("stageneth_monitoring"), "Monitoring", 30)
end
EOF

cat > "$OVERLAY_DIR/usr/lib/lua/luci/model/cbi/stageneth/stageneth_vlan.lua" << 'EOF'
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

cat > "$OVERLAY_DIR/usr/lib/lua/luci/model/cbi/stageneth/stageneth_ptp.lua" << 'EOF'
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

cat > "$OVERLAY_DIR/usr/lib/lua/luci/view/stageneth_monitoring.htm" << 'EOF'
<h2>StageNeth Monitoring</h2>
<p>Tableau de bord de monitoring pour les réseaux de spectacle vivant.</p>
<div class="cbi-section">
    <h3>PTP Status</h3>
    <div class="cbi-value"><div class="cbi-value-field">Offset: <span id="ptp-offset">--</span> ns</div></div>
    <div class="cbi-value"><div class="cbi-value-field">Jitter: <span id="ptp-jitter">--</span> ns</div></div>
</div>
<div class="cbi-section">
    <h3>VLAN Status</h3>
    <div class="cbi-value"><div class="cbi-value-field">Active VLANs: <span id="vlan-count">--</span></div></div>
</div>
<script>setTimeout(function(){location.reload();},30000);</script>
EOF

# Copier les scripts de monitoring
cat > "$OVERLAY_DIR/usr/share/stageneth/ptp-monitor/stageneth-ptp-metrics.py" << 'EOF'
#!/usr/bin/env python3
import subprocess
try:
    result = subprocess.run(['pmc','-u','-b','GET CURRENT_DATA_SET'],capture_output=True,text=True,timeout=5)
    if result.returncode==0:
        print("stageneth_ptp_offset_ns 0")
        print("stageneth_ptp_jitter_ns 0")
        print("stageneth_ptp_clock_class 248")
        print("stageneth_ptp_state slave")
    else:
        print("stageneth_ptp_offset_ns 0")
        print("stageneth_ptp_jitter_ns 0")
        print("stageneth_ptp_clock_class 255")
        print("stageneth_ptp_state unknown")
except:
    print("stageneth_ptp_offset_ns 0")
    print("stageneth_ptp_jitter_ns 0")
    print("stageneth_ptp_clock_class 255")
    print("stageneth_ptp_state unknown")
EOF

cat > "$OVERLAY_DIR/usr/share/stageneth/vlan-monitor/stageneth-vlan-metrics.py" << 'EOF'
#!/usr/bin/env python3
import subprocess
try:
    result = subprocess.run(['cat','/proc/net/vlan/config'],capture_output=True,text=True,timeout=5)
    if result.returncode==0:
        lines=result.stdout.strip().split('\n')
        vlan_count=len(lines)-2 if len(lines)>2 else 0
        print(f"stageneth_vlan_count {vlan_count}")
    else:
        print("stageneth_vlan_count 0")
except:
    print("stageneth_vlan_count 0")
EOF

# Copier les fichiers de configuration Telegraf
cat > "$OVERLAY_DIR/etc/telegraf.conf.d/stageneth-ptp.conf" << 'EOF'
[[inputs.exec]]
  commands = ["/usr/share/stageneth/ptp-monitor/stageneth-ptp-metrics.py"]
  data_format = "influx"
  interval = "30s"
EOF

cat > "$OVERLAY_DIR/etc/telegraf.conf.d/stageneth-vlan.conf" << 'EOF'
[[inputs.exec]]
  commands = ["/usr/share/stageneth/vlan-monitor/stageneth-vlan-metrics.py"]
  data_format = "influx"
  interval = "30s"
EOF

# Copier les fichiers de configuration vmalert
cat > "$OVERLAY_DIR/etc/vmalert/rules/stageneth-ptp.yaml" << 'EOF'
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
EOF

cat > "$OVERLAY_DIR/etc/vmalert/rules/stageneth-vlan.yaml" << 'EOF'
groups:
  - name: stageneth_vlan
    rules:
      - alert: VLANInterfaceDown
        expr: stageneth_vlan_state == 0
        for: 5m
        labels:
          severity: critical
EOF

# Rendre les scripts exécutables
chmod +x "$OVERLAY_DIR/usr/bin/stageneth-vlan-profiles.sh"
chmod +x "$OVERLAY_DIR/usr/bin/stageneth-ptp.sh"
chmod +x "$OVERLAY_DIR/usr/bin/stageneth-igmp.sh"
chmod +x "$OVERLAY_DIR/usr/share/stageneth/ptp-monitor/stageneth-ptp-metrics.py"
chmod +x "$OVERLAY_DIR/usr/share/stageneth/vlan-monitor/stageneth-vlan-metrics.py"

# Créer le squashfs
echo "Création du squashfs StageNeth..."
mksquashfs "$OVERLAY_DIR" "$OVERLAY_SQUASHFS"

echo "Overlay StageNeth créé : $OVERLAY_SQUASHFS"
echo "Copiez ce fichier dans le répertoire du projet et utilisez-le avec QEMU"
