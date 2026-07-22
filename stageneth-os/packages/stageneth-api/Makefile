#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

include $(TOPDIR)/rules.mk

PKG_NAME:=stageneth-api
PKG_VERSION:=0.1.0
PKG_RELEASE:=1
PKG_LICENSE:=GPL-2.0-only

PKG_BUILD_DIR:=$(BUILD_DIR)/stageneth-api-$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk

define Package/stageneth-api
	SECTION:=base
	CATEGORY:=StageNeth
	TITLE:=StageNeth REST API server
	DEPENDS:=+stageneth-network +nginx-ssl +openssl-util
	PKGARCH:=all
endef

define Package/stageneth-api/description
	Go REST API for StageNeth: login, service CRUD, apply.
endef

define Build/Prepare
	mkdir -p $(PKG_BUILD_DIR)
	cp -R ./files/* $(PKG_BUILD_DIR)/
endef

define Build/Compile
	cd $(PKG_BUILD_DIR) && \
		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o stageneth-api .
endef

define Package/stageneth-api/install
	$(INSTALL_DIR) $(1)/usr/sbin
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/stageneth-api $(1)/usr/sbin/

	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/config $(1)/etc/config/stageneth-api

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/stageneth-api.init $(1)/etc/init.d/stageneth-api
endef

define Package/stageneth-api/postinst
#!/bin/sh
if [ -z "$$IPKG_INSTROOT" ]; then
  /etc/init.d/stageneth-api enable
  /etc/init.d/stageneth-api restart
fi
exit 0
endef

$(eval $(call BuildPackage,stageneth-api))
