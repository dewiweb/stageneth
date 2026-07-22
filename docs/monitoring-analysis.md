# Analyse du monitoring NethSecurity et propositions StageNeth

## Monitoring actuel de NethSecurity

### Architecture
```
Netifyd → ns-flows (Go) → Victoria Metrics
System → Telegraf → Victoria Metrics
Victoria Metrics → vmalert → Alertes → ns-plug-alert-proxy → my.nethesis.it
```

### Composants

#### 1. ns-monitoring (Go)
- **ns-flows** : Analyse des flux réseau via netifyd
  - Détection des applications et protocoles
  - Statistiques de trafic par application
  - Configuration dans `/etc/config/ns-flows`
  
- **ns-stats** : Statistiques système
  - Agrégation de métriques système
  - Configuration dans `/etc/config/ns-stats`

#### 2. Telegraf
Collecte de métriques via plugins :
- **Système** : CPU, mémoire, disque, réseau (inputs standards)
- **Services** : État des services via ubus (`/usr/libexec/telegraf-services`)
- **Multi-WAN** : État des interfaces WAN via mwan3 (`/usr/libexec/telegraf-mwan`)
- **Stockage** : État du stockage (`/usr/libexec/telegraf-storage-status`)

Services monitorés : nginx, dnsmasq, netifyd, ns-api-server, ns-ui, etc.

#### 3. Victoria Metrics
- **Base de données time-series** : Port 8428
- **vmalert** : Évaluation des règles d'alerte, port 8081
- **Règles d'alerte** : YAML dans `/etc/vmalert/rules/*.yaml`
- **Modèle de sévérité** : Warning/Critical avec suppression du warning si critical actif
- **Intégration** : Forwarding vers my.nethesis.it (Mimir) via ns-plug-alert-proxy

#### 4. ns-plug-alert-proxy
- Proxy pour forwarding des alertes
- Filtre les alertes legacy pour my.nethesis.it
- Support des alertes HA (failover, sync)

---

## Besoins spécifiques au spectacle vivant

### Contexte
- **Temps réel critique** : Problèmes réseau = arrêt du spectacle
- **Protocoles AV sensibles** : Dante, AES67, NDI, sACN nécessitent monitoring spécifique
- **Équipements professionnels** : Consoles, stage boxes, cameras, PTP grandmaster
- **Personnel non-IT** : Techniciens spectacle, pas d'experts réseau
- **Environnement changeant** : Setup/démontage quotidien, différents types d'événements

---

## Propositions de monitoring StageNeth

### 1. Package stageneth-ptp-monitor

**Fonctionnalités :**
- Monitoring de l'état PTP (via linuxptp/ptp4l)
- Métriques collectées :
  - PTP offset (ns)
  - PTP jitter (ns)
  - PTP path delay (ns)
  - PTP clock class
  - PTP port state (master/slave/listening)
  - Nombre de PTP masters détectés

- Alertes :
  - `PTPOffsetHigh` : Offset > 1μs pour 5min (warning), > 10μs (critical)
  - `PTPGrandmasterDown` : Grandmaster PTP inaccessible
  - `PTPUnsynced` : Clock class invalide ou non synchronisée
  - `PTPPathDelayHigh` : Path delay anormal

**Implementation :**
- Script Python utilisant `pmc` (PTP Management Client) de linuxptp
- Intégration Telegraf via plugin exec
- Règles vmalert dans `/etc/vmalert/rules/stageneth-ptp.yaml`

### 2. Package stageneth-dante-monitor

**Fonctionnalités :**
- Monitoring des devices Dante via Dante Controller API
- Métriques collectées :
  - Nombre de devices Dante actifs
  - Latence Dante par device
  - Packet loss Dante
  - État des subscriptions Dante
  - Bande passante audio Dante

- Alertes :
  - `DanteDeviceDown` : Device Dante déconnecté
  - `DanteLatencyHigh` : Latence > seuil configurable
  - `DantePacketLoss` : Packet loss détecté
  - `DanteSubscriptionLost` : Subscription audio perdue

**Implementation :**
- Script Python utilisant Dante Controller API (si disponible) ou Dante Via API
- Intégration Telegraf
- Dashboard LuCI spécifique Dante

### 3. Package stageneth-ndi-monitor

**Fonctionnalités :**
- Monitoring des flux NDI via NDI Discovery
- Métriques collectées :
  - Nombre de sources NDI actives
  - Bande passante NDI par source
  - Latence NDI
  - État des encodeurs/décodeurs NDI

- Alertes :
  - `NDISourceDown` : Source NDI disparue
  - `NDIBandwidthHigh` : Bande passante > capacité VLAN
  - `NDILatencyHigh` : Latence > seuil

**Implementation :**
- Script Python utilisant NDI Tools ou parsing de mDNS
- Intégration Telegraf
- Dashboard LuCI spécifique NDI

### 4. Package stageneth-sacn-monitor

**Fonctionnalités :**
- Monitoring sACN/Art-Net via sniffing multicast
- Métriques collectées :
  - État des consoles sACN (detected via source IP)
  - Latence DMX (via timestamps sACN)
  - Taux de paquets sACN par seconde
  - Détection de collision Art-Net

- Alertes :
  - `sACNConsoleDown` : Console sACN disparue
  - `sACNPacketLoss` : Perte de paquets sACN
  - `ArtNetCollision` : Collision Art-Net détectée

