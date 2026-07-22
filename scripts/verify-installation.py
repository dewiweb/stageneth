#!/usr/bin/env python3
import pexpect
import time

def verify_installation():
    print("Connecting to QEMU via serial console...")
    child = pexpect.spawn('telnet localhost 5555', encoding='utf-8', timeout=60)
    
    # Wait for shell prompt
    child.expect(['#', '$'], timeout=30)
    print("Connected!")
    
    # Check StageNeth installation
    print("Checking StageNeth installation...")
    child.sendline('ls -la /usr/share/stageneth/')
    child.expect(['#', '$'], timeout=10)
    print("StageNeth directory:", child.before)
    
    child.sendline('ls -la /usr/bin/stageneth-*.sh')
    child.expect(['#', '$'], timeout=10)
    print("StageNeth scripts:", child.before)
    
    child.sendline('cat /usr/share/stageneth/vlan-profiles/dante.json')
    child.expect(['#', '$'], timeout=10)
    print("Dante profile:", child.before)
    
    child.sendline('stageneth-vlan-profiles.sh list')
    child.expect(['#', '$'], timeout=10)
    print("VLAN profiles list:", child.before)
    
    child.close()
    print("Verification complete!")

if __name__ == "__main__":
    verify_installation()
