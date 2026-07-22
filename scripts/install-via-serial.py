#!/usr/bin/env python3
import pexpect
import time
import sys

def install_stageneth_via_serial():
    # Wait for QEMU to boot
    print("Waiting for QEMU to boot...")
    time.sleep(30)
    
    # Connect to QEMU via serial console
    print("Connecting to QEMU via serial console...")
    child = pexpect.spawn('telnet localhost 5555', encoding='utf-8', timeout=60)
    
    # Wait for console prompt
    try:
        child.expect("Please press Enter to activate this console.", timeout=30)
        child.sendline("")
        print("Console activated!")
    except pexpect.exceptions.TIMEOUT:
        print("Timeout waiting for console prompt, trying anyway...")
    
    # Wait for shell prompt
    try:
        child.expect(["#", "$"], timeout=30)
        print("Shell prompt ready!")
    except pexpect.exceptions.TIMEOUT:
        print("Timeout waiting for shell prompt")
        child.close()
        return
    
    # Activate SSH
    print("Activating SSH...")
    child.sendline("/etc/init.d/dropbear start")
    child.expect(["#", "$"], timeout=10)
    child.sendline("/etc/init.d/dropbear enable")
    child.expect(["#", "$"], timeout=10)
    
    # Mount the scripts directory
    print("Mounting scripts directory...")
    child.sendline("mkdir -p /mnt/scripts")
    child.expect(["#", "$"], timeout=10)
    child.sendline("mount -t 9p -o trans=virtio stageneth_scripts /mnt/scripts")
    child.expect(["#", "$"], timeout=10)
    
    # Run the install script
    print("Running StageNeth installation script...")
    child.sendline("sh /mnt/scripts/install-stageneth-simple.sh")
    child.expect(["#", "$"], timeout=30)
    
    # Test the installation
    print("Testing installation...")
    child.sendline("stageneth-vlan-profiles.sh list")
    child.expect(["#", "$"], timeout=10)
    
    # Close connection
    print("Installation complete!")
    child.close()

if __name__ == "__main__":
    install_stageneth_via_serial()
