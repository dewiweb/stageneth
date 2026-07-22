#!/bin/bash
#
# Lance NethSecurity dans QEMU avec bridge réseau br-nethsec
# Permet l'accès direct depuis l'hôte à l'IP 192.168.1.1
#

set -e

IMAGE_PATH="${1:-nethsecurity-8.7.2-x86-64-generic-squashfs-combined-efi.img}"
BRIDGE="${QEMU_BRIDGE:-br-nethsec}"
MEMORY="${QEMU_MEMORY:-2048}"
CPUS="${QEMU_CPUS:-2}"

# Prérequis réseau
if ! ip link show "$BRIDGE" >/dev/null 2>&1; then
    echo "Création du bridge $BRIDGE ..."
    sudo ip link add name "$BRIDGE" type bridge
    sudo ip addr add 192.168.1.254/24 dev "$BRIDGE"
    sudo ip link set "$BRIDGE" up
fi

# Autoriser le bridge dans QEMU
if ! grep -q "allow $BRIDGE" /etc/qemu/bridge.conf 2>/dev/null; then
    echo "allow $BRIDGE" | sudo tee /etc/qemu/bridge.conf >/dev/null
    sudo chmod 0644 /etc/qemu/bridge.conf
fi

# Lancer QEMU
exec qemu-system-x86_64 \
    -m "$MEMORY" \
    -smp "$CPUS" \
    -enable-kvm \
    -drive "file=$IMAGE_PATH,if=virtio,format=raw" \
    -netdev "bridge,id=net0,br=$BRIDGE" \
    -device e1000,netdev=net0 \
    -nographic \
    -serial mon:stdio
