#!/usr/bin/env bash
set -Eeuo pipefail

PACKAGE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BINARY_SOURCE="${PACKAGE_ROOT}/bin/linux-amd64/nadi-agent"
UNIT_SOURCE="${PACKAGE_ROOT}/systemd/nadi-agent.service"
BINARY_TARGET="/opt/nadi/bin/nadi-agent"
UNIT_TARGET="/etc/systemd/system/nadi-agent.service"
ENV_TARGET="/etc/nadi-agent/agent.env"
STATE_DIR="/var/lib/nadi-agent"
KEY_FILE="/etc/nadi-agent/keys/activation-public.pem"

if [[ "$(id -u)" -ne 0 ]]; then
    echo "ERROR: jalankan script ini dengan sudo/root." >&2
    exit 1
fi

if [[ "$(uname -m)" != "x86_64" ]]; then
    echo "ERROR: paket ini untuk Linux x86_64; arsitektur terdeteksi: $(uname -m)." >&2
    exit 1
fi

if [[ ! -x "${BINARY_SOURCE}" ]]; then
    echo "ERROR: binary AMD64 tidak ditemukan: ${BINARY_SOURCE}" >&2
    exit 1
fi

if [[ ! -f "${UNIT_SOURCE}" ]]; then
    echo "ERROR: unit systemd tidak ditemukan: ${UNIT_SOURCE}" >&2
    exit 1
fi

if [[ ! -s "${KEY_FILE}" ]]; then
    echo "ERROR: Activation public key tidak ditemukan: ${KEY_FILE}" >&2
    echo "Installer tidak membuat atau mengganti trust anchor Central." >&2
    exit 1
fi

if [[ ! -s "${STATE_DIR}/state.json" ]]; then
    echo "ERROR: state.json enrollment tidak ditemukan di ${STATE_DIR}." >&2
    echo "Installer dihentikan agar tidak memasang service dengan identitas Agent kosong." >&2
    exit 1
fi

state_owner="$(stat -c '%U:%G' "${STATE_DIR}/state.json")"
state_mode="$(stat -c '%a' "${STATE_DIR}/state.json")"
if [[ "${state_owner}" != "nadi-agent:nadi-agent" || "${state_mode}" != "600" ]]; then
    echo "ERROR: permission state.json harus 600 nadi-agent:nadi-agent (saat ini ${state_mode} ${state_owner})." >&2
    echo "Perbaiki ownership/permission melalui prosedur aman; jangan tampilkan isi file." >&2
    exit 1
fi

if ! id nadi-agent >/dev/null 2>&1; then
    useradd --system --home-dir /var/lib/nadi-agent --shell /usr/sbin/nologin nadi-agent
fi

install -d -o root -g root -m 0755 /opt/nadi /opt/nadi/bin
install -d -o root -g root -m 0755 /etc/nadi-agent
install -d -o root -g root -m 0755 /etc/nadi-agent/keys
install -d -o nadi-agent -g nadi-agent -m 0700 "${STATE_DIR}"

if [[ ! -e "${ENV_TARGET}" ]]; then
    cat > "${ENV_TARGET}" <<'ENVEOF'
NADI_CENTRAL_URL=https://nadi.pesantrenalirsyad.org
NADI_STATE_DIR=/var/lib/nadi-agent
NADI_ACTIVATION_PUBLIC_KEY=/etc/nadi-agent/keys/activation-public.pem
NADI_ACTIVATION_KEY_ID=nadi-central-2026-01
NADI_LOG_LEVEL=info
NADI_POLL_INTERVAL_SECONDS=30
NADI_HTTP_TIMEOUT_SECONDS=10
ENVEOF
    chown root:nadi-agent "${ENV_TARGET}"
    chmod 0640 "${ENV_TARGET}"
fi

if [[ -e "${BINARY_TARGET}" ]]; then
    backup="${BINARY_TARGET}.before-0.7.5-dev.$(date -u +%Y%m%dT%H%M%SZ)"
    cp -a "${BINARY_TARGET}" "${backup}"
    echo "Binary lama disimpan di: ${backup}"
fi

install -o root -g root -m 0755 "${BINARY_SOURCE}" "${BINARY_TARGET}.new"
"${BINARY_TARGET}.new" --version
mv -f "${BINARY_TARGET}.new" "${BINARY_TARGET}"

install -o root -g root -m 0644 "${UNIT_SOURCE}" "${UNIT_TARGET}"
chown root:root "${KEY_FILE}"
chmod 0644 "${KEY_FILE}"
chown nadi-agent:nadi-agent "${STATE_DIR}"
chmod 0700 "${STATE_DIR}"

systemctl daemon-reload

echo
echo "NADI Agent 0.7.5-dev berhasil dipasang (belum di-enable/start)."
echo "Pastikan state.json hasil enrollment ACTIVE tersedia tanpa menampilkan isinya."
echo "Periksa: sudo stat -c '%a %U:%G %n' ${STATE_DIR} ${STATE_DIR}/state.json"
echo "Setelah preflight, aktifkan dengan: sudo systemctl enable --now nadi-agent"
echo "Pantau dengan: sudo journalctl -u nadi-agent -f"
