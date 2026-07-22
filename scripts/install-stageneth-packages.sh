#!/bin/bash
# Script d'installation manuelle des packages StageNeth dans une image NethSecurity

set -e

IMG_FILE="$1"
MOUNT_DIR="/tmp/nethsecurity-mount"

if [ -z "$IMG_FILE" ]; then
    echo "Usage: $0 <image-file>"
    exit 1
fi

if [ ! -f "$IMG_FILE" ]; then
    echo "Error: Image file not found: $IMG_FILE"
    exit 1
fi

# Créer le répertoire de montage
mkdir -p "$MOUNT_DIR"

# Monter la partition squashfs (partition 2)
echo "Mounting image..."
sudo mount -o loop,offset=$((33280*512)) "$IMG_FILE" "$MOUNT_DIR"

# Copier les fichiers des packages StageNeth
echo "Installing StageNeth packages..."

# stageneth-vlan-profiles
echo "Installing stageneth-vlan-profiles..."
sudo mkdir -p "$MOUNT_DIR/usr/share/stageneth/vlan-profiles"
sudo cp nethsecurity/packages/stageneth-vlan-profiles/files/*.json "$MOUNT_DIR/usr/share/stageneth/vlan-profiles/"
sudo cp nethsecurity/packages/stageneth-vlan-profiles/files/stageneth-vlan-profiles.sh "$MOUNT_DIR/usr/bin/"
sudo chmod +x "$MOUNT_DIR/usr/bin/stageneth-vlan-profiles.sh"

# stageneth-ptp
echo "Installing stageneth-ptp..."
sudo mkdir -p "$MOUNT_DIR/usr/share/stageneth/ptp-profiles"
sudo cp nethsecurity/packages/stageneth-ptp/files/*.json "$MOUNT_DIR/usr/share/stageneth/ptp-profiles/"
sudo cp nethsecurity/packages/stageneth-ptp/files/stageneth-ptp.sh "$MOUNT_DIR/usr/bin/"
sudo chmod +x "$MOUNT_DIR/usr/bin/stageneth-ptp.sh"

# stageneth-igmp
echo "Installing stageneth-igmp..."
sudo mkdir -p "$MOUNT_DIR/usr/share/stageneth/igmp-profiles"
sudo cp nethsecurity/packages/stageneth-igmp/files/*.json "$MOUNT_DIR/usr/share/stageneth/igmp-profiles/"
sudo cp nethsecurity/packages/stageneth-igmp/files/stageneth-igmp.sh "$MOUNT_DIR/usr/bin/"
sudo chmod +x "$MOUNT_DIR/usr/bin/stageneth-igmp.sh"

# stageneth-luci-module
echo "Installing stageneth-luci-module..."
sudo mkdir -p "$MOUNT_DIR/usr/lib/lua/luci/controller/stageneth"
sudo cp nethsecurity/packages/stageneth-luci-module/files/*.lua "$MOUNT_DIR/usr/lib/lua/luci/controller/stageneth/"
sudo mkdir -p "$MOUNT_DIR/usr/lib/lua/luci/model/cbi/stageneth"
sudo cp nethsecurity/packages/stageneth-luci-module/files/stageneth_*.lua "$MOUNT_DIR/usr/lib/lua/luci/model/cbi/stageneth/"
sudo mkdir -p "$MOUNT_DIR/usr/lib/lua/luci/view/stageneth"
sudo cp nethsecurity/packages/stageneth-luci-module/files/*.htm "$MOUNT_DIR/usr/lib/lua/luci/view/stageneth/"

# stageneth-ptp-monitor
echo "Installing stageneth-ptp-monitor..."
sudo mkdir -p "$MOUNT_DIR/usr/share/stageneth/ptp-monitor"
sudo cp nethsecurity/packages/stageneth-ptp-monitor/files/stageneth-ptp-metrics.py "$MOUNT_DIR/usr/share/stageneth/ptp-monitor/"
sudo mkdir -p "$MOUNT_DIR/etc/telegraf.conf.d"
sudo cp nethsecurity/packages/stageneth-ptp-monitor/files/ptp.conf "$MOUNT_DIR/etc/telegraf.conf.d/stageneth-ptp.conf"
sudo mkdir -p "$MOUNT_DIR/etc/vmalert/rules"
sudo cp nethsecurity/packages/stageneth-ptp-monitor/files/stageneth-ptp.yaml "$MOUNT_DIR/etc/vmalert/rules/"

# stageneth-vlan-monitor
echo "Installing stageneth-vlan-monitor..."
sudo mkdir -p "$MOUNT_DIR/usr/share/stageneth/vlan-monitor"
sudo cp nethsecurity/packages/stageneth-vlan-monitor/files/stageneth-vlan-metrics.py "$MOUNT_DIR/usr/share/stageneth/vlan-monitor/"
sudo mkdir -p "$MOUNT_DIR/etc/telegraf.conf.d"
sudo cp nethsecurity/packages/stageneth-vlan-monitor/files/vlan.conf "$MOUNT_DIR/etc/telegraf.conf.d/stageneth-vlan.conf"
sudo mkdir -p "$MOUNT_DIR/etc/vmalert/rules"
sudo cp nethsecurity/packages/stageneth-vlan-monitor/files/stageneth-vlan.yaml "$MOUNT_DIR/etc/vmalert/rules/"

echo "Unmounting image..."
sudo umount "$MOUNT_DIR"

echo "StageNeth packages installed successfully!"
