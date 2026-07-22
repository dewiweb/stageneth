# Guide de création de packages StageNeth

> Guide basé sur l'analyse des packages NethSecurity pour créer des packages personnalisés pour StageNeth.

---

## Structure d'un package StageNeth

Basée sur la convention NethSecurity, un package StageNeth doit suivre cette structure :

```
packages/stageneth-<nom>/
├── Makefile              # Méta-données du package
├── README.md             # Documentation du package
└── files/                # Fichiers à installer
    ├── stageneth-<nom>.py     # Script principal (si Python)
    ├── stageneth-<nom>.init    # Script init (service)
    └── config                  # Configuration UCI par défaut
```

---

## Conventions de nommage

- **Préfixe** : `stageneth-` (au lieu de `ns-` pour NethSecurity)
- **Exemples** :
  - `stageneth-vlan-profiles`
  - `stageneth-ptp`
  - `stageneth-igmp`
  - `stageneth-luci-module`

---

## Makefile type

### Package simple (scripts shell/Python)

```makefile
#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

include $(TOPDIR)/rules.mk

PKG_NAME:=stageneth-vlan-profiles
PKG_VERSION:=0.0.1
PKG_RELEASE:=1

PKG_BUILD_DIR:=$(BUILD_DIR)/stageneth-vlan-profiles-$(PKG_VERSION)

PKG_MAINTAINER:=StageNeth Contributors
PKG_LICENSE:=GPL-2.0-only

include $(INCLUDE_DIR)/package.mk

define Package/stageneth-vlan-profiles
	SECTION:=base
	CATEGORY:=StageNeth
	TITLE:=StageNeth VLAN profiles for live events
	URL:=https://github.com/stageneth/stageneth
	DEPENDS:=+python3-nethsec +python3-jinja2
	PKGARCH:=all
endef

define Package/stageneth-vlan-profiles/description
	Pre-configured VLAN profiles for audio, video and lighting protocols
endef

define Build/Compile
endef

define Package/stageneth-vlan-profiles/postinst
#!/bin/sh
if [ -z "$${IPKG_INSTROOT}" ]; then
  /etc/init.d/stageneth-vlan-profiles restart
fi
exit 0
endef

define Package/stageneth-vlan-profiles/install
	$(INSTALL_DIR) $(1)/usr/sbin
	$(INSTALL_BIN) ./files/stageneth-vlan-profiles.py $(1)/usr/sbin/stageneth-vlan-profiles
	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/stageneth-vlan-profiles.init $(1)/etc/init.d/stageneth-vlan-profiles
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/config $(1)/etc/config/stageneth-vlan-profiles
endef

$(eval $(call BuildPackage,stageneth-vlan-profiles))
```

### Package avec dépendances externes (Node.js, Go, etc.)

```makefile
#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

include $(TOPDIR)/rules.mk

PKG_NAME:=stageneth-ui
# renovate: datasource=github-releases depName=StageNeth/stageneth-ui
PKG_VERSION:=1.0.0
PKG_RELEASE:=1

PKG_SOURCE_PROTO:=git
PKG_SOURCE_URL:=https://github.com/StageNeth/stageneth-ui.git
PKG_SOURCE_VERSION:=$(PKG_VERSION)
PKG_SOURCE_SUBDIR:=stageneth-ui-$(PKG_SOURCE_VERSION)
PKG_BUILD_DIR:=$(BUILD_DIR)/$(PKG_SOURCE_SUBDIR)
PKG_MIRROR_HASH:=skip

PKG_MAINTAINER:=StageNeth Contributors
PKG_LICENSE:=GPL-3.0-only

PKG_BUILD_DEPENDS:=node/host
PKG_BUILD_PARALLEL:=1

include $(INCLUDE_DIR)/package.mk

define Package/stageneth-ui
	SECTION:=base
	CATEGORY:=StageNeth
	TITLE:=StageNeth UI
	URL:=https://github.com/StageNeth/stageneth-ui/
	DEPENDS:=+nginx-ssl
	PKGARCH:=all
endef

define Package/stageneth-ui/description
	StageNeth web user interface for live event network configuration
endef

define Build/Configure
endef

define Package/stageneth-ui/conffiles
/etc/config/stageneth-ui
/www-stageneth/branding.js
endef

define Build/Compile
	(cd $(PKG_BUILD_DIR) && npm ci && npm run build)
endef

define Package/stageneth-ui/install
	$(INSTALL_DIR) $(1)/www-stageneth
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_DIR) $(1)/usr/sbin
	$(INSTALL_DIR) $(1)/etc/nginx/conf.d
	$(INSTALL_CONF) ./files/00stageneth.locations $(1)/etc/nginx/conf.d/
	$(INSTALL_CONF) ./files/config $(1)/etc/config/stageneth-ui
	$(INSTALL_BIN) ./files/stageneth-ui $(1)/usr/sbin
	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/stageneth-ui.init $(1)/etc/init.d/stageneth-ui
	$(INSTALL_DIR) $(1)/etc/uci-defaults
	$(INSTALL_BIN) ./files/stageneth-ui.uci-defaults $(1)/etc/uci-defaults
	$(CP) $(PKG_BUILD_DIR)/dist/* $(1)/www-stageneth
endef

$(eval $(call BuildPackage,stageneth-ui))
```

