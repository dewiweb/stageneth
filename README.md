# StageNeth

Routeur dédié au spectacle vivant, basé sur **OpenWrt**.  
StageNeth intègre une interface web Vue 3, une API Go et un orchestrateur réseau Python pour gérer VLANs, QoS, firewall, NTP, mDNS et SNMP sur des réseaux audio/vidéo/lumière.

## Fonctionnalités

- Interface web Vue 3 servie en HTTPS sur le LAN
- API Go avec authentification JWT
- Orchestration réseau : VLANs 802.1q, bridges, firewall, QoS, NTP
- Surveillance : monitoring, logs en temps réel, découverte mDNS
- Build conteneurisée avec Podman

## Structure du dépôt

- `packages/` : packages OpenWrt StageNeth (`stageneth-api`, `stageneth-ui`, `stageneth-network`)
- `files/` : overlay OpenWrt (`uci-defaults`, bannière, `openwrt_release`)
- `config/` : fragments de configuration OpenWrt
- `builder/` : Containerfile et scripts de build
- `build-stageneth.sh` : script de build conteneurisé
- `openwrt/` : arbre de build OpenWrt (ignoré par git)

## Build rapide

```bash
./build-stageneth.sh
```

Les images sont copiées dans `bin/` à la racine.  
Voir [docs/build.md](docs/build.md) pour les détails.

## Test rapide avec QEMU

```bash
gunzip -c bin/stageneth-0.1.0-alpha-x86-64-generic-ext4-combined.img.gz > /tmp/stageneth-test.img
qemu-system-x86_64 -m 1024 -smp 2 -enable-kvm \
  -hda /tmp/stageneth-test.img -display none -serial stdio \
  -netdev user,id=net0,net=192.168.1.0/24,hostfwd=tcp::2222-192.168.1.1:22,hostfwd=tcp::8443-192.168.1.1:443 \
  -device e1000,netdev=net0
```

- **UI** : `https://192.168.1.1/` (forwardée sur `https://localhost:8443/`)
- **SSH** : `ssh -p 2222 root@localhost`
- **Mot de passe root test** : `stageneth`

Voir [docs/testing.md](docs/testing.md) pour la checklist complète.

## Notes

- L'image de test définit un mot de passe root connu (`stageneth`) et génère un secret JWT aléatoire au premier démarrage. **Ne pas utiliser en production telle quelle.**
- Le répertoire `openwrt/` est un arbre de build complet et est ignoré par git.

## Licence

GPL-2.0-only (conforme à OpenWrt).
