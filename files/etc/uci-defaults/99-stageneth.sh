#!/bin/sh

# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only

# Set default root password
if [ -f /etc/shadow ] && command -v openssl >/dev/null 2>&1; then
    HASH=$(openssl passwd -1 -salt stageneth stageneth)
    sed -i "s|^root:[^:]*:|root:${HASH}:|" /etc/shadow
fi

# Generate self-signed certificate for nginx
if command -v openssl >/dev/null 2>&1; then
    mkdir -p /etc/nginx
    if [ ! -f /etc/nginx/nginx.cer ]; then
        openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
            -keyout /etc/nginx/nginx.key -out /etc/nginx/nginx.cer \
            -subj "/CN=stageneth" 2>/dev/null
    fi
fi

# Apply StageNeth network configuration on first boot
if [ -x /usr/libexec/stageneth/stageneth-network.py ]; then
    /usr/libexec/stageneth/stageneth-network.py apply
fi

exit 0
