# Instalasi NADI Agent v0.7.5-dev — simpiav2

Paket ini disiapkan untuk simpiav2 Linux Ubuntu x86_64. Installer hanya memasang binary dan unit systemd; installer **tidak** melakukan enrollment ulang, membuat Activation Grant, mengubah NADI Central, mengubah Nginx, atau memulai service secara otomatis.

## Prasyarat

- Server `simpiav2` menggunakan `x86_64`.
- Agent sudah terdaftar di Central dan status monitoring aplikasi `ACTIVE`.
- State Agent hasil enrollment tersedia di `/var/lib/nadi-agent/state.json` dan dimiliki `nadi-agent:nadi-agent` dengan mode `0600`.
- Public key Activation Central tersedia di `/etc/nadi-agent/keys/activation-public.pem` (Ed25519 PEM yang sudah digunakan sebelumnya).
- Jangan menghapus, mengganti, atau menyalin `state.json` dari server lain. File tersebut berisi credential.

## 1. Verifikasi paket di server

Ekstrak ZIP ke direktori kerja, lalu:

```bash
uname -m
sha256sum -c BINARIES-SHA256SUMS.txt
./bin/linux-amd64/nadi-agent --version
```

Hasil arsitektur yang diharapkan `x86_64` dan versi `NADI Agent 0.7.5-dev`.

## 2. Jalankan installer

Dari direktori hasil ekstraksi:

```bash
sudo ./scripts/install-simpiav2.sh
```

Installer akan membuat/memastikan user `nadi-agent`, direktori standar, memasang binary, menyiapkan file environment jika belum ada, memasang unit systemd, dan menjalankan `systemctl daemon-reload`. Binary lama akan dicadangkan dengan timestamp. Existing `state.json` tidak disentuh.

Installer sengaja **tidak** menjalankan atau mengaktifkan service.

## 3. Preflight state dan permission

Jalankan:

```bash
sudo stat -c '%a %U:%G %n' \
    /var/lib/nadi-agent \
    /var/lib/nadi-agent/state.json \
    /etc/nadi-agent/keys/activation-public.pem \
    /opt/nadi/bin/nadi-agent
```

Ekspektasi:

- `/var/lib/nadi-agent`: `700 nadi-agent:nadi-agent`
- `state.json`: `600 nadi-agent:nadi-agent`
- Activation public key: `644 root:root`
- Binary: `755 root:root`

Jangan menjalankan `cat` atau `jq` terhadap `state.json`; jangan menyalin atau mengirimkan isinya karena memuat credential.

Jika `state.json` belum ada, **jangan start service**. Jangan melakukan enrollment ulang otomatis; periksa status registrasi dan state dengan prosedur operasional NADI terlebih dahulu.

## 4. Aktifkan service setelah preflight

```bash
sudo systemctl enable --now nadi-agent
sudo systemctl status nadi-agent --no-pager
sudo journalctl -u nadi-agent -n 100 --no-pager
```

Control Poll tetap berjalan terpisah dari Activation Gate. Collector dan pengiriman telemetry hanya boleh berjalan ketika grant yang valid mengizinkan scope terkait.

## 5. Pemeriksaan setelah start

```bash
/opt/nadi/bin/nadi-agent --version
sudo systemctl is-active nadi-agent
sudo journalctl -u nadi-agent --since '10 minutes ago' --no-pager
```

Jangan menganggap telemetry sudah tersimpan di Central hanya karena service aktif atau log menyatakan request terkirim. Konfirmasi penerimaan dan penyimpanan perlu dilakukan melalui bukti ACK/record Central.

## 6. Rollback binary

Hentikan service, lalu gunakan backup binary yang dibuat installer. Jangan restore atau mengganti `state.json` untuk rollback binary.

```bash
sudo systemctl stop nadi-agent
ls -1t /opt/nadi/bin/nadi-agent.before-0.7.5-dev.*
```

Pilih backup yang benar setelah memeriksa versinya. Jangan mengembalikan state lama karena dapat mengembalikan credential yang sudah tidak aktif.

## Batasan build

Build ini menyediakan jalur batch JSON untuk record `server_metrics` dan `application_metrics` yang sudah ada di spool. Stream Nginx khusus, HTTP request/error events, redaction HTTP event, penanganan quarantine lengkap untuk seluruh status HTTP, serta pemecahan batch 413 belum dinyatakan selesai. Build ini belum melewati E2E terhadap Central sungguhan; karena itu statusnya development/pilot, bukan production-ready.
