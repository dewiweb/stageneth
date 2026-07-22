#!/bin/sh
# Script d'installation StageNeth étape par étape
# Copiez et collez chaque bloc de commandes séparément

# ÉTAPE 1: Créer le répertoire StageNeth
mkdir -p /opt/stageneth
mkdir -p /opt/stageneth/vlan-profiles
mkdir -p /opt/stageneth/ptp-profiles
mkdir -p /opt/stageneth/igmp-profiles
mkdir -p /opt/stageneth/ptp-monitor
mkdir -p /opt/stageneth/vlan-monitor

# ÉTAPE 2: Créer les profils VLAN
echo '{"name":"Dante","description":"Profil pour réseaux Dante (Audinate)","vlan_id":20,"qos":{"priority":7,"dscp":46},"ptp":true,"multicast":true}' > /opt/stageneth/vlan-profiles/dante.json
echo '{"name":"AES67","description":"Profil pour réseaux AES67 / RAVENNA","vlan_id":21,"qos":{"priority":7,"dscp":46},"ptp":true,"multicast":true}' > /opt/stageneth/vlan-profiles/aes67.json
echo '{"name":"NDI","description":"Profil pour réseaux NDI / NDI|HX","vlan_id":30,"qos":{"priority":6,"dscp":40},"ptp":false,"multicast":true}' > /opt/stageneth/vlan-profiles/ndi.json
echo '{"name":"Art-Net","description":"Profil pour réseaux Art-Net","vlan_id":40,"qos":{"priority":4,"dscp":32},"ptp":false,"multicast":true}' > /opt/stageneth/vlan-profiles/artnet.json
echo '{"name":"sACN","description":"Profil pour réseaux sACN (E1.31)","vlan_id":41,"qos":{"priority":4,"dscp":32},"ptp":false,"multicast":true}' > /opt/stageneth/vlan-profiles/sacn.json

# ÉTAPE 3: Créer les profils PTP
echo '{"name":"Default","description":"Configuration PTP par défaut","profile":"default","clock_class":248,"priority1":128,"priority2":128}' > /opt/stageneth/ptp-profiles/default.json
echo '{"name":"Audio","description":"Configuration PTP optimisée pour audio","profile":"audio","clock_class":248,"priority1":128,"priority2":128}' > /opt/stageneth/ptp-profiles/audio.json

# ÉTAPE 4: Créer les profils IGMP
echo '{"name":"Default","description":"Configuration IGMP par défaut","version":3,"query_interval":125,"query_response_interval":10}' > /opt/stageneth/igmp-profiles/default.json

# ÉTAPE 5: Créer le script stageneth-vlan-profiles.sh
cat > /usr/bin/stageneth-vlan-profiles.sh << 'SCRIPT'
#!/bin/sh
case "$1" in
    list) ls /opt/stageneth/vlan-profiles/*.json 2>/dev/null | xargs -n1 basename -s .json ;;
    get) cat "/opt/stageneth/vlan-profiles/$2.json" 2>/dev/null || echo "Profile not found" ;;
    *) echo "Usage: $0 {list|get <profile>}" ;;
esac
SCRIPT
chmod +x /usr/bin/stageneth-vlan-profiles.sh

# ÉTAPE 6: Créer le script stageneth-ptp.sh
cat > /usr/bin/stageneth-ptp.sh << 'SCRIPT'
#!/bin/sh
case "$1" in
    list) ls /opt/stageneth/ptp-profiles/*.json 2>/dev/null | xargs -n1 basename -s .json ;;
    get) cat "/opt/stageneth/ptp-profiles/$2.json" 2>/dev/null || echo "Profile not found" ;;
    *) echo "Usage: $0 {list|get <profile>}" ;;
esac
SCRIPT
chmod +x /usr/bin/stageneth-ptp.sh

# ÉTAPE 7: Créer le script stageneth-igmp.sh
cat > /usr/bin/stageneth-igmp.sh << 'SCRIPT'
#!/bin/sh
case "$1" in
    list) ls /opt/stageneth/igmp-profiles/*.json 2>/dev/null | xargs -n1 basename -s .json ;;
    get) cat "/opt/stageneth/igmp-profiles/$2.json" 2>/dev/null || echo "Profile not found" ;;
    *) echo "Usage: $0 {list|get <profile>}" ;;
esac
SCRIPT
chmod +x /usr/bin/stageneth-igmp.sh

echo "StageNeth packages installed successfully!"
echo "Test with: stageneth-vlan-profiles.sh list"
