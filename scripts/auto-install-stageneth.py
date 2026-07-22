#!/usr/bin/env python3
import pexpect
import time
import sys

def auto_install_stageneth():
    print("Lancement de QEMU...")
    qemu_cmd = "./scripts/run-qemu.sh nethsecurity-8.7.2-x86-64-generic-squashfs-combined-efi.img"
    child = pexpect.spawn(qemu_cmd, encoding='utf-8', timeout=120)
    
    # Wait for console activation prompt
    print("Attente du prompt de console...")
    child.expect("Please press Enter to activate this console.", timeout=60)
    print("Prompt de console détecté")
    child.sendline("")
    
    # Wait for login prompt
    print("Attente du prompt de login...")
    child.expect("login:", timeout=30)
    print("Login prompt détecté")
    
    # Wait for user to login manually
    print("Veuillez vous connecter manuellement (root / Nethesis,1234)")
    print("Appuyez sur Entrée dans la console QEMU pour activer la connexion")
    print("Une fois connecté, tapez 'continue' dans le terminal pour continuer l'installation...")
    
    # Wait for user to type 'continue' to proceed
    child.expect("continue", timeout=300)
    print("Continuation détectée, démarrage de l'installation...")
    
    # Enable SSH
    print("Activation de SSH...")
    child.sendline("/etc/init.d/dropbear start")
    child.expect(["#", "$"], timeout=10)
    child.sendline("/etc/init.d/dropbear enable")
    child.expect(["#", "$"], timeout=10)
    print("SSH activé")
    
    # Create stageneth directory
    print("Création du répertoire StageNeth...")
    child.sendline("mkdir -p /opt/stageneth")
    child.sendline("mkdir -p /opt/stageneth/vlan-profiles")
    child.sendline("mkdir -p /opt/stageneth/ptp-profiles")
    child.sendline("mkdir -p /opt/stageneth/igmp-profiles")
    child.expect(["#", "$"], timeout=10)
    
    # Create VLAN profiles
    print("Création des profils VLAN...")
    child.sendline('echo \'{"name":"Dante","description":"Profil pour réseaux Dante (Audinate)","vlan_id":20,"qos":{"priority":7,"dscp":46},"ptp":true,"multicast":true}\' > /opt/stageneth/vlan-profiles/dante.json')
    child.expect(["#", "$"], timeout=10)
    child.sendline('echo \'{"name":"AES67","description":"Profil pour réseaux AES67 / RAVENNA","vlan_id":21,"qos":{"priority":7,"dscp":46},"ptp":true,"multicast":true}\' > /opt/stageneth/vlan-profiles/aes67.json')
    child.expect(["#", "$"], timeout=10)
    child.sendline('echo \'{"name":"NDI","description":"Profil pour réseaux NDI / NDI|HX","vlan_id":30,"qos":{"priority":6,"dscp":40},"ptp":false,"multicast":true}\' > /opt/stageneth/vlan-profiles/ndi.json')
    child.expect(["#", "$"], timeout=10)
    
    # Create PTP profiles
    print("Création des profils PTP...")
    child.sendline('echo \'{"name":"Default","description":"Configuration PTP par défaut","profile":"default","clock_class":248,"priority1":128,"priority2":128}\' > /opt/stageneth/ptp-profiles/default.json')
    child.expect(["#", "$"], timeout=10)
    child.sendline('echo \'{"name":"Audio","description":"Configuration PTP optimisée pour audio","profile":"audio","clock_class":248,"priority1":128,"priority2":128}\' > /opt/stageneth/ptp-profiles/audio.json')
    child.expect(["#", "$"], timeout=10)
    
    # Create IGMP profiles
    print("Création des profils IGMP...")
    child.sendline('echo \'{"name":"Default","description":"Configuration IGMP par défaut","version":3,"query_interval":125,"query_response_interval":10}\' > /opt/stageneth/igmp-profiles/default.json')
    child.expect(["#", "$"], timeout=10)
    
    # Create stageneth-vlan-profiles.sh script
    print("Création du script stageneth-vlan-profiles.sh...")
    child.sendline('cat > /usr/bin/stageneth-vlan-profiles.sh << \'SCRIPT\'')
    child.sendline('#!/bin/sh')
    child.sendline('case "$1" in')
    child.sendline('    list) ls /opt/stageneth/vlan-profiles/*.json 2>/dev/null | xargs -n1 basename -s .json ;;')
    child.sendline('    get) cat "/opt/stageneth/vlan-profiles/$2.json" 2>/dev/null || echo "Profile not found" ;;')
    child.sendline('    *) echo "Usage: $0 {list|get <profile>}" ;;')
    child.sendline('esac')
    child.sendline('SCRIPT')
    child.expect(["#", "$"], timeout=10)
    child.sendline('chmod +x /usr/bin/stageneth-vlan-profiles.sh')
    child.expect(["#", "$"], timeout=10)
    
    # Create stageneth-ptp.sh script
    print("Création du script stageneth-ptp.sh...")
    child.sendline('cat > /usr/bin/stageneth-ptp.sh << \'SCRIPT\'')
    child.sendline('#!/bin/sh')
    child.sendline('case "$1" in')
    child.sendline('    list) ls /opt/stageneth/ptp-profiles/*.json 2>/dev/null | xargs -n1 basename -s .json ;;')
    child.sendline('    get) cat "/opt/stageneth/ptp-profiles/$2.json" 2>/dev/null || echo "Profile not found" ;;')
    child.sendline('    *) echo "Usage: $0 {list|get <profile>}" ;;')
    child.sendline('esac')
    child.sendline('SCRIPT')
    child.expect(["#", "$"], timeout=10)
    child.sendline('chmod +x /usr/bin/stageneth-ptp.sh')
    child.expect(["#", "$"], timeout=10)
    
    # Create stageneth-igmp.sh script
    print("Création du script stageneth-igmp.sh...")
    child.sendline('cat > /usr/bin/stageneth-igmp.sh << \'SCRIPT\'')
    child.sendline('#!/bin/sh')
    child.sendline('case "$1" in')
    child.sendline('    list) ls /opt/stageneth/igmp-profiles/*.json 2>/dev/null | xargs -n1 basename -s .json ;;')
    child.sendline('    get) cat "/opt/stageneth/igmp-profiles/$2.json" 2>/dev/null || echo "Profile not found" ;;')
    child.sendline('    *) echo "Usage: $0 {list|get <profile>}" ;;')
    child.sendline('esac')
    child.sendline('SCRIPT')
    child.expect(["#", "$"], timeout=10)
    child.sendline('chmod +x /usr/bin/stageneth-igmp.sh')
    child.expect(["#", "$"], timeout=10)
    
    # Test installation
    print("Test de l'installation...")
    child.sendline("stageneth-vlan-profiles.sh list")
    child.expect(["#", "$"], timeout=10)
    
    print("Installation réussie !")
    print("Vous pouvez maintenant vous connecter via SSH : ssh -p 2224 root@localhost")
    
    # Keep QEMU running in background
    print("QEMU continue de tourner en arrière-plan.")
    print("Appuyez sur Ctrl+C pour arrêter QEMU.")
    
    # Don't close the child process to keep QEMU running
    try:
        child.interact()
    except KeyboardInterrupt:
        print("\nArrêt de QEMU...")
        child.close()

if __name__ == "__main__":
    auto_install_stageneth()
