# Stratégie de test pour StageNeth

> Comment tester le développement de StageNeth à différents niveaux.

---

## Niveaux de test

### 1. Tests unitaires (scripts et logique)

**Objectif** : Tester les scripts Python, les fonctions de logique métier, les templates.

**Outils** :
- `pytest` pour Python
- `unittest` pour tests basiques
- Tests de configuration UCI (mock)

**Exemple** :

```python
# tests/test_vlan_profiles.py
import pytest
from stageneth_vlan_profiles import apply_profile

def test_apply_concert_profile():
    config = apply_profile('concert')
    assert config['audio_vlan'] == 20
    assert config['video_vlan'] == 30
    assert config['light_vlan'] == 40
```

**Exécution** :

```bash
cd packages/stageneth-vlan-profiles
python3 -m pytest tests/
```

---

### 2. Tests d'intégration (conteneur build)

**Objectif** : Tester les packages dans l'environnement de build OpenWrt.

**Méthode** : Utiliser le conteneur Podman de build pour tester les packages.

**Étapes** :

```bash
# Entrer dans le conteneur de build
cd nethsecurity
./build-nethsec.sh bash

# Compiler uniquement le package
make package/feeds/nethsecurity/stageneth-vlan-profiles/{download,compile} V=sc

# Tester le package manuellement
opkg install bin/packages/x86_64/nethsecurity/stageneth-vlan-profiles_*.ipk
/usr/sbin/stageneth-vlan-profiles --help
```

---

### 3. Tests système (image complète)

**Objectif** : Tester l'image complète StageNeth dans une VM.

#### Option A : QEMU/KVM (recommandé)

**Avantages** :
- Rapide, léger
- Support x86_64 natif
- Snapshot facile
- Support réseau via bridge/tap

**Installation** :

```bash
# Installer QEMU et KVM
sudo apt install qemu-kvm libvirt-daemon-system libvirt-clients bridge-utils virt-manager

# Ajouter l'utilisateur au groupe libvirt
sudo usermod -aG libvirt $USER
```

**Lancer l'image StageNeth** :

```bash
# Après build, l'image est dans nethsecurity/bin/targets/x86_64/
qemu-system-x86_64 \
  -m 2048 \
  -smp 2 \
  -net nic,model=e1000 \
  -net user,hostfwd=tcp::2222-:22 \
  -net nic,model=e1000 \
  -net bridge,br=br0 \
  -drive file=bin/targets/x86_64/stageneth-x86-64-generic-ext4.img.gz,format=raw \
  -nographic
```

**Accès** :

```bash
# SSH via port forward
ssh -p 2222 root@localhost

# Ou via console série
# (si -nographic, directement dans le terminal)
```

#### Option B : VirtualBox

**Avantages** :
- Interface graphique
- Facile pour les débutants
- Support snapshots

**Étapes** :

1. Créer une VM Linux (type Other Linux, 64-bit)
2. 2GB RAM minimum
3. Ajouter un disque vide (au moins 4GB)
4. Convertir l'image StageNeth en VDI si nécessaire
5. Démarrer sur l'image

**Conversion d'image** :

```bash
# Convertir raw en VDI
VBoxManage convertfromraw bin/targets/x86_64/stageneth-x86-64-generic-ext4.img stageneth.vdi
```

#### Option C : VMware Workstation/Player

**Avantages** :
- Support réseau avancé
- Snapshots
- Compatible avec beaucoup de matériels

**Étapes** :

1. Créer une VM Linux (Other Linux 64-bit)
2. 2GB RAM minimum
3. Utiliser l'image raw directement ou convertir en vmdk

**Conversion d'image** :

```bash
# Convertir raw en vmdk
qemu-img convert -O vmdk bin/targets/x86_64/stageneth-x86-64-generic-ext4.img stageneth.vmdk
```

---

### 4. Tests réseau (simulation topologie)

**Objectif** : Tester les VLANs, le routage inter-VLAN, le multicast, PTP dans une topologie réaliste.

#### Option A : GNS3

**Avantages** :
- Simulation réseau réaliste
- Support QEMU/KVM
- Topologies complexes
- Capture Wireshark intégrée

**Setup** :

1. Installer GNS3
2. Ajouter une image QEMU de StageNeth
3. Créer une topologie avec :
   - Routeur StageNeth
   - Switchs (Open vSwitch ou IOS)
   - Clients (Linux, Windows)
   - Appareils simulés (Dante Controller, NDI Tools, etc.)

**Exemple de topologie** :

```
[Internet] -- [WAN] -- [StageNeth] -- [Switch L2] -- [Clients]
                                        |-- [VLAN 20 Audio]
                                        |-- [VLAN 30 Video]
                                        |-- [VLAN 40 Light]
```

#### Option B : EVE-NG

**Avantages** :
- Similaire à GNS3
- Support multi-utilisateur
- Plus orienté lab réseau pro

**Setup** :

1. Installer EVE-NG (VM ou bare metal)
2. Importer l'image StageNeth comme QEMU node
3. Créer des labs de test

#### Option C : Linux Network Namespaces (léger)

**Avantages** :
- Pas besoin de virtualisation lourde
- Tests rapides sur une seule machine
- Idéal pour tests unitaires réseau

**Exemple** :

