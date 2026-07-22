#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

include $(TOPDIR)/rules.mk

PKG_NAME:=stageneth-ui
PKG_VERSION:=0.2.0
PKG_RELEASE:=1
PKG_LICENSE:=GPL-2.0-only

include $(INCLUDE_DIR)/package.mk

define Package/stageneth-ui
	SECTION:=base
	CATEGORY:=StageNeth
	TITLE:=StageNeth web UI
	DEPENDS:=+stageneth-api +nginx-ssl
	PKGARCH:=all
endef

define Package/stageneth-ui/description
	Minimal Vue 3 UI served via nginx, served from /www-stageneth.
endef

define Build/Compile
endef

define Package/stageneth-ui/install
	$(INSTALL_DIR) $(1)/www-stageneth
	$(INSTALL_DATA) ./files/index.html $(1)/www-stageneth/
	$(INSTALL_DATA) ./files/app.js $(1)/www-stageneth/
	$(INSTALL_DATA) ./files/api.js $(1)/www-stageneth/
	$(INSTALL_DATA) ./files/utils.js $(1)/www-stageneth/
	$(CP) ./files/vendor $(1)/www-stageneth/
	$(CP) ./files/tabs $(1)/www-stageneth/

	$(INSTALL_DIR) $(1)/etc/nginx/conf.d
	$(INSTALL_DATA) ./files/stageneth-ui.locations $(1)/etc/nginx/conf.d/
endef

define Package/stageneth-ui/postinst
#!/bin/sh
if [ -z "$$IPKG_INSTROOT" ]; then
  /etc/init.d/nginx reload 2>/dev/null || true
fi
exit 0
endef

$(eval $(call BuildPackage,stageneth-ui))
