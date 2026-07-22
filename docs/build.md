# Build StageNeth

## Prérequis

- [Podman](https://podman.io/) (testé avec Podman, compatible Docker)
- Espace disque : l'arbre OpenWrt occupe plusieurs Go
- `~/.local` et `/tmp` accessibles en écriture (config par défaut dans `build.conf`)

## Build conteneurisée (recommandée)

Depuis la racine du projet :

```bash
./build-stageneth.sh
```

Le script :
- construit l'image `stageneth-next` à partir de `builder/Containerfile`
- lance un conteneur qui compile OpenWrt avec les packages StageNeth
- copie les images résultantes dans `bin/` à la racine

La configuration est dans `build.conf.defaults` et peut être surchargée dans `build.conf`.

## Build manuelle

Si vous souhaitez utiliser l'arbre `openwrt/` déjà présent :

```bash
cd openwrt
make menuconfig   # optionnel
make -j$(nproc)
```

Assurez-vous que `openwrt/stageneth-packages` et `openwrt/files` pointent bien vers les dossiers `packages/` et `files/` de la racine (liens symboliques).
