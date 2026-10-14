# Build StageNeth

## Prérequis

- [Podman](https://podman.io/) (testé avec Podman, compatible Docker)
- Espace disque : l'arbre OpenWrt occupe plusieurs Go
- Espace de build dans `./.build` (surcharge : `STAGENETH_BUILD_ROOT`)

## Build conteneurisée (recommandée)

Depuis la racine du projet :

```bash
./build-stageneth.sh
```

Le script :
- construit l'image `stageneth-next` à partir de `builder/Containerfile`
- lance un conteneur qui compile OpenWrt avec les packages StageNeth
- copie les images résultantes dans `bin/` à la racine

La configuration est dans `build.conf.defaults` et peut être surchargée dans `build.conf`.

## Cibles (`TARGET`)

`TARGET` désigne un fichier de `config/targets/` et peut être passé en
variable d'environnement ou dans `build.conf` :

```bash
TARGET=nanopi_r5s ./build-stageneth.sh
```

| `TARGET` | Matériel | Ports | Remarques |
|---|---|---|---|
| `x86_64` | PC / mini-PC / VM (défaut) | selon machine | image GRUB BIOS + EFI |
| `armsr` | ARM64 générique EFI, QEMU `virt` | selon machine | équivalent arm64 du x86_64 générique |
| `nanopi_r4s` | NanoPi R4S (RK3399) | 2× 1GbE | banc minimal, souvent d'occasion |
| `nanopi_r5s` | NanoPi R5S (RK3568S) | 2× 2.5G + 1G | excellent rapport prix/ports |
| `nanopi_r6s` | NanoPi R6S (RK3588S) | 2× 2.5G + 1G | plus de CPU pour les flux multicast |
| `rpi4` | Raspberry Pi 4 / CM4 | 1G + USB NIC | RTL8152/AX88179 inclus (base.conf) |
| `bpi_r3` | Banana Pi BPI-R3 (Filogic 830) | 4× 1G + 2× SFP | vraie topologie routeur AV |
| `bpi_r4` | Banana Pi BPI-R4 (Filogic 880) | 4× 1G + 2× SFP+ 10G | uplinks fibre ST 2110 |
| `glinet_mt6000` | GL.iNet Flint 2 (MT7986) | 2× 2.5G + 4× 1G | meilleur routeur grand public (~130 EUR) |
| `glinet_mt3000` | GL.iNet Beryl AX (MT7981) | 2.5G + 1G | banc minimal de poche |
| `xiaomi_ax3000t` | Xiaomi AX3000T (MT7981) | 4× 1G | le moins cher du marché (~40 EUR) |
| `linksys_e8450` | Linksys E8450 / Belkin RT3200 (MT7622) | 4+1× 1G | classique d'occasion |
| `dynalink_dl-wrx36` | Dynalink DL-WRX36 (IPQ8072A) | 4+1× 1G | quad-core, marché US |

### Professionnel / industriel

| `TARGET` | Matériel | Ports | Remarques |
|---|---|---|---|
| `mikrotik_rb5009` | MikroTik RB5009U(G/P)+S+IN | 7×1G + 2.5G + 10G SFP+ | fanless, kit rack 19″ ; la variante PoE alimente des endpoints Dante |
| `mikrotik_760igs` | MikroTik hEX S (MT7621) | 5×1G + SFP | ~60 EUR, banc pro entrée de gamme (mipsle) |
| `zyxel_xgs1250` | Zyxel XGS1250-12 (RTL9302B) | 8×1G + 3×2.5G + SFP+ | **switch géré** — StageNeth tourne dans le switch AV lui-même |
| `hpe_1920_48g_poe` | HPE 1920-48G-PoE+ (RTL8393) | 48×1G PoE + 4×SFP | switch d'accès entreprise 48 ports |
| `panasonic_m48eg` | Panasonic M48EG-PN28480K | 48×1G + 4×SFP+ | switch broadcast AVoIP (écosystème KAIROS) |

Tout PC industriel fanless/DIN-rail x86-64 (OnLogic, Beckhoff, Axiomtek,
Moxa…) exécute directement l'image `x86_64` — aucune cible dédiée n'est
nécessaire. MikroTik RB5009 = la meilleure option pro/rackable (~220 EUR).
Voir [docs/hardware.md](hardware.md) pour la topologie des ports RB5009
(`p1`–`p8`, `sfp`) et la procédure de flashage BOOTP/TFTP.

⚠️ Les cibles `realtek` (switches managés) sont supportées par OpenWrt
24.10 mais la couche switch est plus jeune — à valider sur banc réel
avant production.

Le daemon Go est `CGO_ENABLED=0` et n'utilise que des API Linux portables
(AF_PACKET, netlink, sysfs) : aucune modification de code n'est requise
pour une nouvelle cible.

## Routeurs OpenWrt existants (sans image dédiée)

Les paquets `stagenethd` / `stageneth-ui` ne dépendent que de paquets des
feeds officiels (`firewall4`, `nginx-ssl`, `linuxptp`, `umdns`,
`mdns-repeater`, `snmpd`, `rsyslog`, `ethtool`, `ip-bridge`,
`mdnsresponder`, `tcpdump`). Le build produit les `.ipk` dans
`bin/packages/<arch>/` : ils s'installent donc sur **n'importe quel
routeur supporté par OpenWrt 24.10** de la même architecture, sans image
dédiée :

```bash
opkg update && opkg install ./stagenethd_*.ipk ./stageneth-ui_*.ipk
```

Mises en garde :

- StageNeth **prend le contrôle** de `/etc/config/network`, `firewall`,
  `dhcp`, `rsyslog`, `umdns` et `mdns_repeater` — le routeur devient une
  appliance StageNeth, la configuration existante est remplacée à
  l'application.
- nginx écoute sur 80/443 → désactiver uhttpd/LuCI
  (`/etc/init.d/uhttpd disable; /etc/init.d/uhttpd stop`) sinon conflit
  de ports.
- Il faut au moins 2 ports Ethernet distincts (mgmt + trunk). Sur un
  switch DSA interne (`lan1`…`lan4`, `wan`), un port `wan`/`lanX` peut
  porter le trunk 802.1Q — mais pour du vrai transit AV, préférez des
  ports PHY séparés (GL-MT6000, BPI-R3) ou un switch géré en amont.

### Ajouter une cible

1. Créer `config/targets/<nom>.conf` avec le triplet OpenWrt :

   ```
   CONFIG_TARGET_<target>=y
   CONFIG_TARGET_<target>_<subtarget>=y
   CONFIG_TARGET_<target>_<subtarget>_DEVICE_<profile>=y
   ```

   (le nom exact du device profile se trouve dans
   `target/linux/<target>/image/` du source OpenWrt)
2. `TARGET=<nom> ./build-stageneth.sh` — les images arrivent dans `bin/targets/`.
3. Référencer le nom dans la liste `workflow_dispatch` de
   `.github/workflows/build.yml` pour un build CI à la demande.

Un `TARGET` sans fichier correspondant fait échouer le build immédiatement
(`configure-build.sh` s'arrête sur le `cat` manquant — pas de fallback
silencieux vers x86).

## Build manuelle

Si vous souhaitez utiliser l'arbre `openwrt/` déjà présent :

```bash
cd openwrt
make menuconfig   # optionnel
make -j$(nproc)
```

Assurez-vous que `openwrt/stageneth-packages` et `openwrt/files` pointent bien vers les dossiers `packages/` et `files/` de la racine (liens symboliques).
