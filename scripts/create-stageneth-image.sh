#!/bin/bash
# Script pour créer une image NethSecurity modifiée avec les packages StageNeth intégrés

set -e

IMAGE_FILE="$1"
OUTPUT_FILE="${IMAGE_FILE%.img}-stageneth.img"

if [ -z "$IMAGE_FILE" ]; then
    echo "Usage: $0 <image-file>"
    exit 1
fi

if [ ! -f "$IMAGE_FILE" ]; then
    echo "Error: Image file not found: $IMAGE_FILE"
    exit 1
fi

echo "Création d'une image NethSecurity modifiée avec StageNeth..."
echo "Image source : $IMAGE_FILE"
echo "Image sortie : $OUTPUT_FILE"

# Créer un répertoire de travail
WORK_DIR="/tmp/stageneth-work"
rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR"

# Copier l'image originale
cp "$IMAGE_FILE" "$OUTPUT_FILE"

# Monter l'image pour modification
MOUNT_DIR="$WORK_DIR/mount"
mkdir -p "$MOUNT_DIR"

# Trouver l'offset de la partition squashfs
OFFSET=$(fdisk -l "$OUTPUT_FILE" | grep "Linux" | awk '{print $2 * 512}')
echo "Offset de la partition squashfs : $OFFSET"

# Monter la partition squashfs
sudo mount -o loop,offset=$OFFSET "$OUTPUT_FILE" "$MOUNT_DIR"

# Créer les répertoires StageNeth
sudo mkdir -p "$MOUNT_DIR/usr/share/stageneth/vlan-profiles"
sudo mkdir -p "$MOUNT_DIR/usr/share/stageneth/ptp-profiles"
sudo mkdir -p "$MOUNT_DIR/usr/share/stageneth/igmp-profiles"
sudo mkdir -p "$MOUNT_DIR/usr/share/stageneth/ptp-monitor"
sudo mkdir -p "$MOUNT_DIR/usr/share/stageneth/vlan-monitor"
sudo mkdir -p "$MOUNT_DIR/usr/bin"
sudo mkdir -p "$MOUNT_DIR/usr/lib/lua/luci/controller/stageneth"
sudo mkdir -p "$MOUNT_DIR/usr/lib/lua/luci/model/cbi/stageneth"
sudo mkdir -p "$MOUNT_DIR/usr/lib/lua/luci/view/stageneth"
sudo mkdir -p "$MOUNT_DIR/etc/telegraf.conf.d"
sudo mkdir -p "$MOUNT_DIR/etc/vmalert/rules"

# Copier les fichiers de profils VLAN
sudo tee "$MOUNT_DIR/usr/share/stageneth/vlan-profiles/dante.json" > /dev/null << 'EOF'
{"name":"Dante","description":"Profil pour réseaux Dante (Audinate)","vlan_id":20,"qos":{"priority":7,"dscp":46},"ptp":true,"multicast":true}
EOF

sudo tee "$MOUNT_DIR/usr/share/stageneth/vlan-profiles/aes67.json" > /dev/null << 'EOF'
{"name":"AES67","description":"Profil pour réseaux AES67 / RAVENNA","vlan_id":21,"qos":{"priority":7,"dscp":46},"ptp":true,"multicast":true}
EOF

sudo tee "$MOUNT_DIR/usr/share/stageneth/vlan-profiles/ndi.json" > /dev/null << 'EOF'
{"name":"NDI","description":"Profil pour réseaux NDI / NDI|HX","vlan_id":30,"qos":{"priority":6,"dscp":40},"ptp":false,"multicast":true}
EOF

sudo tee "$MOUNT_DIR/usr/share/stageneth/vlan-profiles/artnet.json" > /dev/null << 'EOF'
{"name":"Art-Net","description":"Profil pour réseaux Art-Net","vlan_id":40,"qos":{"priority":4,"dscp":32},"ptp":false,"multicast":true}
EOF

sudo tee "$MOUNT_DIR/usr/share/stageneth/vlan-profiles/sacn.json" > /dev/null << 'EOF'
{"name":"sACN","description":"Profil pour réseaux sACN (E1.31)","vlan_id":41,"qos":{"priority":4,"dscp":32},"ptp":false,"multicast":true}
EOF

