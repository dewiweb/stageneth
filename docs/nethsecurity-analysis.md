# Analyse NethSecurity — Base technique pour StageNeth

> Analyse du projet [NethSecurity](https://github.com/NethServer/nethsecurity) pour comprendre comment construire StageNeth.

---

## Vue d'ensemble

NethSecurity est un **downstream rebuild d'OpenWrt** avec une interface web LuCI modifiée, conçu comme firewall pour x86_64.

- **Licence** : GPL-2.0
- **Cible principale** : x86_64
- **Base OpenWrt** : v25.12.5 (dans build.conf.defaults)
- **Version NethSecurity** : 8.8.0
- **Build system** : Podman + Containerfile

---

## Structure du projet

```
nethsecurity/
├── build-nethsec.sh          # Script principal de build
├── build.conf.defaults       # Configuration par défaut
├── builder/                  # Containerfile et scripts de build
│   ├── Containerfile
│   ├── apply-patches.sh
│   ├── configure-build.sh
│   └── entrypoint.sh
├── config/                   # Fichiers de configuration des composants
│   ├── luci.conf            # Configuration LuCI
│   ├── targets/             # Cibles de build
│   └── *.conf               # Configs par package (nginx, dnsmasq, etc.)
├── packages/                # Packages NethSecurity custom
│   ├── ns-ui               # Interface web NethSecurity
│   ├── ns-api-server       # API REST
│   ├── ns-monitoring       # Monitoring
│   └── ...                 # Autres packages ns-*
├── files/                   # Structure des fichiers système
│   ├── bin/
│   ├── etc/
│   └── usr/
├── patches/                 # Patches à appliquer à OpenWrt
└── scripts/                 # Scripts utilitaires
```

---

## Build system

### Configuration par défaut (`build.conf.defaults`)

```bash
OWRT_VERSION=v25.12.5
NETHSECURITY_VERSION=8.8.0
BUILD_SEMVER_SUFFIX=-beta.3
TARGET=x86_64
```

### Processus de build (`build-nethsec.sh`)

1. Charge `build.conf.defaults` puis `build.conf` (overrides locaux)
2. Construit l'image Podman avec le Containerfile
3. Lance le conteneur avec volumes persistants (cache, build_dir, staging_dir)
4. Exécute la commande passée (par défaut `make`)
5. Copie les résultats dans `bin/` et `build-logs/`

### Variables d'environnement clés

- `OWRT_VERSION` : version d'OpenWrt
- `NETHSECURITY_VERSION` : version du produit
- `REPO_CHANNEL` : dev/stable
- `TARGET` : architecture cible
- `BUILD_SEMVER_SUFFIX` : suffixe de version (beta, rc, etc.)

---

## Packages

### Packages NethSecurity (custom)

Les packages NethSecurity sont dans `packages/` avec le préfixe `ns-` :

- `ns-ui` : Interface web NethSecurity (LuCI modifiée)
- `ns-api-server` : API REST pour gestion
- `ns-api` : Client API
- `ns-monitoring` : Monitoring (Telegraf, Victoria Metrics)
- `ns-netmap` : Cartographie réseau
- `ns-openvpn` : OpenVPN intégré
- `ns-ha` : High availability (Keepalived)
- `ns-dpi` : Deep packet inspection
- `ns-storage` : Gestion stockage
- `ns-clm` : Central License Manager
- Et de nombreux autres...

**Convention** : 
- Nom : `ns-<nom>`
- CATEGORY : `NethSecurity`
- SECTION : `base`

### Packages OpenWrt

Les packages OpenWrt sont inclus via `feeds.conf.default` avec des commits figés :

```
src-git packages https://git.openwrt.org/feed/packages.git^<commit>
src-git luci https://git.openwrt.org/project/luci.git^<commit>
src-git routing https://git.openwrt.org/feed/routing.git^<commit>
src-link nethsecurity /home/buildbot/openwrt/nspackages
```

**Important** : Les versions des feeds OpenWrt sont figées par commit, pas par branche. Cela garantit la reproductibilité des builds.

---

## Configuration

### Fichiers de configuration dans `config/`

Chaque package a son fichier `.conf` qui définit s'il est inclus dans l'image :

```
CONFIG_PACKAGE_ns-ui=y
CONFIG_PACKAGE_ns-api-server=y
CONFIG_PACKAGE_luci-base=y
...
```

### Cibles de build dans `config/targets/`

Probablement définit les profils de build (generic, virt, etc.).

---

## Interface utilisateur

### LuCI

NethSecurity utilise LuCI (interface web OpenWrt) avec des modifications via `ns-ui`.

**Configuration** : `config/luci.conf`

### API REST

Package `ns-api-server` expose une API REST pour la gestion programmatique.

---

## Stratégie pour StageNeth

### Approche 1 : Fork NethSecurity

**Avantages** :
- Base solide déjà configurée
- Interface web prête
- Build system fonctionnel
- Packages réseau déjà intégrés

**Inconvénients** :
- Dépendance forte à NethSecurity
- Plus difficile de revenir à OpenWrt pur

### Approche 2 : Fork OpenWrt direct

**Avantages** :
- Indépendance totale
- Contrôle complet de la stack
- Plus léger

**Inconvénients** :
- Plus de travail initial
- Interface web à construire

### Recommandation : Fork NethSecurity

Pour StageNeth, je recommande de **forker NethSecurity** car :

1. Il a déjà une interface web (LuCI + ns-ui) que nous pouvons adapter
2. Le build system est mature et documenté
3. Les packages réseau sont déjà intégrés
4. Nous pouvons ajouter nos propres packages `stageneth-*` dans `packages/`

### Étapes pour StageNeth basé sur NethSecurity

1. **Fork du repo NethSecurity**
2. **Renommage branding** (`config/branding.conf`)
3. **Ajout de packages StageNeth** dans `packages/` :
   - `stageneth-vlan-profiles` : Profils VLAN par type de spectacle
   - `stageneth-ptp` : Configuration PTP avancée
   - `stageneth-igmp` : Configuration IGMP/MLD
   - `stageneth-luci-module` : Module LuCI pour interface spectacle
4. **Modification de la configuration par défaut** :
   - VLANs préconfigurés
   - Règles firewall inter-VLAN
   - QoS par protocole
5. **Build d'image personnalisée** pour x86_64

---

## Packages OpenWrt utiles pour StageNeth

Depuis les feeds OpenWrt, les packages suivants seront nécessaires :

- **PTP** : `linuxptp` (ptp4l, phc2sys, pmc, etc.)
- **Multicast** : `igmpproxy`, `smcroute`, `mld-proxy`
- **LLDP** : `lldpd`
- **QoS** : `tc`, `qosify`, `sqm-scripts`
- **Monitoring** : `collectd`, `prometheus-node-exporter`
- **SNMP** : `snmpd`
- **Timing** : `chrony` (NTP)
- **Diagnostic** : `iperf3`, `tcpdump`, `wireshark-cli`

---

## Prochaine étape

Cloner le repo NethSecurity localement et explorer la structure détaillée des packages et de la configuration pour préparer le fork StageNeth.
