#!/bin/bash
#
# Script pour lancer une image OpenWrt dans QEMU avec accès webui
# Basé sur la documentation officielle OpenWrt x86
#

set -e

IMAGE_PATH="${1:-openwrt-23.05.5-x86-64-generic-ext4-combined.img}"
MEMORY="${QEMU_MEMORY:-1024}"
CPUS="${QEMU_CPUS:-2}"
SSH_PORT="${QEMU_SSH_PORT:-2222}"
HTTP_PORT="${QEMU_HTTP_PORT:-8080}"
HTTPS_PORT="${QEMU_HTTPS_PORT:-8443}"

if [ ! -f "$IMAGE_PATH" ]; then
    echo "Erreur: L'image n'existe pas: $IMAGE_PATH"
    exit 1
fi

echo "Lancement de QEMU OpenWrt avec:"
echo "  Image: $IMAGE_PATH"
echo "  Mémoire: ${MEMORY}MB"
echo "  CPUs: $CPUS"
echo "  Port SSH: $SSH_PORT"
echo "  Port HTTP: $HTTP_PORT"
echo "  Port HTTPS: $HTTPS_PORT"

QEMU_CMD="qemu-system-x86_64"
QEMU_CMD+=" -m $MEMORY"
QEMU_CMD+=" -smp $CPUS"
QEMU_CMD+=" -enable-kvm"

# Disque
QEMU_CMD+=" -drive file=$IMAGE_PATH,if=virtio,format=raw"

# Réseau avec ports forward pour webui
QEMU_CMD+=" -nic user,model=virtio,hostfwd=tcp::$SSH_PORT-:22,hostfwd=tcp::$HTTP_PORT-:80,hostfwd=tcp::$HTTPS_PORT-:443"

# Display
QEMU_CMD+=" -nographic"

# Serial console
QEMU_CMD+=" -serial mon:stdio"

echo ""
echo "Commande QEMU:"
echo "$QEMU_CMD"
echo ""
echo "Appuyez sur Ctrl+A puis X pour quitter"
echo "SSH: ssh -p $SSH_PORT root@localhost"
echo "WebUI: http://localhost:$HTTP_PORT ou https://localhost:$HTTPS_PORT"
echo ""

$QEMU_CMD
