# Test et validation de l'image StageNeth

## Tests automatisés

```bash
# Tests unitaires Go (moteur, API, parsers, auth)
cd packages/stagenethd/files && go test -count=1 ./...

# E2E réel : démarre stageneth-api + stagenethd sur une arborescence
# isolée (aucune dépendance OpenWrt nécessaire) et vérifie le flux
# complet : wizard → session → ressources v1 → apply → rollback → logout
tests/e2e/run.sh

# E2E HTTPS : la pile complète docker nginx:alpine → socket unix → API,
# avec les fichiers *.locations RÉELS du paquet (SSI, CSP, redirection
# HTTP→HTTPS, refus API en clair, SSE non bufferisé). Vérifie aussi
# l'authentification, les rôles et une trame SSE effective.
# Requiert docker ; KEEP=1 conserve l'environnement lancé.
tests/e2e-https/run.sh
```

Le harnais E2E utilise les variables `STAGENETH_CONFIG_DIR`,
`STAGENETH_STATE_DIR`, `STAGENETH_INIT_DIR`, `STAGENETH_SOCKET`,
`STAGENETHD_BIN` et `STAGENETH_RSYSLOG_CONF` pour rediriger tous les
chemins système vers un répertoire temporaire ; les scripts init.d sont
remplacés par des stubs qui journalisent les rechargements demandés.

## Lancer l'image avec QEMU

