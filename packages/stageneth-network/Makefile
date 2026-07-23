#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

include $(TOPDIR)/rules.mk

PKG_NAME:=stageneth-network
PKG_VERSION:=0.2.0
PKG_RELEASE:=1
PKG_LICENSE:=GPL-2.0-only

include $(INCLUDE_DIR)/package.mk

define Package/stageneth-network
	SECTION:=base
	CATEGORY:=StageNeth
	TITLE:=StageNeth multi-service networking
	DEPENDS:=+python3 +firewall4 +ethtool +tcpdump
	PKGARCH:=all
endef

define Package/stageneth-network/description
	Configure and generate network, firewall and QoS for simultaneous StageNeth services.
endef

define Build/Prepare
	mkdir -p $(PKG_BUILD_DIR)
endef

define Build/Configure
endef

define Build/Compile
endef

define Package/stageneth-network/install
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/config $(1)/etc/config/stageneth

	$(INSTALL_DIR) $(1)/usr/libexec/stageneth
	$(INSTALL_BIN) ./files/stageneth-network.py $(1)/usr/libexec/stageneth/

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/stageneth-network.init $(1)/etc/init.d/stageneth-network

	$(INSTALL_DIR) $(1)/etc/uci-defaults
	$(INSTALL_BIN) ./files/uci-defaults/99_stageneth-ntp $(1)/etc/uci-defaults/
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