```bash
# Créer namespaces
ip netns add mgmt
ip netns add audio
ip netns add video

# Créer veth pairs
ip link add veth0 type veth peer name veth1
ip link set veth0 netns mgmt
ip link set veth1 netns audio

# Configurer les interfaces
ip netns exec mgmt ip addr add 192.168.10.1/24 dev veth0
ip netns exec audio ip addr add 192.168.20.1/24 dev veth1

# Tester le routage
ip netns exec mgmt ping 192.168.20.1
```

---

### 5. Tests sur matériel réel

**Objectif** : Validation finale sur le matériel cible.

**Matériel recommandé** :

- **Mini-PC x86_64** : Intel NUC, Zotac, Fujitsu, etc.
- **Appliances de routage** : Protectli, Qotom, etc.
- **Anciens PC** : tout PC x86_64 avec 2 ports réseau minimum

**Flashage de l'image** :

```bash
# Sur Linux
dd if=bin/targets/x86_64/stageneth-x86-64-generic-ext4.img of=/dev/sdX bs=4M status=progress

# Sur Windows (avec Rufus)
# Utiliser Rufus pour flasher l'image sur USB
```

**Configuration de test** :

1. Connecter WAN vers Internet/box
2. Connecter LAN vers switch de test
3. Connecter plusieurs clients sur différents VLANs
4. Tester :
   - Accès web UI
   - Configuration VLANs
   - Routage inter-VLAN
   - PTP (si disponible)
   - Multicast (igmpquerier, etc.)

---

### 6. Tests automatisés (CI/CD)

**Objectif** : Intégrer les tests dans le pipeline de build.

**GitHub Actions** :

```yaml
name: Test StageNeth

on: [push, pull_request]

jobs:
  test-unit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Set up Python
        uses: actions/setup-python@v4
        with:
          python-version: '3.11'
      - name: Install dependencies
        run: |
          pip install pytest
      - name: Run unit tests
        run: |
          pytest packages/stageneth-*/tests/

  test-build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Build image
        run: |
          cd nethsecurity
          ./build-nethsec.sh
      - name: Upload artifacts
        uses: actions/upload-artifact@v3
        with:
          name: stageneth-image
          path: nethsecurity/bin/
```

---

## Scénarios de test prioritaires

### Scénario 1 : Profil VLAN concert

**Objectif** : Valider l'application d'un profil VLAN complet.

**Étapes** :
1. Démarrer StageNeth dans QEMU
2. Accéder à l'interface web
3. Sélectionner le profil "Concert"
4. Vérifier que les VLANs sont créés (20, 30, 40)
5. Vérifier que les interfaces sont configurées
6. Vérifier que le firewall est configuré
7. Tester le ping entre VLANs autorisés
8. Tester que les VLANs non autorisés sont isolés

### Scénario 2 : Configuration PTP

**Objectif** : Valider la configuration PTP pour Dante.

**Étapes** :
1. Activer PTP dans l'interface
2. Configurer le profil PTP Dante
3. Vérifier que ptp4l tourne
4. Vérifier la synchronisation (phc2sys)
5. Simuler un client PTP et vérifier la sync

### Scénario 3 : Multicast IGMP

**Objectif** : Valider le multicast pour sACN/NDI.

**Étapes** :
1. Configurer IGMP querier
2. Activer IGMP snooping sur le switch
3. Lancer un flux multicast (sACN)
4. Vérifier que les flux sont routés correctement
5. Vérifier que les groupes sont correctement gérés

### Scénario 4 : Appareil NIC unique

**Objectif** : Valider la configuration trunk pour appareils à NIC unique.

**Étapes** :
1. Sélectionner un appareil "Console Dante" dans l'interface
2. Appliquer la configuration trunk (VLAN 20 + 10 tagged)
3. Vérifier la configuration du port
4. Simuler un appareil sur ce port
5. Vérifier l'accès management et les flux

---

## Outils de diagnostic

**Sur le système StageNeth** :

```bash
# Vérifier les VLANs
ip link show
bridge vlan show

# Vérifier le routage
ip route show
ip rule show

# Vérifier le firewall
nft list ruleset
iptables -L -v

# Vérifier PTP
ptp4l -h
pmc -u -b 0 'GET PORT_DATA_SET'

# Vérifier IGMP
cat /proc/net/igmp
igmpproxy -h

# Capturer le trafic
tcpdump -i eth0 -n vlan
tcpdump -i eth0.20 -n udp port 6454  # Art-Net
```

**Depuis un client** :

```bash
# Tester la latence
ping -i 0.001 192.168.20.1

# Tester le débit
iperf3 -c 192.168.20.1

# Tester PTP
ptp4l -i eth0 -s -m

# Capturer multicast
tcpdump -i eth0 udp multicast
```

---

## Recommandation pour le développement initial

**Phase 1 (développement)** :
- Tests unitaires avec pytest
- Tests dans le conteneur de build
- Scripts de validation de configuration

**Phase 2 (intégration)** :
- VM QEMU pour tests système
- Network namespaces pour tests réseau légers
- Scénarios de test automatisés

**Phase 3 (validation)** :
- GNS3 pour topologies complexes
- Matériel réel pour validation finale
- Tests terrain sur un vrai spectacle

---

## Prochaine étape

Commencer par mettre en place un environnement de test QEMU basique pour valider le build d'une image NethSecurity vanilla, puis adapter pour StageNeth.