**Implementation :**
- Script Python utilisant scapy pour sniffing multicast
- Intégration Telegraf
- Dashboard LuCI spécifique lighting

### 5. Package stageneth-vlan-monitor

**Fonctionnalités :**
- Monitoring des VLANs spécifiques StageNeth
- Métriques collectées :
  - État des interfaces VLAN (up/down)
  - Traffic par VLAN (bande passante, packets/sec)
  - Nombre de devices connectés par VLAN (via ARP)
  - Erreurs par VLAN

- Alertes :
  - `VLANDown` : Interface VLAN down
  - `VLANBandwidthHigh` : Bande passante > 80% capacité
  - `VLANDeviceCountChanged` : Variation significative du nombre de devices

**Implementation :**
- Script Python utilisant `/proc/net/vlan/` et commandes bridge
- Intégration Telegraf
- Dashboard LuCI avec graphiques par VLAN

### 6. Dashboard LuCI StageNeth Monitoring

**Fonctionnalités :**
- Page "Monitoring" dans menu StageNeth
- Vue globale avec indicateurs de santé :
  - 🟢 PTP : Synchronisé (offset: 125ns)
  - 🟢 Dante : 12 devices actifs
  - 🟢 NDI : 4 sources actives
  - 🟢 Lighting : Console connectée
  - 🟢 VLANs : Tous up

- Graphiques temps réel :
  - PTP offset sur 1h
  - Bande passante par VLAN
  - Latence Dante/NDI
  - Packet loss par protocole

- Alertes actives avec contexte :
  - "⚠️ Console Dante FOH déconnectée depuis 2min"
  - "🔴 PTP offset élevé (15μs) - vérifier grandmaster"

- Interface simplifiée pour techniciens :
  - Terminologie spectacle (FOH, stage, console, etc.)
  - Actions rapides (redémarrer service, voir logs)
  - Export de rapport pour show report

### 7. Package stageneth-show-report

**Fonctionnalités :**
- Génération de rapports de monitoring par événement
- Données collectées :
  - Statistiques PTP (min/max/avg offset)
  - Statistiques Dante (devices, latency, packet loss)
  - Statistiques NDI (sources, bandwidth)
  - Incidents et alertes survenues
  - Graphiques de performance

- Export formats :
  - PDF (pour documentation)
  - JSON (pour intégration)
  - CSV (pour analyse)

- Interface LuCI :
  - Sélection de période (début/fin show)
  - Génération de rapport
  - Historique des rapports

---

## Architecture proposée

```
StageNeth-specific monitors:
├── stageneth-ptp-monitor (Python)
├── stageneth-dante-monitor (Python)
├── stageneth-ndi-monitor (Python)
├── stageneth-sacn-monitor (Python)
├── stageneth-vlan-monitor (Python)
└── stageneth-show-report (Python)
     │
     ▼
Telegraf (exec plugins)
     │
     ▼
Victoria Metrics
     │
     ▼
vmalert (règles StageNeth)
     │
     ▼
LuCI Dashboard StageNeth
```

---

## Priorités d'implémentation

### Phase 1 (Essentiel)
1. **stageneth-ptp-monitor** : PTP critique pour audio/video
2. **stageneth-vlan-monitor** : Base pour monitoring réseau
3. **Dashboard LuCI** : Visualisation pour techniciens

### Phase 2 (Important)
4. **stageneth-dante-monitor** : Dante très répandu
5. **stageneth-sacn-monitor** : Lighting critique

### Phase 3 (Avancé)
6. **stageneth-ndi-monitor** : NDI moins critique que Dante
7. **stageneth-show-report** : Documentation post-show

---

## Intégration avec NethSecurity existant

### Réutilisation des composants
- **Telegraf** : Ajouter plugins exec pour les scripts StageNeth
- **Victoria Metrics** : Stocker les métriques StageNeth
- **vmalert** : Ajouter règles `/etc/vmalert/rules/stageneth-*.yaml`
- **LuCI** : Nouveau menu "StageNeth Monitoring"

### Configuration
- Activer ns-monitoring dans build.conf
- Ajouter dépendances : python3-scapy, python3-requests, linuxptp
- Configurer netifyd pour détecter Dante/NDI/sACN

### Alertes
- Adapter les alertes legacy pour contexte spectacle
- Ajouter sévérité "show-critical" (arrêt du spectacle)
- Suppression intelligente (ex: PTP high offset si Dante down)

---

## Exemple de règle vmalert StageNeth

```yaml
# /etc/vmalert/rules/stageneth-ptp.yaml
groups:
  - name: "stageneth_ptp"
    interval: "1m"
    rules:
      - alert: PTPOffsetHigh
        expr: 'ptp_offset_nanoseconds > 1000'
        for: "5m"
        labels:
          severity: "warning"
          category: "ptp"
        annotations:
          summary_en: "PTP offset elevated"
          description_en: "PTP offset is {{ $value }}ns, expected < 1μs"
          
      - alert: PTPOffsetCritical
        expr: 'ptp_offset_nanoseconds > 10000'
        for: "2m"
        labels:
          severity: "critical"
          category: "ptp"
          show_critical: "true"
        annotations:
          summary_en: "PTP offset critical - audio sync at risk"
          description_en: "PTP offset is {{ $value }}ns, audio/video sync may be affected"
```

---

## Prochaine étape

Créer le package `stageneth-ptp-monitor` comme premier composant de monitoring spécifique au spectacle vivant.
