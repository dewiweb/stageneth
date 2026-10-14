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
    # a patch that doesn't apply must fail the build — silently
    # shipping an unpatched image is worse than no image
    (cd "$target" && patch -p1 -f) < "$patch" || { echo "FAILED: $patch"; exit 1; }
done
