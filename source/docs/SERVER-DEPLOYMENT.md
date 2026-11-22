# NADI Agent v0.7.0-dev — Panduan Penerapan Credential Rotation

## Status dan batasan

Build ini menambahkan Agent-side credential delivery retrieve, persist, switch,
ACK, serta pemulihan state setelah restart. Paket ini adalah development build.
Jangan menyebut rotasi Central berhasil sebelum respons ACK aktual diterima dan
status credential di Central diverifikasi.

GET contract yang dipakai:

- `GET /api/v1/nadi/agent/credentials/deliveries/{deliveryId}`
- Bearer credential lama
- JSON `success` dan `data.delivery_id`, `data.credential_id`, `data.token`,
  `data.expires_at`

ACK contract yang dipakai:

- `POST /api/v1/nadi/agent/credentials/deliveries/{deliveryId}/ack`
- Bearer credential baru
- JSON `success` dan `data.agent_id`, `data.credential_id`,
  `data.key_version`, `data.status`, `data.expires_at`

GET saat ini tidak menyertakan `key_version` maupun `delivery_status`; Agent
memvalidasi field yang benar-benar tersedia dan tidak membuat asumsi tambahan.
ACK response divalidasi terhadap Agent ID dan credential ID lokal, key version
positif, status `active`, dan expiry credential baru yang masih berlaku.

## Prasyarat

- Agent v0.6.4-dev yang sudah enrolled, dengan `state.json` valid.
- Delivery pending sudah dibuat Central dan belum expired.
- Binary v0.7.0-dev untuk arsitektur server tersedia.
- Akses sudo terbatas untuk menghentikan dan menjalankan kembali service.
- Service menggunakan user `nadi-agent` dan state directory yang sama dengan
  instalasi aktif.
- `NADI_AGENT_TOKEN` tidak boleh di-set pada proses rotasi karena akan
  meng-override credential tersimpan.

## 1. Verifikasi paket dan arsitektur

Jalankan di direktori tempat ZIP dan checksum disimpan:

```bash
sha256sum -c nadi-agent-v0.7.0-dev-package-SHA256SUMS.txt
unzip -l nadi-agent-v0.7.0-dev-package.zip
```

Pilih binary sesuai `uname -m`:

```bash
uname -m
```

- `x86_64`: `bin/linux-amd64/nadi-agent`
- `aarch64` atau `arm64`: `bin/linux-arm64/nadi-agent`

## 2. Backup binary dan state

Hentikan Agent untuk melepas exclusive state lock. Jangan menghapus atau
mengedit state file.

```bash
sudo systemctl stop nadi-agent
sudo install -d -o root -g root -m 0755 /var/backups/nadi-agent
sudo cp -a /opt/nadi/bin/nadi-agent /var/backups/nadi-agent/nadi-agent.before-0.7.0-dev
sudo cp -a /var/lib/nadi-agent/state.json /var/backups/nadi-agent/state.json.before-credential-rotation
sudo chmod 0600 /var/backups/nadi-agent/state.json.before-credential-rotation
```

Jika instalasi memakai path berbeda, gunakan path aktual dari unit systemd.
Backup state mengandung credential; simpan hanya pada lokasi root-only yang
terlindungi dan jangan mengirimkannya ke pihak lain.

## 3. Pasang binary

Contoh untuk Linux amd64:

```bash
sudo install -o root -g root -m 0755 bin/linux-amd64/nadi-agent /opt/nadi/bin/nadi-agent.new
sudo /opt/nadi/bin/nadi-agent.new --version
sudo mv /opt/nadi/bin/nadi-agent.new /opt/nadi/bin/nadi-agent
```

Untuk ARM64, ganti sumber binary menjadi `bin/linux-arm64/nadi-agent`.

## 4. Jalankan rotasi delivery yang diberikan Central

Delivery ID untuk pengujian ini:

`3b26f2d6-448a-4c39-b224-359a30c4b38e`

Jalankan dengan user runtime Agent. Perintah ini menggunakan credential lama
dari state lokal untuk GET, menyimpan credential baru secara atomik, mengganti
credential aktif, lalu mengirim ACK menggunakan credential baru.

