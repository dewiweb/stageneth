#!/bin/bash
#
# Script pour lancer une image StageNeth/NethSecurity dans QEMU
#

set -e

# Configuration
IMAGE_PATH="${1:-}"
MEMORY="${QEMU_MEMORY:-2048}"
CPUS="${QEMU_CPUS:-2}"
SSH_PORT="${QEMU_SSH_PORT:-2224}"
DISPLAY="${QEMU_DISPLAY:-none}"

# Vérifier si une image est spécifiée
if [ -z "$IMAGE_PATH" ]; then
    echo "Usage: $0 <chemin_image> [options]"
    echo ""
    echo "Options:"
    echo "  QEMU_MEMORY=2048     Mémoire en MB (défaut: 2048)"
    echo "  QEMU_CPUS=2          Nombre de CPUs (défaut: 2)"
    echo "  QEMU_SSH_PORT=2222   Port SSH forward (défaut: 2222)"
    echo "  QEMU_DISPLAY=none    Display: none, gtk, sdl (défaut: none)"
    echo ""
    echo "Exemples:"
    echo "  $0 nethsecurity/bin/targets/x86_64/stageneth-x86-64-generic-ext4.img.gz"
    echo "  QEMU_MEMORY=4096 $0 nethsecurity/bin/targets/x86_64/stageneth-x86-64-generic-ext4.img.gz"
    exit 1
fi

# Vérifier si l'image existe
if [ ! -f "$IMAGE_PATH" ]; then
    echo "Erreur: L'image n'existe pas: $IMAGE_PATH"
    exit 1
fi

# Décompresser si .gz
if [[ "$IMAGE_PATH" == *.gz ]]; then
    echo "Décompression de l'image..."
    gunzip -k "$IMAGE_PATH"
    IMAGE_PATH="${IMAGE_PATH%.gz}"
fi

echo "Lancement de QEMU avec:"
echo "  Image: $IMAGE_PATH"
echo "  Mémoire: ${MEMORY}MB"
echo "  CPUs: $CPUS"
echo "  Port SSH: $SSH_PORT"
echo "  Display: $DISPLAY"

# Construire la commande QEMU
QEMU_CMD="qemu-system-x86_64"
QEMU_CMD+=" -m $MEMORY"
QEMU_CMD+=" -smp $CPUS"
QEMU_CMD+=" -enable-kvm"

# Réseau
QEMU_CMD+=" -net nic,model=e1000"
QEMU_CMD+=" -net user,hostfwd=tcp::$SSH_PORT-:22,hostfwd=tcp::8080-:80,hostfwd=tcp::8443-:443"

# Partage de fichiers pour l'installation des packages StageNeth
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
QEMU_CMD+=" -virtfs local,path=$SCRIPT_DIR,security_model=mapped,mount_tag=stageneth_scripts"

# Display
if [ "$DISPLAY" = "none" ]; then
    QEMU_CMD+=" -nographic"
else
    QEMU_CMD+=" -display $DISPLAY"
fi

# Disque
QEMU_CMD+=" -drive file=$IMAGE_PATH,format=raw"

# Overlay StageNeth squashfs
OVERLAY_SQUASHFS="$(dirname "$IMAGE_PATH")/stageneth-overlay.squashfs"
if [ -f "$OVERLAY_SQUASHFS" ]; then
    echo "Overlay StageNeth détecté : $OVERLAY_SQUASHFS"
    QEMU_CMD+=" -drive file=$OVERLAY_SQUASHFS,format=raw,if=virtio,readonly=on"
fi

# Serial console
QEMU_CMD+=" -serial mon:stdio"

echo ""
echo "Commande QEMU:"
echo "$QEMU_CMD"
echo ""
echo "Appuyez sur Ctrl+A puis X pour quitter (mode -nographic)"
echo "Ou connectez-vous via SSH: ssh -p $SSH_PORT root@localhost"
echo ""

# Lancer QEMU
$QEMU_CMD
