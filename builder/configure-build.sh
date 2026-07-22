#!/usr/bin/env sh

#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

set -e

stageneth_version=${STAGENETH_VERSION:?Missing STAGENETH_VERSION}
repo_channel=${REPO_CHANNEL:?Missing REPO_CHANNEL}
target=${TARGET:?Missing TARGET}
owrt_version=${OWRT_VERSION:?Missing OWRT_VERSION}
build_semver_suffix=${BUILD_SEMVER_SUFFIX:-}

if [ -n "$build_semver_suffix" ]; then
    image_version="${stageneth_version}${build_semver_suffix}"
else
    image_version="${stageneth_version}"
fi

for file in config/*.conf; do
    if [ -f "$file" ]; then
        echo "Processing $file"
        cat "$file" >> .config
    fi
done

cat <<EOF >> .config
CONFIG_GRUB_TITLE="StageNeth"
CONFIG_VERSION_DIST="StageNeth"
CONFIG_VERSION_MANUFACTURER="StageNeth"
CONFIG_VERSION_MANUFACTURER_URL="https://github.com/stageneth"
CONFIG_VERSION_NUMBER="${image_version}"
CONFIG_VERSION_CODE="${owrt_version}"
CONFIG_VERSION_PRODUCT="StageNeth"
CONFIG_VERSION_REPO="https://updates.stageneth.org/${repo_channel}/${stageneth_version}"
CONFIG_VERSION_HOME_URL="https://github.com/stageneth"
CONFIG_VERSION_SUPPORT_URL="https://github.com/stageneth"
CONFIG_VERSION_BUG_URL="https://github.com/stageneth/issues"
EOF

cat "config/targets/${target}.conf" >> .config

echo "${repo_channel}" > files/etc/repo-channel

make defconfig