```bash
sudo -u nadi-agent env -u NADI_AGENT_TOKEN \
    NADI_STATE_DIR=/var/lib/nadi-agent \
    NADI_CENTRAL_URL=https://nadi.pesantrenalirsyad.org \
    /opt/nadi/bin/nadi-agent \
    --rotate-credential \
    --delivery-id 3b26f2d6-448a-4c39-b224-359a30c4b38e
```

Jangan menambahkan token ke command line, environment, atau file konfigurasi.
Agent tidak menampilkan token. Perintah sukses berarti Agent menerima dan
memvalidasi ACK response sesuai schema yang tersedia; tetap verifikasi record
Central untuk memastikan credential lama revoked dan credential baru active.

## 5. Nyalakan kembali service

Jika perintah rotasi berhasil:

```bash
sudo systemctl start nadi-agent
sudo systemctl status nadi-agent --no-pager
sudo journalctl -u nadi-agent -n 100 --no-pager
```

Jika ACK gagal karena gangguan jaringan, state rotasi tetap tersimpan. Nyalakan
service kembali; Agent akan mencoba ACK menggunakan credential baru sebelum
melanjutkan Control Poll. Jangan jalankan retrieve ulang dan jangan membuat
delivery baru.

## 6. Perilaku restart dan expiry

- Crash sebelum state staging tersimpan: Agent tetap memakai credential lama;
  operator dapat mengulang command dengan delivery ID yang sama.
- Crash setelah staging tetapi sebelum switch: startup memvalidasi expiry,
  lalu melanjutkan switch dan ACK tanpa GET ulang.
- Crash setelah switch/ACK mulai: startup mencoba ACK lagi dengan credential
  baru. Credential lama tetap dipertahankan sampai ACK response tervalidasi.
- ACK gagal sementara: pending rotation disimpan dan dicoba ulang dengan
  exponential backoff; Control Poll dijeda sampai ACK selesai.
- Delivery expired sebelum switch: staged token dibuang, credential lama
  dipertahankan, dan ACK tidak dikirim.
- Delivery expired setelah ACK mulai: hasil bisa ambigu karena Central mungkin
  sudah memproses ACK tetapi response hilang. Agent mempertahankan kedua
  credential dan menjeda Control Poll. Hentikan percobaan otomatis dan
  koordinasikan pemulihan status dengan Central; jangan memaksa ACK atau
  membuat delivery baru.

## 7. Verifikasi tanpa menampilkan secret

Periksa versi binary:

```bash
/opt/nadi/bin/nadi-agent --version
```

Periksa permission state:

```bash
sudo stat -c '%a %U:%G %n' /var/lib/nadi-agent /var/lib/nadi-agent/state.json
```

Hasil yang diharapkan untuk permission adalah direktori `700` dan file `600`.
Jangan menjalankan `cat`, `jq`, atau perintah lain yang menampilkan isi
`state.json`, karena file tersebut berisi credential.

Periksa log tanpa mengekspor atau menyalin data sensitif:

```bash
sudo journalctl -u nadi-agent --since '15 minutes ago' --no-pager
```

## 8. Rollback

Rollback binary hanya aman jika Central mengonfirmasi credential mana yang
masih aktif. Setelah ACK berhasil, binary lama tetap dapat memakai credential
baru yang tersimpan di state, tetapi jangan restore backup `state.json` lama
karena dapat mengembalikan credential yang telah revoked.

Jika rotasi belum ACK dan delivery masih valid, pertahankan state saat ini dan
biarkan Agent melanjutkan retry. Jika status Central tidak diketahui atau
delivery expired setelah ACK dimulai, jangan restore state backup dan jangan
menghapus salah satu credential secara manual. Minta Central memverifikasi
status credential dan memberikan prosedur recovery.

## Catatan kontrak retry Central

Pemulihan setelah respons jaringan hilang mengharuskan retrieve GET untuk
delivery yang sama dapat diulang selama delivery masih pending, serta ACK untuk
delivery yang sama bersifat idempotent atau mengembalikan hasil sukses yang
setara ketika ACK sebelumnya sudah diproses. Handover yang tersedia belum
menyatakan perilaku retry tersebut secara eksplisit. Verifikasi ini diperlukan
pada pengujian end-to-end sebelum fitur dinyatakan production-ready.
