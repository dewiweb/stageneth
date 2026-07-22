#!/bin/bash
# Script pour tester l'installation StageNeth dans QEMU

set -e

IMAGE_FILE="${1:-nethsecurity-8.7.2-x86-64-generic-squashfs-combined-efi.img}"

if [ ! -f "$IMAGE_FILE" ]; then
    echo "Erreur: Image non trouvée: $IMAGE_FILE"
    exit 1
fi

echo "Lancement de QEMU pour tester l'installation StageNeth..."
echo "Image: $IMAGE_FILE"
echo ""
echo "Instructions:"
echo "1. Appuyez sur Entrée pour activer la console"
echo "2. Connectez-vous en tant que root (mot de passe: Nethesis,1234)"
echo "3. Copiez et collez le contenu de scripts/install-stageneth-simple.sh"
echo "4. Testez avec: stageneth-vlan-profiles.sh list"
echo ""
echo "Appuyez sur Ctrl+A puis X pour quitter"
echo ""

./scripts/run-qemu.sh "$IMAGE_FILE"