# Copier les fichiers de profils PTP
sudo tee "$MOUNT_DIR/usr/share/stageneth/ptp-profiles/default.json" > /dev/null << 'EOF'
{"name":"Default","description":"Configuration PTP par défaut","profile":"default","clock_class":248,"priority1":128,"priority2":128}
EOF

sudo tee "$MOUNT_DIR/usr/share/stageneth/ptp-profiles/audio.json" > /dev/null << 'EOF'
{"name":"Audio","description":"Configuration PTP optimisée pour audio","profile":"audio","clock_class":248,"priority1":128,"priority2":128}
EOF

# Copier les fichiers de profils IGMP
sudo tee "$MOUNT_DIR/usr/share/stageneth/igmp-profiles/default.json" > /dev/null << 'EOF'
{"name":"Default","description":"Configuration IGMP par défaut","version":3,"query_interval":125,"query_response_interval":10}
EOF

# Copier les scripts
sudo tee "$MOUNT_DIR/usr/bin/stageneth-vlan-profiles.sh" > /dev/null << 'EOF'
#!/bin/sh
case "$1" in
    list) ls /usr/share/stageneth/vlan-profiles/*.json 2>/dev/null | xargs -n1 basename -s .json ;;
    get) cat "/usr/share/stageneth/vlan-profiles/$2.json" 2>/dev/null || echo "Profile not found" ;;
    *) echo "Usage: $0 {list|get <profile>}" ;;
esac
EOF

sudo tee "$MOUNT_DIR/usr/bin/stageneth-ptp.sh" > /dev/null << 'EOF'
#!/bin/sh
case "$1" in
    list) ls /usr/share/stageneth/ptp-profiles/*.json 2>/dev/null | xargs -n1 basename -s .json ;;
    get) cat "/usr/share/stageneth/ptp-profiles/$2.json" 2>/dev/null || echo "Profile not found" ;;
    *) echo "Usage: $0 {list|get <profile>}" ;;
esac
EOF

sudo tee "$MOUNT_DIR/usr/bin/stageneth-igmp.sh" > /dev/null << 'EOF'
#!/bin/sh
case "$1" in
    list) ls /usr/share/stageneth/igmp-profiles/*.json 2>/dev/null | xargs -n1 basename -s .json ;;
    get) cat "/usr/share/stageneth/igmp-profiles/$2.json" 2>/dev/null || echo "Profile not found" ;;
    *) echo "Usage: $0 {list|get <profile>}" ;;
esac
EOF

# Rendre les scripts exécutables
sudo chmod +x "$MOUNT_DIR/usr/bin/stageneth-vlan-profiles.sh"
sudo chmod +x "$MOUNT_DIR/usr/bin/stageneth-ptp.sh"
sudo chmod +x "$MOUNT_DIR/usr/bin/stageneth-igmp.sh"

# Copier les modules LuCI
sudo tee "$MOUNT_DIR/usr/lib/lua/luci/controller/stageneth.lua" > /dev/null << 'EOF'
module("luci.controller.stageneth", package.seeall)
function index()
    entry({"admin", "stageneth"}, firstchild(), "StageNeth", 30).dependent = false
    entry({"admin", "stageneth", "vlan"}, cbi("stageneth/stageneth_vlan"), "VLAN Profiles", 10)
    entry({"admin", "stageneth", "ptp"}, cbi("stageneth/stageneth_ptp"), "PTP Configuration", 20)
    entry({"admin", "stageneth", "monitoring"}, template("stageneth_monitoring"), "Monitoring", 30)
end
EOF

sudo tee "$MOUNT_DIR/usr/lib/lua/luci/model/cbi/stageneth/stageneth_vlan.lua" > /dev/null << 'EOF'
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

sudo tee "$MOUNT_DIR/usr/lib/lua/luci/model/cbi/stageneth/stageneth_ptp.lua" > /dev/null << 'EOF'
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

sudo tee "$MOUNT_DIR/usr/lib/lua/luci/view/stageneth_monitoring.htm" > /dev/null << 'EOF'
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

# Démonter l'image
sudo umount "$MOUNT_DIR"

# Nettoyer
rm -rf "$WORK_DIR"

echo "Image NethSecurity modifiée créée : $OUTPUT_FILE"
echo "Vous pouvez maintenant tester cette image dans QEMU :"
echo "./scripts/run-qemu.sh $OUTPUT_FILE"
