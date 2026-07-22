# StageNeth — Routeur dédié au spectacle vivant

> Système d'exploitation de routeur, basé sur [NethSecurity](https://github.com/NethServer/nethsecurity) (fork OpenWrt + LuCI), optimisé pour gérer les réseaux professionnels de spectacle : **audio, vidéo, lumière**.

---

## Objectif

Dans le spectacle vivant, les réseaux techniques transportent simultanément des protocoles AV très sensibles au temps, à la latence, au multicast et à la priorisation. **StageNeth** propose un routeur préconfiguré et modulaire qui :

- Sépare automatiquement chaque famille de flux dans des **VLANs dédiés**.
- Applique des règles de **QoS / CoS / DSCP** adaptées à chaque protocole.
- Gère le **multicast** (IGMP snooping, querier, MLD) et le **PTP**.
- Offre une interface web simple via LuCI pour configurer un spectacle en quelques clics.

---

## Pourquoi NethSecurity / OpenWrt ?

- Mature, open-source, embarqué, compatible grand nombre de matériels x86/ARM/MIPS.
- Stack réseau avancé : VLANs 802.1q, firewall par zones, QoS, SQM, VPN.
- Interface LuCI modulaire : idéale pour créer un panneau « spectacle ».
- Packaging simple (`opkg`) pour intégrer des outils comme `ptp4l`, `igmpproxy`, `lldpd`, etc.

---

## Protocoles cibles

| Domaine | Protocoles |
|---|---|
| **Audio** | Dante, AES67 / RAVENNA, AVB / Milan, MADI over IP |
| **Vidéo** | NDI, SMPTE ST 2110, SDVoE, RTSP/RTP |
| **Lumière** | Art-Net, sACN (E1.31), MA-Net, Pathport |
| **Temps** | PTP (IEEE 1588-2008 / PTPv2), NTP |
| **Général** | DHCP, SNMP, LLDP, syslog, monitoring |

---

## Schéma de VLANs par défaut (proposition)

| VLAN ID | Nom | Usage | Priorité |
|---|---|---|---|
| `10` | `mgmt` | Administration routeur / accès SSH/HTTPS | Standard |
| `20` | `audio-dante` | Dante (Layer 3) | Haute |
| `21` | `audio-aes67` | AES67 / RAVENNA | Haute |
| `22` | `audio-avb` | AVB / Milan | Critique (TSN) |
| `30` | `video-ndihx` | NDI / NDI|HX | Très haute |
| `31` | `video-st2110` | SMPTE ST 2110 | Critique |
| `40` | `light-artnet` | Art-Net | Standard |
| `41` | `light-sacn` | sACN | Standard |
| `42` | `light-proprietary` | MA-Net / autre propiétaire | Standard |
| `50` | `ptp` | Timing PTP (idéalement séparé ou fusionné selon config) | Critique |
| `99` | `guest` | Internet / backoffice non technique | Basse |

> Ces IDs et priorités sont entièrement configurables via l'interface StageNeth.

---

## Règles inter-VLAN et flux nécessaires

Certains flux doivent traverser les VLANs pour assurer le fonctionnement du spectacle. StageNeth permet de définir des règles de routage inter-VLANs précises :

| Source VLAN | Destination VLAN | Flux autorisé | Justification |
|---|---|---|---|
| `mgmt` | Tous | HTTP/HTTPS, SSH, SNMP, Syslog | Accès management des équipements |
| `ptp` | Tous | PTP (UDP 319/320) | Synchronisation temps pour tous les équipements |
| `audio-dante` | `mgmt` | Dante Control, discovery | Configuration et monitoring des équipements Dante |
| `audio-aes67` | `mgmt` | AES67 control/monitoring | Configuration et monitoring AES67 |
| `video-ndihx` | `mgmt` | NDI Discovery, HTTP API | Configuration et monitoring NDI |
| `video-st2110` | `mgmt` | NMOS IS-04/05, HTTP API | Discovery et contrôle ST 2110 |
| `light-sacn` | `mgmt` | sACN E1.31 (multicast) | Contrôle lumière depuis console |
| `light-artnet` | `mgmt` | Art-Net (UDP 6454) | Contrôle lumière depuis console |
| `guest` | WAN uniquement | HTTP/HTTPS, DNS | Internet pour backoffice, isolation des flux techniques |

> Par défaut, les flux inter-VLANs sont bloqués sauf exceptions listées. L'interface StageNeth permet de créer des profils de règles selon le type de spectacle.

---

## Appareils à NIC unique (management + flux sur même port)

De nombreux équipements AV n'ont qu'une seule interface réseau pour :
- Le management (web UI, SSH, SNMP)
- Les flux audio/vidéo/lumière

**Solution :** port trunk avec plusieurs VLANs taggués

