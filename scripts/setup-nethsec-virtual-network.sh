#!/bin/bash
#
# Configure un bridge reseau dedie pour lancer NethSecurity dans QEMU
# et obtenir l'acces direct depuis l'hote a 192.168.1.1
#

set -e

BRIDGE="${QEMU_BRIDGE:-br-nethsec}"

if ! ip link show "$BRIDGE" >/dev/null 2>&1; then
    echo "Creation du bridge $BRIDGE ..."
    sudo ip link add name "$BRIDGE" type bridge
    sudo ip addr add 192.168.1.254/24 dev "$BRIDGE"
    sudo ip link set "$BRIDGE" up
fi

if ! grep -q "allow $BRIDGE" /etc/qemu/bridge.conf 2>/dev/null; then
    echo "Autorisation du bridge dans QEMU ..."
    echo "allow $BRIDGE" | sudo tee /etc/qemu/bridge.conf >/dev/null
    sudo chmod 0644 /etc/qemu/bridge.conf
fi

if ! pgrep -x dnsmasq >/dev/null 2>&1 || ! ss -lnp | grep -q ":53.*dnsmasq"; then
    echo "Lancement de dnsmasq DHCP/DNS sur $BRIDGE ..."
    nohup sudo dnsmasq --interface="$BRIDGE" --bind-interfaces \
        --dhcp-range=192.168.1.100,192.168.1.200,255.255.255.0,12h \
        --dhcp-option=3,192.168.1.254 >/dev/null 2>&1 &
fi

echo "Bridge $BRIDGE pret. Lancez QEMU avec:"
echo "  qemu-system-x86_64 -m 2048 -smp 2 -enable-kvm -drive file=NETHSECURITY.img,if=virtio,format=raw -netdev bridge,id=net0,br=$BRIDGE -device e1000,netdev=net0 -nographic -serial mon:stdio"
echo "Accedez ensuite a https://192.168.1.1 (root / Nethesis,1234)"
