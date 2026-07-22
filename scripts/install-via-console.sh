#!/usr/bin/env python3
import subprocess
import time
import sys

def send_command(process, command):
    process.stdin.write(command + '\n')
    process.stdin.flush()
    time.sleep(1)

def main():
    # Start QEMU
    qemu_cmd = [
        'qemu-system-x86_64',
        '-m', '2048',
        '-smp', '2',
        '-enable-kvm',
        '-net', 'nic,model=e1000',
        '-net', 'user,hostfwd=tcp::2223-:22',
        '-nographic',
        '-drive', 'file=nethsecurity-8.7.2-x86-64-generic-squashfs-combined-efi.img,format=raw',
        '-serial', 'mon:stdio'
    ]
    
    process = subprocess.Popen(qemu_cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    
    # Wait for boot
    time.sleep(30)
    
    # Send Enter to activate console
    send_command(process, '')
    
    # Read the install script
    with open('scripts/install-stageneth-simple.sh', 'r') as f:
        install_script = f.read()
    
    # Send the install script
    send_command(process, install_script)
    
    # Execute the script
    send_command(process, 'sh /tmp/install-stageneth.sh')
    
    # Wait for completion
    time.sleep(10)
    
    # Test the installation
    send_command(process, 'stageneth-vlan-profiles.sh list')
    
    # Keep the process running
    process.wait()

if __name__ == '__main__':
    main()
