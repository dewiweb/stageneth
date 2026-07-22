#!/usr/bin/env python3
import pexpect
import time
import sys

def install_stageneth():
    # Wait for QEMU to boot
    print("Waiting for QEMU to boot...")
    time.sleep(30)
    
    # Connect to QEMU via SSH
    ssh_cmd = "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p 2224 root@localhost"
    
    print("Connecting to QEMU via SSH...")
    child = pexpect.spawn(ssh_cmd, timeout=60, encoding='utf-8')
    child.logfile = open('/tmp/ssh-install.log', 'w')
    
    # Wait for password prompt
    try:
        child.expect(["password:", "Password:"], timeout=30)
        child.sendline("Nethesis,1234")
    except pexpect.exceptions.TIMEOUT:
        print("Timeout waiting for password prompt, trying without password...")
    
    # Wait for shell prompt
    try:
        child.expect(["#", "$"], timeout=30)
        print("Connected successfully!")
    except pexpect.exceptions.TIMEOUT:
        print("Timeout waiting for shell prompt")
        child.close()
        return
    
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
    install_stageneth()
