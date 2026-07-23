# Protocoles AV supportés et recommandations

StageNeth sert de routeur de services pour les flux audios/vidéos/lumières (AV). Ce document résume les contraintes et recommandations par protocole.

## Principes généraux

- **Séparation VLAN** : chaque famille de protocoles doit être isolée pour limiter la diffusion inutile et garantir la QoS.
- **MTU par protocole** :
  - ST 2110 : **jumbo frames 9000**
  - Tous les autres (Dante, AES67, RAVENNA, NDI, Art-Net, sACN, PTP, NMOS, AVB, MA-Net...) : **1500**
  - NDI recommande explicitement de **ne pas activer** les jumbo frames.
- **Multicast/IGMP** : la plupart des protocoles AV utilisent le multicast. StageNeth **ne route pas** les flux média entre VLANs : le switch AV doit faire l'IGMP snooping/querier et, si besoin, le routage multicast (PIM/IGMP querier).
- **PTP** : StageNeth **ne fait pas Boundary Clock** ; la distribution PTP doit être assurée par les switchs AV ou un appareil dédié. Le VLAN `ptp` permet de transporter les trames PTP.
- **DSCP/QoS** : StageNeth **préserve** les marquages DSCP. Activez `trust DSCP` / `classofservice trust DSCP` sur le port trunk du switch AV. Les valeurs typiques restent configurables par service dans `/etc/config/stageneth`.
- **mDNS** : activez le refleteur `umdns` de StageNeth pour que les endpoints d'un VLAN découvrent ceux d'un autre VLAN (Dante Controller, NDI discovery).
- **NTP** : StageNeth fournit un serveur NTP par VLAN (DHCP option 42). PTP et Dante ont besoin d'une source temporelle stable.
- **EEE / offloads** : désactivez EEE et les offloads GRO/TSO/LRO/GSO sur les interfaces AV (fait automatiquement par `stageneth-network`).

## Tableau récapitulatif

| Protocole | MTU | Multicast | PTP | DSCP indicatif | Notes |
|---|---|---|---|---|---|
| ST 2110 | 9000 | Oui, transport vidéo | Non (PTP sur VLAN `ptp`) | Vidéo/audio AF41 (34) | Nécessite une interface physique dédiée (`eth2` par défaut) et un switch supportant les jumbos. StageNeth ne route pas les flux ST 2110. |
| Dante | 1500 | Oui | Oui | Audio AF41 (34), contrôle/PTP EF (46) | Nécessite IGMP snooping/querier. PTP transparent sur le VLAN `ptp` ou Dante Domain Manager. |
| AES67 / RAVENNA | 1500 | Oui | Oui | Média AF41 (34), PTP EF (46) | Compatible Dante en mode AES67. PTP IEEE 1588-2008 obligatoire. |
| NDI | 1500 | Oui (multicast possible) | Non | AF41 (34) ou CS4 (32) selon modèle | **Ne pas utiliser de jumbo frames.** NDI|HX utilise Bonjour/mDNS. |
| Art-Net | 1500 | Oui | Non | CS7 (56) ou Best Effort | Flux DMX. Très sensible à la latence. |
| sACN | 1500 | Oui | Non | CS7 (56) ou Best Effort | Diffusion multicast E1.31. |
| PTP | 1500 | Adresse 224.0.1.129 / 224.0.0.107 | Oui | EF (46) | VLAN dédié `ptp` ou transport via switch AV. StageNeth ne joue pas le rôle de Boundary Clock. |
| NMOS IS-04/IS-05 | 1500 | Oui (mDNS/HTTP) | Non | Best Effort ou AF33 (30) | Découverte via mDNS. Nécessite le refleteur `umdns` inter-VLAN. |
| AVB (IEEE 802.1BA) | 1500 | Oui | Oui (gPTP) | SR class A/B | StageNeth ne fournit pas de fonction AVB ; configurez les switchs AVB compatibles. |
| MA-Net / Proprietary | 1500 | Oui | Non | Dépend du constructeur | Utilisez le service `proprietary` pour les protocoles non standards. |
| Guest / Mgmt | 1500 | Non | Non | Best Effort | VLAN de gestion ou invité, pas de routage vers les VLANs média. |

## Configuration StageNeth

Les paramètres sont stockés dans `/etc/config/stageneth` (section `service`) :

- `vlan_id` : identifiant VLAN
- `dscp` : valeur DSCP en décimal
- `priority` : priorité 802.1p (cos)
- `mtu` : MTU (`9000` pour ST 2110, `1500` par défaut)
- `ptp` : `1` si le VLAN transporte du PTP
- `multicast` : `1` si le VLAN utilise du multicast
- `untagged` : `1` pour envoyer le VLAN untagged sur une interface

Exemple :

```uci
config service 'st2110'
    option vlan_id '10'
    option dscp '34'
    option priority '5'
    option mtu '9000'
    option ptp '0'
    option multicast '1'
```

## Vérifications rapides

- `ip link show eth1.<vlan>` : VLAN up
- `ip addr show svc_<service>` : interface service
- `ethtool --show-eee eth1` : EEE désactivé
- `tcpdump -i eth1.<vlan> -v` : DSCP visible
- `ubus call umdns browse` : services mDNS annoncés
