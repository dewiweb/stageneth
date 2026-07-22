#!/usr/bin/env sh

#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

set -e

if [ ! -d patches ]; then
    echo "No patches directory, skipping"
    exit 0
fi

find patches -type f -name '*.patch' | sort | while read -r patch; do
    dir=$(dirname "$patch" | sed 's|^patches/||')
    target=""
    case "$dir" in
        openwrt)
            target="."
            ;;
        packages)
            target="feeds/packages"
            ;;
        *)
            echo "Unknown patch directory: $dir"
            continue
            ;;
    esac
    echo "Applying $patch to $target"
    (cd "$target" && patch -p1 -f) < "$patch" || true
done
