#!/usr/bin/env sh

#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

set -e

if which "$1" >/dev/null 2>&1; then
    exec "$@"
fi

JOBS=${BUILD_JOBS:-2}

case "$BUILD_VERBOSE" in
    1|true|yes|y|on)
        exec stdbuf -oL -eL make -j"$JOBS" V=sc "$@"
        ;;
    *)
        exec stdbuf -oL -eL make -j"$JOBS" "$@"
        ;;
esac
