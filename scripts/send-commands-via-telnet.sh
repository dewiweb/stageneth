#!/bin/bash
sleep 30
(
    echo ""
    sleep 5
    cat scripts/install-stageneth-simple.sh
    sleep 2
    echo "sh /tmp/install-stageneth.sh"
    sleep 5
    echo "stageneth-vlan-profiles.sh list"
    sleep 2
) | telnet localhost 5555