---

## Fichiers de configuration

### Activer un package dans l'image

Créer un fichier `config/stageneth-<nom>.conf` :

```bash
CONFIG_PACKAGE_stageneth-vlan-profiles=y
CONFIG_PACKAGE_stageneth-ptp=y
CONFIG_PACKAGE_stageneth-igmp=y
```

### Configuration UCI par défaut

Dans `files/config` :

```
config stageneth 'vlan_profiles'
	option profile 'concert'
		option audio_vlan '20'
		option video_vlan '30'
		option light_vlan '40'
```

---

## Scripts Python

### En-tête requis

```python
#!/usr/bin/python3
#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

import sys
from nethsec import uci
```

### Utilisation de python3-nethsec

Le package `python3-nethsec` fournit des utilitaires pour interagir avec UCI :

```python
from nethsec import uci

# Lire une configuration
config = uci.get('network.lan')

# Modifier une configuration
uci.set('network.lan.ipaddr', '192.168.1.1/24')

# Commiter les changements
uci.commit()
```

---

## Scripts init (OpenWrt)

### Exemple de script init

```bash
#!/bin/sh /etc/rc.common
#
# Copyright (C) 2026 StageNeth Contributors
# SPDX-License-Identifier: GPL-2.0-only
#

START=90
STOP=10

USE_PROCD=1
PROCD_DEBUG=0

start_service() {
	procd_open_instance
	procd_set_param command /usr/sbin/stageneth-vlan-profiles
	procd_set_param respawn
	procd_close_instance
}

stop_service() {
	# Cleanup if needed
	:
}
```

---

## Packages StageNeth proposés

### 1. stageneth-vlan-profiles

**Objectif** : Profils VLAN préconfigurés pour différents types de spectacles

**Contenu** :
- Profils par défaut (concert, théâtre, conférence, broadcast)
- Scripts d'application des profils
- Templates UCI pour configuration réseau

**Dépendances** : `+python3-nethsec +python3-jinja2`

### 2. stageneth-ptp

**Objectif** : Configuration avancée PTP pour synchronisation temps

**Contenu** :
- Configuration `linuxptp` (ptp4l, phc2sys)
- Profils PTP par protocole (Dante, AES67, ST 2110)
- Monitoring PTP

**Dépendances** : `+linuxptp +python3-nethsec`

### 3. stageneth-igmp

**Objectif** : Configuration IGMP/MLD pour multicast

**Contenu** :
- Configuration `igmpproxy` et `smcroute`
- Règles IGMP snooping par VLAN
- Profils multicast par protocole

**Dépendances** : `+igmpproxy +smcroute +python3-nethsec`

### 4. stageneth-luci-module

**Objectif** : Module LuCI pour interface StageNeth

**Contenu** :
- Pages LuCI pour configuration VLAN
- Assistant de profil de spectacle
- Interface de gestion des appareils

**Dépendances** : `+luci-base +luci-mod-network`

### 5. stageneth-qos

**Objectif** : Configuration QoS par protocole AV

**Contenu** :
- Règles QoS par VLAN
- Profils de priorité DSCP/CoS
- Configuration SQM

**Dépendances** : `+qosify +sqm-scripts +python3-nethsec`

---

## Intégration dans le build system

### 1. Ajouter le package dans packages/

Créer le répertoire et les fichiers du package.

### 2. Créer la config dans config/

Créer `config/stageneth-<nom>.conf` avec `CONFIG_PACKAGE_stageneth-<nom>=y`.

### 3. Rebuild

```bash
./build-nethsec.sh
```

Ou pour compiler uniquement le package :

```bash
./build-nethsec.sh bash -- -c "make package/feeds/nethsecurity/stageneth-<nom>/{download,compile} V=sc"
```

---

## Modifications de branding

Pour transformer NethSecurity en StageNeth, modifier :

### config/branding.conf

```bash
CONFIG_VERSION_DIST="StageNeth"
CONFIG_VERSION_PRODUCT="StageNeth Router"
CONFIG_VERSION_MANUFACTURER="StageNeth Project"
CONFIG_VERSION_HOME_URL="https://github.com/stageneth/stageneth"
CONFIG_VERSION_SUPPORT_URL="https://github.com/stageneth/stageneth/issues"
CONFIG_VERSION_BUG_URL="https://github.com/stageneth/stageneth/issues"
```

### Supprimer les packages NethSecurity non nécessaires

Commenter ou supprimer dans les fichiers config correspondants :
- `ns-api-server.conf`
- `ns-ui.conf`
- `ns-monitoring.conf`
- etc.

---

## Prochaine étape

Créer le premier package StageNeth : `stageneth-vlan-profiles` avec les profils VLAN définis dans le README.