Remplacez `0.2.0-beta.19` par la version de `build.conf` (ou prenez les images dans les [GitHub Releases](https://github.com/dewiweb/stageneth/releases)) :

```bash
gunzip -c bin/stageneth-0.2.0-beta.19-x86-64-generic-ext4-combined.img.gz > /tmp/stageneth-test.img
qemu-system-x86_64 -m 1024 -smp 2 -enable-kvm \
  -hda /tmp/stageneth-test.img -display none -serial file:/tmp/stageneth-qemu.log \
  -netdev user,id=net0,net=192.168.1.0/24,hostfwd=tcp::2222-192.168.1.1:22,hostfwd=tcp::8443-192.168.1.1:443 \
  -device e1000,netdev=net0
```

## Accès

- **UI** : `https://192.168.1.1/`
  Avec le forward QEMU : `https://localhost:8443/`
- **SSH** : `ssh -p 2222 root@localhost` — le mot de passe root/console
  est celui choisi dans le wizard de premier démarrage (il provisionne
  le compte web `admin` et le compte console en même temps). SSH reste
  inutilisable tant que le wizard n'a pas été complété.

## Vérifications générales

- `nginx`, `stageneth-api` et `stagenethd` sont démarrés (`ps w`, `logread -f`)
- L'API `/api/v1/session` (POST) retourne un cookie de session HttpOnly
- Les endpoints `/api/v1/interfaces`, `/api/v1/ntp`, `/api/v1/logs` et `/api/v1/monitoring` répondent
- La bannière et le hostname affichent `StageNeth`
- Le wizard de premier démarrage permet de définir le mot de passe et d'activer les services

## Plan de validation banc AV

### 1. VLAN et trunking

| Action | Commande / Test | Attendu |
|---|---|---|
| Vérifier le trunk | `cat /etc/config/network | grep device` | `eth1` en 8021q, VLANs tagués |
| Vérifier un VLAN | `ip link show eth1.<vlan>` | VLAN créé, UP |
| Vérifier les bridges | `brctl show` ou `ip link show br_*` | Un bridge par service avec le bon VLAN en port |
| Vérifier le MTU parent | `ip link show eth1` et `ip link show eth2` | `eth1` à 1500, `eth2` à 9000 si ST 2110 |
| Vérifier les interfaces services | `ip addr show svc_*` | IP `10.<vlan>.0.1/24` |

### 2. DHCP et NTP

| Action | Commande / Test | Attendu |
|---|---|---|
| DHCP pool actif | `cat /etc/config/dhcp | grep svc_` | Une section `dhcp` par service |
| Option 42 NTP | `grep dhcp_option /etc/config/dhcp` | `42,10.<vlan>.0.1` par pool |
| Lease test | Connecter un endpoint sur le VLAN Dante | Obtient `10.<vlan>.0.100+` |
| NTP serveur | `uci show system.ntp` | `enable_server='1'`, pools configurés |
| Sync NTP | `ubus call system info` ou `ntpd` | Temps synchronisé |

### 3. PTP

- StageNeth **ne fait pas Boundary Clock**. Le PTP doit rester sur le switch AV.
- Vérifier que le port trunk permet les trames PTP multicasts `224.0.1.129` / `224.0.0.107`.
- Si un grandmaster est présent, les endpoints du VLAN `ptp` doivent se synchroniser.

### 4. Multicast et IGMP

| Action | Test | Attendu |
|---|---|---|
| Multicast entre VLANs | Flux Dante/AES67 d'un VLAN vers un autre | Refusé (pas de routage multicast) |
| Multicast dans un VLAN | Récepteur + émetteur dans le même VLAN | Passant si le switch AV gère l'IGMP snooping/querier |
| IGMP | `cat /proc/net/igmp` | Groupes visibles si actif |

### 5. DSCP / QoS

| Action | Commande | Attendu |
|---|---|---|
| DSCP préservé | `tcpdump -i eth1.<vlan> -v` sur un flux | Valeur DSCP d'origine inchangée (pas de rewrite) |
| Pas de NAT media | `nft list ruleset | grep masq` | Pas de `masq` sur les zones `svc_*` |

### 6. MTU

| Protocole | MTU | Test |
|---|---|---|
| ST 2110 | 9000 | `ping -M do -s 8972 <ip>` depuis un endpoint jumbo |
| Dante / NDI / AES67 / Art-Net / sACN / PTP | 1500 | `ping -s 1472 <ip>` OK, ping jumbo échoue (volontaire) |
| NDI | 1500 | Confirmer que les paquets NDI restent à 1500 |

### 7. Firewall

| Action | Test | Attendu |
|---|---|---|
| Inter-VLAN | Depuis un VLAN media, pinguer un autre VLAN media | Refusé (forward REJECT) |
| DHCP/DNS/NTP local | Depuis un endpoint, `dig @10.<vlan>.0.1`, `ntpdate -q` | Accepté |
| WAN/Internet | Si WAN configuré | NAT sur zone `wan` seulement |

### 8. mDNS / découverte

| Action | Test | Attendu |
|---|---|---|
| mDNS reflector | `ubus call umdns browse` | Services Dante/NDI visibles depuis d'autres VLANs |
| Découverte Dante Controller | Ouvrir Dante Controller sur un PC | Périphériques du VLAN Dante visibles |

### 9. Supervision

| Action | Test | Attendu |
|---|---|---|
| Monitoring UI | Onglet `Monitoring` | CPU, mémoire, DHCP leases, services UP/DOWN, alertes |
| Logs | Onglet `Logs` | Logs multi-sources (logread, rsyslog, nginx) triés par timestamp, filtrables et colorisés en temps réel |
| SNMP | `snmpwalk -v2c -c public <ip> 1.3.6.1.2.1.1` | Réponse système |
| Collecteur syslog | Activer le collecteur (Système → Collecteur syslog) puis envoyer un log UDP 514 depuis un switch | Fichier `/var/log/remote/<hôte>.log` |
| Relais syslog | Activer l'envoi distant (Système → Journal distant) vers un collecteur | Le collecteur distant reçoit les journaux (omfwd) |
| Métriques Prometheus | `curl http://<ip>/api/metrics` (IP privée requise) | Format exposition 0.0.4 (`stageneth_*`) |

## En cas de problème

- Relancer l'orchestrateur : `/etc/init.d/stagenethd restart`
- Revenir à la dernière configuration fonctionnelle : `stagenethd rollback`
- Redémarrer le refleteur mDNS : `/etc/init.d/umdns restart`
- Vérifier la configuration UCI : `uci show stageneth`, `uci show network`, `uci show dhcp`, `uci show firewall`
- Consulter les logs : `logread -f` et l'onglet `Logs` de l'UI
