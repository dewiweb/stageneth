# Matériel — notes par plateforme

## MikroTik RB5009U(G/P)+S+IN

La meilleure cible pro/rackable : ~220 EUR, fanless, kit rack 19″,
7× 1GbE + 1× 2.5GbE + 1× 10G SFP+. La variante **UP+S+IN** ajoute le
PoE-out (peut alimenter des endpoints Dante/AES67 directement).

### Topologie des ports sous OpenWrt

Tous les ports passent par le switch interne Marvell 88E6393X (DSA) :

| Interface Linux | Port physique | Débit |
|---|---|---|
| `p1` | ether1 | 2.5GbE |
| `p2`–`p8` | ether2–ether8 | 1GbE |
| `sfp` | cage SFP+ | 10G |

Le port CPU est `eth0` (lien 10gbase-r interne vers le switch — ne pas
le mettre dans un bridge). Il n'y a **pas** de `eth1` sur cette carte :
le trunk StageNeth doit référencer `p1`…`p8` ou `sfp`.

Exemple de config `/etc/config/stageneth` typique RB5009 :

```
config stageneth 'globals'
    option trunk 'sfp'          # uplink 10G vers le cœur AV

config binding 'mgmt_port'
    option interface 'p8'       # ether8
    option service 'mgmt'
```

- `sfp` en trunk = uplink fibre/cuivre 10G vers le switch cœur —
  idéal pour les agrégats ST 2110.
- `p1` (2.5G) peut servir de second trunk ou de port d'admin.
- Les bridges `br_<service>` en 802.1Q fonctionnent via DSA sans
  configuration supplémentaire ; l'écoute passive des domaines
  d'horloge (`clock_domains`) fonctionne sur ces bridges.

### Flashage

RB5009 n'a pas d'image disque « combined » — l'installation passe par
le bootloader réseau RouterBOOT :

1. Servir `stageneth-*-mikrotik_rb5009-initramfs-kernel.bin` par TFTP,
   avec BOOTP/DHCP configuré (RouterBOOT netboot : maintenir RESET à
   la mise sous tension, ou `bootp` depuis la console série 115200).
2. Le système démarre en RAM. Vérifier `ip link` → `p1`…`p8`, `sfp`.
3. `sysupgrade -n` avec `stageneth-*-mikrotik_rb5009-squashfs-sysupgrade.bin`
   (flash NAND UBI — définitif).
4. Premier boot : wizard HTTPS sur l'IP de gestion.

> ⚠️ Le flash via sysupgrade écrase RouterOS. Le retour arrière se
> fait par netinstall MikroTik (TFTP), pas par un bouton magique.

### État du support

Profil `mikrotik_rb5009` upstream en 24.10 (mvebu/cortexa72, arm64).
Points à valider sur le banc : SFP+ (selon le module inséré), PoE-out
sur la variante UP, performances de bridge DSA sous charge multicast
réelle.

## PC industriels DIN-rail (x86-64)

Tout PC industriel fanless ou DIN-rail x86-64 (OnLogic, Beckhoff CX,
Axiomtek ICO, Moxa, Siemens SIMATIC IOT…) exécute l'image `x86_64`
telle quelle — c'est la voie « industrielle » sans effort. Choisir un
modèle avec ≥ 2 NICs Intel (i210/i211/i226 de préférence).
