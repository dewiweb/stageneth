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

define Build/Compile
	cd $(PKG_BUILD_DIR) && ( [ -f go.mod ] || go mod init stageneth-api ) && \
		GO111MODULE=on GOOS=linux GOARCH=$(GO_ARCH) CGO_ENABLED=0 go build -mod=vendor -o stageneth-api .
endef

define Package/stageneth-api/install
	$(INSTALL_DIR) $(1)/usr/sbin
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/stageneth-api $(1)/usr/sbin/

	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/config $(1)/etc/config/stageneth-api

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/stageneth-api.init $(1)/etc/init.d/stageneth-api

	$(INSTALL_DIR) $(1)/etc/nginx/conf.d
	$(INSTALL_DATA) ./files/stageneth-api.locations $(1)/etc/nginx/conf.d/
endef

define Package/stageneth-api/postinst
#!/bin/sh
if [ -z "$$IPKG_INSTROOT" ]; then
  /etc/init.d/stageneth-api enable
  /etc/init.d/stageneth-api restart
  /etc/init.d/nginx reload 2>/dev/null || true
fi
exit 0
endef

$(eval $(call BuildPackage,stageneth-api))