Exemple de configuration sur un port de switch connecté à un appareil Dante à NIC unique :

```
Port 5 (vers console Dante) :
  - VLAN 20 (audio-dante) : tagged
  - VLAN 10 (mgmt) : tagged
  - PVID : 10 (par défaut management)
```

StageNeth facilite cette configuration via :
- **Profils d'appareils** : préconfigurations pour les équipements courants (Dante, NDI, consoles lumière)
- **Assistant de port** : sélection de l'appareil → application automatique des VLANs taggués
- **VLAN natif par défaut** : `mgmt` pour garantir l'accès management

**Cas particuliers :**
- **AVB/Milan** : souvent nécessite un port dédié sans tag (TSN), mais certains équipements supportent le mode hybride
- **PTP** : doit être accessible sur tous les VLANs qui en ont besoin, donc soit VLAN dédié taggué, soit broadcast sur le VLAN mgmt

---

## Architecture cible

```
┌─────────────────────────────────────────────────────────────┐
│                       Matériel routeur                       │
│                    (x86 mini-PC / appliance)                 │
│                           StageNeth                          │
└─────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
     Trunk VLAN              WAN                 Console / MGMT
        │
   Switch PoE / L2 (avec IGMP snooping, ACLs VLAN)
        │
   ┌────┴────┬─────┬─────┬─────┬─────┬─────┬─────┐
   │ Console │ FOH │ FOH │  Lumière │  Moniteur │ Autres
   │ réseau  │ Audio │ Vidéo │        │          │
   └─────────┴─────┴─────┴─────┴─────┴─────┴─────┘
```

---

## Roadmap

1. **Analyse NethSecurity** : comprendre la build, les packages et l'interface LuCI.
2. **Définition des profils** : schémas VLANs + QoS par type de spectacle.
3. **Intégration des packages** : PTP, IGMP/MLD, LLDP, outils de monitoring.
4. **Module LuCI StageNeth** : assistant de configuration « spectacle ».
5. **Image buildable** : générer une image flashable pour matériel cible.
6. **Documentation et tests terrain**.

---

## Construction

**Approche recommandée : utiliser une image NethSecurity précompilée**

En raison de problèmes de build sur des packages upstream non liés à StageNeth, l'approche recommandée est :

1. **Télécharger une image NethSecurity précompilée** depuis les releases officielles
2. **Installer les packages StageNeth** manuellement sur l'image existante
3. **Tester les fonctionnalités** dans QEMU ou sur matériel réel

```bash
# Télécharger l'image NethSecurity précompilée (x86_64)
wget https://updates.nethsecurity.nethserver.org/stable/8.7.2/targets/x86/64/nethsecurity-8.7.2-x86-64-generic-squashfs-combined-efi.img.gz

# Décompresser
gunzip nethsecurity-8.7.2-x86-64-generic-squashfs-combined-efi.img.gz

# Tester dans QEMU
./scripts/run-qemu.sh nethsecurity-8.7.2-x86-64-generic-squashfs-combined-efi.img
```

**Installation manuelle des packages StageNeth via console série**

Dans la console QEMU (après le démarrage), appuyez sur Entrée pour activer la console, puis :

1. **Activer SSH** (optionnel) :
```bash
/etc/init.d/dropbear start
/etc/init.d/dropbear enable
```

2. **Copier-coller le script d'installation** :
   - Option 1 : Copiez et collez le contenu de `scripts/install-stageneth-simple.sh` (version complète)
   - Option 2 : Copiez et collez les commandes de `scripts/install-stageneth-step-by-step.sh` (version étape par étape, plus facile)

Le script utilise `/opt/stageneth` au lieu de `/usr/share/stageneth` pour éviter les problèmes de système de fichiers squashfs en lecture seule.

**Test de l'installation**

Après l'installation, testez avec :

```bash
stageneth-vlan-profiles.sh list
stageneth-ptp.sh list
stageneth-igmp.sh list
```

**Note** : Le partage de fichiers virtio-fs ne fonctionne pas correctement dans QEMU avec NethSecurity. L'installation doit être effectuée manuellement via la console série en copiant-collant le script d'installation.

**Approche alternative : build complet NethSecurity (expérimental)**

Pour un build complet incluant StageNeth dans l'image de base :

```bash
cd nethsecurity
./build-nethsec.sh
```

Note : Cette approche peut échouer en raison de problèmes de build sur des packages upstream non liés à StageNeth.

---

## Contribuer

StageNeth est en phase de conception. Les retours des techniciens spectacle, ingénieurs réseau et utilisateurs OpenWrt sont les bienvenus.

---

## Licence

À définir — probablement une licence open-source compatible OpenWrt (GPLv2 ou similaire) selon la base NethSecurity.
