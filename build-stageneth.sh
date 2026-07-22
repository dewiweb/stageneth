#!/usr/bin/env sh

#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

set -e

_env_snapshot=$(export -p)

set -o allexport
if [ -f build.conf.defaults ]; then
    echo "Loading build.conf.defaults..."
    . ./build.conf.defaults
fi
if [ -f build.conf ]; then
    echo "Loading build.conf..."
    . ./build.conf
fi
set +o allexport

eval "$_env_snapshot"

OWRT_VERSION=${OWRT_VERSION:?Missing OWRT_VERSION}
STAGENETH_VERSION=${STAGENETH_VERSION:?Missing STAGENETH_VERSION}
REPO_CHANNEL=${REPO_CHANNEL:-dev}
TARGET=${TARGET:-x86_64}
BUILD_SEMVER_SUFFIX=${BUILD_SEMVER_SUFFIX:-}
BUILD_VERBOSE=${BUILD_VERBOSE:-}
STAGENETH_BUILD_ROOT=${STAGENETH_BUILD_ROOT:-/home/dewi/newhome/tmp/stageneth}

RUNTIME=${RUNTIME:-podman}
if [ "$RUNTIME" = "podman" ]; then
    USERSNS="--userns=keep-id"
else
    USERSNS=""
fi

BUILD_BASE="${STAGENETH_BUILD_ROOT}/${OWRT_VERSION}"
for dir in \
    "${BUILD_BASE}/build_dir:/home/buildbot/openwrt/build_dir" \
    "${BUILD_BASE}/staging_dir:/home/buildbot/openwrt/staging_dir" \
    "${BUILD_BASE}/cache:/home/buildbot/openwrt/.ccache" \
    "${BUILD_BASE}/dl:/home/buildbot/openwrt/dl" \
    "${BUILD_BASE}/download:/home/buildbot/openwrt/download"; do
    mkdir -p "${dir%%:*}"
done

chmod -R a+rw "${BUILD_BASE}"

${RUNTIME} build \
    --force-rm \
    --file builder/Containerfile \
    --tag stageneth-next \
    --build-arg OWRT_VERSION="$OWRT_VERSION" \
    --build-arg REPO_CHANNEL="$REPO_CHANNEL" \
    --build-arg TARGET="$TARGET" \
    --build-arg STAGENETH_VERSION="$STAGENETH_VERSION" \
    --build-arg BUILD_SEMVER_SUFFIX="$BUILD_SEMVER_SUFFIX" \
    .

set +e

${RUNTIME} rm -f stageneth-builder >/dev/null 2>&1 || true

status=0
${RUNTIME} run \
    --env BUILD_VERBOSE="$BUILD_VERBOSE" \
    --name stageneth-builder \
    ${USERSNS} \
    --volume "${BUILD_BASE}/build_dir:/home/buildbot/openwrt/build_dir" \
    --volume "${BUILD_BASE}/staging_dir:/home/buildbot/openwrt/staging_dir" \
    --volume "${BUILD_BASE}/cache:/home/buildbot/openwrt/.ccache" \
    --volume "${BUILD_BASE}/dl:/home/buildbot/openwrt/dl" \
    --volume "${BUILD_BASE}/download:/home/buildbot/openwrt/download" \
    stageneth-next \
    "$@" || status=$?

if [ $status -eq 0 ]; then
    rm -rf bin
    ${RUNTIME} cp stageneth-builder:/home/buildbot/openwrt/bin bin
fi

rm -rf build-logs
${RUNTIME} cp stageneth-builder:/home/buildbot/openwrt/logs build-logs 2>/dev/null || true
${RUNTIME} stop stageneth-builder >/dev/null 2>&1 || true
${RUNTIME} rm stageneth-builder >/dev/null 2>&1 || true

exit $status
