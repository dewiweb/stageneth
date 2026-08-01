#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

include $(TOPDIR)/rules.mk

PKG_NAME:=stageneth-network
PKG_VERSION:=0.2.0
PKG_RELEASE:=1
PKG_LICENSE:=GPL-2.0-only

PKG_BUILD_DIR:=$(BUILD_DIR)/stageneth-network-$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk

define Package/stageneth-network
	SECTION:=base
	CATEGORY:=StageNeth
	TITLE:=StageNeth multi-service networking
	DEPENDS:=+firewall4 +ethtool +tcpdump +snmpd +rsyslog +umdns +mdns-repeater +linuxptp
endef

define Package/stageneth-network/description
	Configure and generate network, firewall and QoS for simultaneous StageNeth services.
endef

GO_ARCH:=amd64
ifeq ($(ARCH),x86_64)
GO_ARCH:=amd64
endif
ifeq ($(ARCH),i386)
GO_ARCH:=386
endif
ifeq ($(ARCH),aarch64)
GO_ARCH:=arm64
endif
ifeq ($(ARCH),arm)
GO_ARCH:=arm
endif
ifeq ($(ARCH),mips)
GO_ARCH:=mips
endif
ifeq ($(ARCH),mipsel)
GO_ARCH:=mipsle
endif
ifeq ($(ARCH),mips64)
GO_ARCH:=mips64
endif
ifeq ($(ARCH),mips64el)
GO_ARCH:=mips64le
endif

define Build/Prepare
	mkdir -p $(PKG_BUILD_DIR)
	cp -R ./files/* $(PKG_BUILD_DIR)/
endef

define Build/Configure
endef

define Build/Compile
	cd $(PKG_BUILD_DIR) && \
		GO111MODULE=on GOOS=linux GOARCH=$(GO_ARCH) CGO_ENABLED=0 go build -o stageneth-network .
endef

define Package/stageneth-network/install
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/config $(1)/etc/config/stageneth

	$(INSTALL_DIR) $(1)/usr/sbin
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/stageneth-network $(1)/usr/sbin/

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/stageneth-network.init $(1)/etc/init.d/stageneth-network

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/stageneth-ptp.init $(1)/etc/init.d/stageneth-ptp
	$(INSTALL_DIR) $(1)/etc/stageneth
	$(INSTALL_DATA) ./files/ptp4l-stage.conf $(1)/etc/stageneth/ptp4l-stage.conf
	$(INSTALL_DIR) $(1)/etc/uci-defaults
	$(INSTALL_BIN) ./files/uci-defaults/99_stageneth-ntp $(1)/etc/uci-defaults/
	$(INSTALL_BIN) ./files/uci-defaults/99_stageneth-monitoring $(1)/etc/uci-defaults/
endef

define Package/stageneth-network/postinst
#!/bin/sh
if [ -z "$$IPKG_INSTROOT" ]; then
  /etc/init.d/stageneth-network enable
  /etc/init.d/stageneth-network restart
fi
exit 0
endef

$(eval $(call BuildPackage,stageneth-network))
