#!/usr/bin/env sh

#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#
# Builds the StageNeth .ipk packages for ONE OpenWrt target using the
# OpenWrt SDK — minutes instead of a full buildroot. The resulting ipk
# carries the generic arch tag (aarch64, mipsel_24kc, x86_64, ...) and
# installs on any device of that arch, not just the SDK's target.
#
# usage: build-ipks.sh <target>/<subtarget> [owrt-version] [outdir]
#   e.g. build-ipks.sh mediatek/filogic v24.10.5 ./ipk-out
#
# Requirements: curl, tar with zstd support, go >= go.mod version,
# make, gcc (SDK sanity checks). No docker needed.

set -e

ts=${1:?usage: build-ipks.sh <target>/<subtarget> [version] [outdir]}
ver=${2:-${OWRT_VERSION:-v24.10.5}}
ver=${ver#v}
out=${3:-$(pwd)/ipk-out}

base="https://downloads.openwrt.org/releases/${ver}/targets/${ts}"
echo "== locating SDK for ${ts} (${ver})"
tarball=$(curl -fsSL "$base/" \
    | grep -oP "openwrt-sdk-${ver}-[a-z0-9_-]+_gcc-[0-9.]+_musl\.Linux-x86_64\.tar\.zst" \
    | sort -u | head -1)
[ -n "$tarball" ] || { echo "ERROR: no SDK tarball listed at $base/"; exit 1; }
echo "== $tarball"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

curl -fsSL "$base/$tarball" -o "$work/sdk.tar.zst"
tar --zstd -xf "$work/sdk.tar.zst" -C "$work"
sdk=$(echo "$work"/openwrt-sdk-*)

# Our packages are NOT a remote feed — copy them straight into the
# SDK's package directory.
pkgroot=$(CDPATH= cd -- "$(dirname -- "$0")/../packages" && pwd)
cp -r "$pkgroot/stagenethd" "$sdk/package/"
cp -r "$pkgroot/stageneth-ui" "$sdk/package/"

cd "$sdk"
cat >> .config <<EOF
CONFIG_PACKAGE_stagenethd=y
CONFIG_PACKAGE_stageneth-ui=y
EOF
make defconfig >/dev/null

make -j"$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)" \
    package/stagenethd/compile \
    package/stageneth-ui/compile \
    V=s

mkdir -p "$out"
find bin/packages -name '*.ipk' -exec cp -v {} "$out/" \;

# Package compile failures can be swallowed by the buildroot — verify
# both packages actually produced an ipk rather than trusting make.
for pkg in stagenethd stageneth-ui; do
    if ! ls "$out/${pkg}_"*.ipk >/dev/null 2>&1; then
        echo "ERROR: no ${pkg}_*.ipk produced" >&2
        for f in "$sdk"/logs/package/"$pkg"/*.txt; do
            [ -f "$f" ] && { echo "--- $f"; cat "$f"; } >&2
        done
        exit 1
    fi
done
echo "== ipks written to $out"
ls -la "$out"
