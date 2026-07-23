# StageNeth

Routeur dédié au spectacle vivant, basé sur **OpenWrt**.
StageNeth intègre une interface web Vue 3, une API Go et un orchestrateur réseau Python pour séparer les flux audio/vidéo/lumière (AV) du réseau de gestion, tout en fournissant les services DHCP, NTP, mDNS et SNMP nécessaires aux équipements scéniques.

> **État du projet** : StageNeth est en **version de développement** (`0.2.0-beta.x`). Beaucoup de fonctionnalités restent à stabiliser, à valider sur banc et à compléter avant d'obtenir un OS déployable en production. Ne pas utiliser sur un spectacle réel sans validation approfondie.

## Fonctionnalités

- **Interface web Vue 3** en HTTPS avec deux niveaux d'accès : mode simple (opérateur) et mode expert (tech)
- **API Go** sécurisée par JWT
- **Orchestration réseau Python** : génération automatique des VLANs 802.1q, bridges, firewall, DHCP, QoS/NIC tuning
- **Configuration par service** : chaque protocole (Dante, NDI, AES67/RAVENNA, ST 2110, Art-Net, sACN, PTP, AVB, NMOS, MA-Net/Proprietary...) a son propre VLAN, son DSCP, sa priorité et son MTU
- **Trunk AV configurable** (`eth1` par défaut) et **interface dédiée ST 2110** (`eth2`) en jumbo frames (MTU 9000)
- **Serveur NTP** avec DHCP option 42 automatique par VLAN
- **Refleteur mDNS** (`umdns`) entre les VLANs AV
- **Surveillance** : monitoring CPU/mémoire/services, alertes, logs temps réel, découverte mDNS, agent SNMP et serveur syslog
- **Support constructeurs** : profils générique, Luminex GigaCore, Cisco SG350/AV, Netgear AV avec conseils IGMP/PTP/jumbo/DSCP
- **Firewall durci** : pas de NAT sur les zones média, inter-VLAN fermé par défaut, DHCP/DNS/NTP autorisés localement
- **Build conteneurisée** avec Podman

## Architecture

- `packages/` : packages OpenWrt StageNeth
  - `stageneth-api` : API Go
  - `stageneth-ui` : interface Vue 3
  - `stageneth-network` : orchestrateur Python et UCI defaults
- `files/` : overlay OpenWrt (`uci-defaults`, bannière, `openwrt_release`)
- `config/` : fragments de configuration OpenWrt
- `builder/` : Containerfile et scripts
- `build-stageneth.sh` : script de build
- `openwrt/` : arbre de build OpenWrt (ignoré par git)

## Configuration clé

| Fichier | Rôle |
|---|---|
| `/etc/config/stageneth` | Services, VLANs, bindings, forwardings, switch vendor, trunk |
| `packages/stageneth-network/files/config` | Template UCI par défaut |
| `build.conf` | Version (`STAGENETH_VERSION`) et suffixe de build (`BUILD_SEMVER_SUFFIX`) |

## Build rapide

```bash
./build-stageneth.sh
```

Les images finales sont copiées dans `bin/` à la racine.
Voir [docs/build.md](docs/build.md) pour les détails.

## Test rapide avec QEMU

Remplacez `0.2.0-beta.11` par la version indiquée dans `build.conf` :

```bash
gunzip -c bin/stageneth-0.2.0-beta.11-x86-64-generic-ext4-combined.img.gz > /tmp/stageneth-test.img
qemu-system-x86_64 -m 1024 -smp 2 -enable-kvm \
  -hda /tmp/stageneth-test.img -display none -serial file:/tmp/stageneth-qemu.log \
  -netdev user,id=net0,net=192.168.1.0/24,hostfwd=tcp::2222-192.168.1.1:22,hostfwd=tcp::8443-192.168.1.1:443 \
  -device e1000,netdev=net0
```

- **UI** : `https://192.168.1.1/` (forwardée sur `https://localhost:8443/`)
- **SSH** : `ssh -p 2222 root@localhost`
- **Mot de passe root test** : `stageneth`

Voir [docs/testing.md](docs/testing.md) pour le plan de validation complet (PTP, multicast, DSCP, MTU, DHCP, NTP, firewall).

## Notes

- L'image de test définit un mot de passe root connu (`stageneth`) et génère un secret JWT aléatoire au premier démarrage. **En production, changez impérativement ce mot de passe dès le premier lancement** via `Paramètres > Credentials` ou en SSH avec `passwd`.
- Le répertoire `openwrt/` est un arbre de build complet et est ignoré par git.

## Licence

GPL-2.0-only (conforme à OpenWrt).
