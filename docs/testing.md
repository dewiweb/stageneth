# Test de l'image StageNeth

## Lancer l'image avec QEMU

```bash
gunzip -c bin/stageneth-0.1.0-alpha-x86-64-generic-ext4-combined.img.gz > /tmp/stageneth-test.img
qemu-system-x86_64 -m 1024 -smp 2 -enable-kvm \
  -hda /tmp/stageneth-test.img -display none -serial file:/tmp/stageneth-qemu.log \
  -netdev user,id=net0,net=192.168.1.0/24,hostfwd=tcp::2222-192.168.1.1:22,hostfwd=tcp::8443-192.168.1.1:443 \
  -device e1000,netdev=net0
```

## Accès

- **UI** : `https://192.168.1.1/`  
  Avec le forward QEMU : `https://localhost:8443/`
- **SSH** : `ssh -p 2222 root@localhost`
- **Mot de passe root test** : `stageneth`

> **Avertissement sécurité** : le mot de passe `stageneth` est un mot de passe de test/documentaire. En production, changez-le dès le premier lancement via **Paramètres > Credentials** de l'interface web, ou en SSH avec `passwd`.

## Vérifications

- `nginx`, `stageneth-api` et `stageneth-network` sont running
- L'API `/api/login` avec `root` / `stageneth` retourne un token
- Les endpoints `/api/network/interfaces`, `/api/ntp`, `/api/logs` répondent
- La bannière et le hostname affichent `StageNeth`
