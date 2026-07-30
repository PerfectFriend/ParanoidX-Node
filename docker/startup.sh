#!/bin/bash
#
# ParanoidX — clean production startup orchestrator
# - Prepares host directories with correct ownership (no root-owned spam)
# - Starts Docker stack with persistent .onion via bind mounts + custom Tor
# - Robust first-boot fingerprint bootstrap (clean container + docker cp)
# - Extracts real SMP/XFTP client addresses with fingerprints
# - Generates real PNG QR codes + ASCII QR in terminal
# - Writes addresses.json + client-connection-info.txt
# - Prints the beautiful final block
# - Starts the Go dashboard

set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

COMPOSE="docker compose"
ERRORS=0

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log()    { echo -e "[$(date '+%H:%M:%S')] $*"; }
success(){ echo -e "${GREEN}✅ $*${NC}"; }
warn()   { echo -e "${YELLOW}⚠️  $*${NC}"; }
error()  { echo -e "${RED}❌ $*${NC}"; ERRORS=$((ERRORS+1)); }
section(){ echo ""; echo "════════════════════════════════════════════════════════════════"; echo "  $1"; echo "════════════════════════════════════════════════════════════════"; }

# === Paths (edit if your layout differs) ===
DASHBOARD_BIN="/home/tomas/bin/ParanoidX"
DASHBOARD_LISTEN="0.0.0.0:8080"
DASHBOARD_DATA_DIR="/home/tomas/.local/share/ParanoidX"
TOR_DIR="$DASHBOARD_DATA_DIR/tor"

mkdir -p "$DASHBOARD_DATA_DIR" "$TOR_DIR" "$SCRIPT_DIR/tor/hidden_services/smp" "$SCRIPT_DIR/tor/hidden_services/xftp" "$SCRIPT_DIR/tor/hidden_services/dashboard" "$SCRIPT_DIR/tor/hidden_services/ice" "$SCRIPT_DIR/tor/hidden_services/auditor"

# === Prepare directories owned by the REAL user (even when the script is run with sudo) ===
# This was the main cause of "tor: Permission denied" + restart loops in the previous run.
prepare_directories() {
    # Detect the real desktop user (works correctly under sudo)
    local real_user="${SUDO_USER:-}"
    if [ -z "$real_user" ] || [ "$real_user" = "root" ]; then
        real_user=$(stat -c '%U' "$HOME" 2>/dev/null || echo "$USER")
    fi
    if [ -z "$real_user" ] || [ "$real_user" = "root" ]; then
        real_user="$USER"
    fi

    local uid
    local gid
    uid=$(id -u "$real_user" 2>/dev/null || echo 1000)
    gid=$(id -g "$real_user" 2>/dev/null || echo 1000)

    log "Preparing persistent directories for user: $real_user ($uid:$gid)"

    # Parent must be traversable (755) so the container user 1000 can reach the subdirs
    mkdir -p "$SCRIPT_DIR/tor/hidden_services"
    chown -R "$uid:$gid" "$SCRIPT_DIR/tor/hidden_services" 2>/dev/null || true
    chmod 755 "$SCRIPT_DIR/tor/hidden_services" 2>/dev/null || true

    for d in smp xftp dashboard ice auditor; do
        mkdir -p "$SCRIPT_DIR/tor/hidden_services/$d"
        chown -R "$uid:$gid" "$SCRIPT_DIR/tor/hidden_services/$d" 2>/dev/null || true
        chmod 700 "$SCRIPT_DIR/tor/hidden_services/$d" 2>/dev/null || true
    done

    # Data dir for Go dashboard + QR PNGs + lock.json
    mkdir -p "$DASHBOARD_DATA_DIR" "$TOR_DIR" "$DASHBOARD_DATA_DIR/logs"
    chown -R "$uid:$gid" "$DASHBOARD_DATA_DIR" 2>/dev/null || true
    chmod 700 "$DASHBOARD_DATA_DIR" 2>/dev/null || true
    chmod 700 "$TOR_DIR" 2>/dev/null || true

    # Vault (2GB logical quota + physical .reserved for apparent full usage, per A0/Island plan)
    mkdir -p "$DASHBOARD_DATA_DIR/vault"
    if [ ! -f "$DASHBOARD_DATA_DIR/vault/.reserved" ]; then
        fallocate -l 2G "$DASHBOARD_DATA_DIR/vault/.reserved" 2>/dev/null || \
        dd if=/dev/zero of="$DASHBOARD_DATA_DIR/vault/.reserved" bs=1M count=2048 status=none 2>/dev/null || true
        chmod 600 "$DASHBOARD_DATA_DIR/vault/.reserved" 2>/dev/null || true
    fi
    chown -R "$uid:$gid" "$DASHBOARD_DATA_DIR/vault" 2>/dev/null || true
    chmod 700 "$DASHBOARD_DATA_DIR/vault" 2>/dev/null || true

    # TRON treasury for USDT TRC20 funding of silver rounds (Island plan: USDT -> broker silver -> TLR tokenization rounds + dividends to banknote holders as shares)
    if [ ! -f "$DASHBOARD_DATA_DIR/tron_treasury.txt" ]; then
        echo "# Set your TRON address for USDT TRC20 treasury (receives inflows to fund broker silver purchases for next TLR/silver batch tokenization)" > "$DASHBOARD_DATA_DIR/tron_treasury.txt"
        echo "TYourRealTronTreasuryAddressForUSDT" >> "$DASHBOARD_DATA_DIR/tron_treasury.txt"
        chmod 600 "$DASHBOARD_DATA_DIR/tron_treasury.txt" 2>/dev/null || true
    fi
    chown "$uid:$gid" "$DASHBOARD_DATA_DIR/tron_treasury.txt" 2>/dev/null || true

    # Sample banknote holders registry for demo dividends (banknotes as shares: holders get ng silver from each new tokenized batch per Island plan)
    if [ ! -f "$DASHBOARD_DATA_DIR/banknotes_registry.json" ]; then
        cat > "$DASHBOARD_DATA_DIR/banknotes_registry.json" << 'JEOF'
[
  {"serial": "A197-001", "denomination_tlr": 1, "holder": "demo-citizen1", "claimed": true},
  {"serial": "B195-010", "denomination_tlr": 10, "holder": "demo-citizen2", "claimed": true}
]
JEOF
        chmod 600 "$DASHBOARD_DATA_DIR/banknotes_registry.json" 2>/dev/null || true
    fi
    chown "$uid:$gid" "$DASHBOARD_DATA_DIR/banknotes_registry.json" 2>/dev/null || true

    # Sample rwa for tokenization of everything demo (silver ng backing for physical items: coins, banknotes, NFC passports)
    if [ ! -f "$DASHBOARD_DATA_DIR/rwa_registry.json" ]; then
        echo '[{"id":"rwa-demo-001","type":"silver_coin","serial":"DEMO-COIN-1oz","backing_ng":31103480000,"holder":"demo","issued":"demo"}]' > "$DASHBOARD_DATA_DIR/rwa_registry.json"
        chmod 600 "$DASHBOARD_DATA_DIR/rwa_registry.json" 2>/dev/null || true
    fi
    chown "$uid:$gid" "$DASHBOARD_DATA_DIR/rwa_registry.json" 2>/dev/null || true

    # Royal node marker (per Island plan: this node has full control over tokenization/rounds/dividends/RWA for the network; subs use SMP for commands)
    if [ ! -f "$DASHBOARD_DATA_DIR/royal.enabled" ]; then
        echo "# Royal node enabled - controls tokenization of everything, silver rounds, dividends to banknote shares, RWA issuance. Other nodes are subordinate." > "$DASHBOARD_DATA_DIR/royal.enabled"
        chmod 600 "$DASHBOARD_DATA_DIR/royal.enabled" 2>/dev/null || true
    fi
    chown "$uid:$gid" "$DASHBOARD_DATA_DIR/royal.enabled" 2>/dev/null || true

    # Coturn TLS certs (will be generated later, but prepare dir and placeholders to avoid mount issues)
    mkdir -p "$SCRIPT_DIR/coturn"
    touch "$SCRIPT_DIR/coturn/turn_cert.pem" "$SCRIPT_DIR/coturn/turn_key.pem" 2>/dev/null || true
    chown -R "$uid:$gid" "$SCRIPT_DIR/coturn" 2>/dev/null || true
    chmod 755 "$SCRIPT_DIR/coturn" 2>/dev/null || true
    chmod 644 "$SCRIPT_DIR/coturn/turn_cert.pem" 2>/dev/null || true
    chmod 600 "$SCRIPT_DIR/coturn/turn_key.pem" 2>/dev/null || true
}

# === First-boot bootstrap for official smp-server / xftp-server (prevents "fingerprint withFile does not exist") ===
bootstrap_smp_if_needed() {
    if [ -f "$SCRIPT_DIR/smp_configs/fingerprint" ] && [ -s "$SCRIPT_DIR/smp_configs/fingerprint" ]; then
        return 0
    fi
    log "First boot: bootstrapping SMP config with clean ephemeral container..."
    docker run --rm \
        -v "$SCRIPT_DIR/smp_configs:/etc/opt/simplex" \
        -v "$SCRIPT_DIR/smp_state:/var/opt/simplex" \
        simplexchat/smp-server:latest \
        /bin/sh -c 'smp-server init -y || true; sleep 1' >/dev/null 2>&1 || true
    sleep 1
    if [ -f "$SCRIPT_DIR/smp_configs/fingerprint" ]; then
        success "SMP fingerprint bootstrapped"
    else
        warn "SMP bootstrap may have failed — will retry on container start"
    fi
}

bootstrap_xftp_if_needed() {
    if [ -f "$SCRIPT_DIR/xftp_configs/fingerprint" ] && [ -s "$SCRIPT_DIR/xftp_configs/fingerprint" ]; then
        return 0
    fi
    log "First boot: bootstrapping XFTP config with clean ephemeral container..."
    docker run --rm \
        -v "$SCRIPT_DIR/xftp_configs:/etc/opt/simplex-xftp" \
        -v "$SCRIPT_DIR/xftp_state:/srv/xftp" \
        simplexchat/xftp-server:latest \
        /bin/sh -c 'xftp-server init -y || true; sleep 1' >/dev/null 2>&1 || true
    sleep 1
    if [ -f "$SCRIPT_DIR/xftp_configs/fingerprint" ]; then
        success "XFTP fingerprint bootstrapped"
    else
        warn "XFTP bootstrap may have failed — will retry on container start"
    fi
}

# === Generate self-signed certs so smp-server stops complaining and restarting ===
# The official image **requires** these exact files for its static web server on port 443.
# Missing or root-owned certs cause the repeated "no HTTPS credentials" error and container restarts.
ensure_smp_certs() {
    local cert_dir="$SCRIPT_DIR/smp_configs/certificates"
    mkdir -p "$cert_dir"

    local crt="$cert_dir/ParanoidX.local.crt"
    local key="$cert_dir/ParanoidX.local.key"

    # Detect the real desktop user (works correctly under sudo)
    local real_user="${SUDO_USER:-}"
    if [ -z "$real_user" ] || [ "$real_user" = "root" ]; then
        real_user=$(stat -c '%U' "$HOME" 2>/dev/null || echo "$USER")
    fi
    local uid=$(id -u "$real_user" 2>/dev/null || echo 1000)
    local gid=$(id -g "$real_user" 2>/dev/null || echo 1000)

    # Always fix ownership first (previous runs left files as root:root)
    chown -R "$uid:$gid" "$cert_dir" 2>/dev/null || true

    # Always (re)generate the certs on every startup for maximum reliability.
    # They are self-signed and long-lived anyway.
    log "Ensuring fresh self-signed web certificate for smp-server..."
    if command -v openssl >/dev/null 2>&1; then
        openssl req -x509 -newkey rsa:4096 \
            -keyout "$key" -out "$crt" \
            -days 3650 -nodes -sha256 \
            -subj "/CN=ParanoidX.local" 2>/dev/null || true

        chmod 600 "$key" 2>/dev/null || true
        chmod 644 "$crt" 2>/dev/null || true
        chown -R "$uid:$gid" "$cert_dir" 2>/dev/null || true

        success "Self-signed certificate ready at $cert_dir (owned by $real_user)"
    else
        warn "openssl not found — smp-server may keep spamming the HTTPS cert error"
    fi
}

# === Get onion preferring host bind mount (true persistence) ===
get_onion() {
    local host_dir=$1
    local container_dir=$2
    local name=$3

    # 1. Host file first (true persistence)
    if [ -f "$host_dir/hostname" ] && [ -s "$host_dir/hostname" ]; then
        local val
        val=$(cat "$host_dir/hostname" 2>/dev/null | tr -d '\r\n')
        if [[ "$val" == *.onion ]]; then
            echo "$val"
            return 0
        fi
    fi

    # 2. Try docker cp (works even if container is a bit flaky)
    mkdir -p "$host_dir"
    if docker cp "ParanoidX-tor:${container_dir}/." "$host_dir/" 2>/dev/null; then
        chmod 700 "$host_dir" 2>/dev/null || true
        chmod 600 "$host_dir"/hs_ed25519* "$host_dir"/hostname 2>/dev/null || true
        chown -R "$(id -u):$(id -g)" "$host_dir" 2>/dev/null || true

        local val
        val=$(cat "$host_dir/hostname" 2>/dev/null | tr -d '\r\n')
        if [[ "$val" == *.onion ]]; then
            echo "$val"
            return 0
        fi
    fi

    # 3. Exec as last resort — completely silence errors so they never leak into variables
    local val
    val=$(docker compose exec -T tor cat "$container_dir/hostname" 2>/dev/null | tr -d '\r\n' || true)
    if [[ "$val" == *.onion ]]; then
        echo "$val" > "$host_dir/hostname"
        docker compose exec -T tor cat "$container_dir/hs_ed25519_secret_key" 2>/dev/null > "$host_dir/hs_ed25519_secret_key" || true
        chmod 600 "$host_dir"/* 2>/dev/null || true
        chown -R "$(id -u):$(id -g)" "$host_dir" 2>/dev/null || true
        echo "$val"
        return 0
    fi

    return 1
}

# === Main flow ===
prepare_directories

# Re-detect the same uid/gid that prepare_directories used (the function uses locals)
real_user_for_export="${SUDO_USER:-}"
if [ -z "$real_user_for_export" ] || [ "$real_user_for_export" = "root" ]; then
    real_user_for_export=$(stat -c '%U' "$HOME" 2>/dev/null || echo "$USER")
fi
export SIMPLEX_UID=$(id -u "$real_user_for_export" 2>/dev/null || echo 1000)
export SIMPLEX_GID=$(id -g "$real_user_for_export" 2>/dev/null || echo 1000)

ensure_smp_certs
bootstrap_smp_if_needed
bootstrap_xftp_if_needed

# === Restore Tor onion keys from backup (защита от потери адресов) ===
# Бекап в /home/tomas/.local/share/ParanoidX/tor-keys-backup/ — вне git
if [ -x "$SCRIPT_DIR/restore-tor-keys.sh" ]; then
    section "Restoring Tor hidden service keys (onion address persistence)"
    "$SCRIPT_DIR/restore-tor-keys.sh" || warn "restore-tor-keys.sh exited with error"
fi

section "Starting Docker Compose stack"

# Always (re)build the custom tor image so we definitely get the fixed entrypoint.sh
# (without the old "mkdir web" that was causing permission crashes).
if ! $COMPOSE up -d --build --remove-orphans 2>&1; then
    error "docker compose up failed"
    exit 1
fi
success "Docker stack started"

# After Docker has potentially auto-created mount points as root,
# force the correct ownership again (this is the nuclear option that finally works).
fix_ownership_after_up() {
    local real_user="${SUDO_USER:-}"
    if [ -z "$real_user" ] || [ "$real_user" = "root" ]; then
        real_user=$(stat -c '%U' "$HOME" 2>/dev/null || echo "$USER")
    fi
    local uid=$(id -u "$real_user" 2>/dev/null || echo 1000)
    local gid=$(id -g "$real_user" 2>/dev/null || echo 1000)

    for d in smp xftp dashboard ice auditor; do
        if [ -d "$SCRIPT_DIR/tor/hidden_services/$d" ]; then
            chown "$uid:$gid" "$SCRIPT_DIR/tor/hidden_services/$d" 2>/dev/null || true
            chmod 700 "$SCRIPT_DIR/tor/hidden_services/$d" 2>/dev/null || true
            # Also fix any files that may have been created as root
            chown "$uid:$gid" "$SCRIPT_DIR/tor/hidden_services/$d"/* 2>/dev/null || true
            chmod 600 "$SCRIPT_DIR/tor/hidden_services/$d"/hs_ed25519* "$SCRIPT_DIR/tor/hidden_services/$d"/hostname 2>/dev/null || true
        fi
    done

    # Fix certs for ICE TLS
    if [ -d "$SCRIPT_DIR/coturn" ]; then
        chown "$uid:$gid" "$SCRIPT_DIR/coturn/turn_cert.pem" "$SCRIPT_DIR/coturn/turn_key.pem" 2>/dev/null || true
        chmod 644 "$SCRIPT_DIR/coturn/turn_cert.pem" 2>/dev/null || true
        chmod 600 "$SCRIPT_DIR/coturn/turn_key.pem" 2>/dev/null || true
    fi
}
fix_ownership_after_up

# Also fix ownership on smp_state / xftp_state bind mounts (official images often write as root or high-uid inside container).
# Without this the Go dashboard cannot compute storage sizes → rich cards stay stuck on "Попытка X...".
fix_state_ownership() {
    local real_user="${SUDO_USER:-}"
    if [ -z "$real_user" ] || [ "$real_user" = "root" ]; then
        real_user=$(stat -c '%U' "$HOME" 2>/dev/null || echo "$USER")
    fi
    local uid=$(id -u "$real_user" 2>/dev/null || echo 1000)
    local gid=$(id -g "$real_user" 2>/dev/null || echo 1000)

    for d in smp_state xftp_state; do
        if [ -d "$SCRIPT_DIR/$d" ]; then
            chown -R "$uid:$gid" "$SCRIPT_DIR/$d" 2>/dev/null || true
            chmod -R u+rwX "$SCRIPT_DIR/$d" 2>/dev/null || true
        fi
    done
    # Also the certificates dir written during ensure_smp_certs
    if [ -d "$SCRIPT_DIR/smp_configs/certificates" ]; then
        chown -R "$uid:$gid" "$SCRIPT_DIR/smp_configs/certificates" 2>/dev/null || true
    fi
}
fix_state_ownership

# The certs may have been (re)generated just now. Restart smp-server so it picks them up cleanly.
# This is the most reliable way to stop the "no HTTPS credentials" restart loop.
log "Restarting smp-server to apply fresh certificates..."
$COMPOSE restart smp-server >/dev/null 2>&1 || true

# Wait properly for smp-server to become stable before trying to read fingerprint.
# This is critical — without this the fingerprint read almost always times out.
wait_for_smp_healthy() {
    log "Waiting for smp-server to stabilize (this can take 30-90 seconds)..."
    local max_wait=90
    local waited=0
    local stable_count=0

    while [ $waited -lt $max_wait ]; do
        if $COMPOSE ps smp-server 2>/dev/null | grep -q "Up"; then
            stable_count=$((stable_count + 1))
            if [ $stable_count -ge 3 ]; then
                success "smp-server is stable"
                return 0
            fi
        else
            stable_count=0
        fi
        sleep 3
        waited=$((waited + 3))
    done

    warn "smp-server is still not fully stable after ${max_wait}s (fingerprint may fail)"
}
wait_for_smp_healthy

# Generate or load TURN auth secret EARLY (before docker up so that coturn starts with correct secret)
ICE_SECRET_FILE="$DASHBOARD_DATA_DIR/ice_turn_secret.txt"
if [ ! -f "$ICE_SECRET_FILE" ]; then
    openssl rand -base64 32 | tr -d '\n' | head -c 40 > "$ICE_SECRET_FILE"
    chmod 600 "$ICE_SECRET_FILE" 2>/dev/null || true
fi
ICE_TURN_SECRET=$(cat "$ICE_SECRET_FILE")
if [ -f "$SCRIPT_DIR/coturn/turnserver.conf" ]; then
    sed -i "s/static-auth-secret=.*/static-auth-secret=$ICE_TURN_SECRET/" "$SCRIPT_DIR/coturn/turnserver.conf" 2>/dev/null || true
fi

section "Waiting for persistent .onion addresses (host bind mounts)"

ONION_SMP=""
ONION_XFTP=""
ONION_DASHBOARD=""
ONION_ICE=""
SMP_CLIENT=""
XFTP_CLIENT=""

for i in {1..45}; do
    if [ -z "$ONION_SMP" ]; then
        ONION_SMP=$(get_onion "$SCRIPT_DIR/tor/hidden_services/smp" "/var/lib/tor/smp" "smp")
    fi
    if [ -z "$ONION_XFTP" ]; then
        ONION_XFTP=$(get_onion "$SCRIPT_DIR/tor/hidden_services/xftp" "/var/lib/tor/xftp" "xftp")
    fi
    if [ -z "$ONION_DASHBOARD" ]; then
        ONION_DASHBOARD=$(get_onion "$SCRIPT_DIR/tor/hidden_services/dashboard" "/var/lib/tor/dashboard" "dashboard")
    fi
    if [ -z "$ONION_ICE" ]; then
        ONION_ICE=$(get_onion "$SCRIPT_DIR/tor/hidden_services/ice" "/var/lib/tor/ice" "ice")
    fi
    if [ -z "$ONION_AUDITOR" ]; then
        ONION_AUDITOR=$(get_onion "$SCRIPT_DIR/tor/hidden_services/auditor" "/var/lib/tor/auditor" "auditor")
    fi
    if [[ -n "$ONION_SMP" && -n "$ONION_XFTP" && -n "$ONION_DASHBOARD" && -n "$ONION_ICE" && -n "$ONION_AUDITOR" ]]; then
        break
    fi
    sleep 2
done

if [[ -n "$ONION_SMP" ]]; then success "SMP onion: $ONION_SMP"; else error "SMP onion not ready"; fi
if [[ -n "$ONION_XFTP" ]]; then success "XFTP onion: $ONION_XFTP"; else error "XFTP onion not ready"; fi
if [[ -n "$ONION_DASHBOARD" ]]; then success "Dashboard onion: $ONION_DASHBOARD"; else warn "Dashboard onion not ready yet"; fi
if [[ -n "$ONION_ICE" ]]; then success "ICE/TURN onion: $ONION_ICE"; else warn "ICE/TURN onion not ready yet"; fi
if [[ -n "$ONION_AUDITOR" ]]; then success "Auditor onion: $ONION_AUDITOR"; else warn "Auditor onion not ready yet"; fi

# === Backup Tor keys after successful extraction (keep backup fresh) ===
BACKUP_BASE="/home/tomas/.local/share/ParanoidX/tor-keys-backup"
for d in smp xftp dashboard ice auditor; do
    BIND_DIR="$SCRIPT_DIR/tor/hidden_services/$d"
    KEY_FILE="$BIND_DIR/hs_ed25519_secret_key"
    BACKUP_DIR="$BACKUP_BASE/$d"
    if [ -f "$KEY_FILE" ]; then
        mkdir -p "$BACKUP_DIR"
        cp "$KEY_FILE" "$BACKUP_DIR/"
        cp "$BIND_DIR/hs_ed25519_public_key" "$BACKUP_DIR/" 2>/dev/null
        cp "$BIND_DIR/hostname" "$BACKUP_DIR/" 2>/dev/null
        chmod 600 "$BACKUP_DIR"/* 2>/dev/null || true
    fi
done

# === Extract real fingerprints and build full client addresses ===
# More patient with SMP because the server can still be starting up.
section "Extracting fingerprints and building client addresses"

get_fingerprint() {
    local service=$1
    local path=$2
    local max_wait=45
    local waited=0
    local fp=""

    while [ $waited -lt $max_wait ]; do
        fp=$(docker compose exec -T "$service" cat "$path" 2>/dev/null | tr -d '\r\n' | head -c 120)
        if [[ -n "$fp" ]]; then
            echo "$fp"
            return 0
        fi
        sleep 2
        waited=$((waited + 2))
    done
    return 1
}

if [[ -n "$ONION_SMP" ]]; then
    FP=$(get_fingerprint smp-server /etc/opt/simplex/fingerprint)
    if [[ -n "$FP" ]]; then
        SMP_CLIENT="smp://${FP}@${ONION_SMP}:5223"
        echo "$SMP_CLIENT" > "$TOR_DIR/smp_client_address.txt"
        echo "$SMP_CLIENT" > "$DASHBOARD_DATA_DIR/smp_client_address.txt"
        success "SMP client address ready"
    else
        warn "Could not read SMP fingerprint (server may still be starting)"
    fi
fi

if [[ -n "$ONION_XFTP" ]]; then
    FPX=$(get_fingerprint xftp-server /etc/opt/simplex-xftp/fingerprint)
    if [[ -n "$FPX" ]]; then
        XFTP_CLIENT="xftp://${FPX}@${ONION_XFTP}:443"
        echo "$XFTP_CLIENT" > "$TOR_DIR/xftp_client_address.txt"
        echo "$XFTP_CLIENT" > "$DASHBOARD_DATA_DIR/xftp_client_address.txt"
        success "XFTP client address ready"
    else
        warn "Could not read XFTP fingerprint yet"
    fi
fi

if [[ -n "$ONION_DASHBOARD" ]]; then
    echo "$ONION_DASHBOARD" > "$TOR_DIR/dashboard_onion.txt"
    echo "$ONION_DASHBOARD" > "$DASHBOARD_DATA_DIR/dashboard_onion.txt"
    success "Dashboard onion address ready: $ONION_DASHBOARD"
fi

if [[ -n "$ONION_ICE" ]]; then
    echo "$ONION_ICE" > "$TOR_DIR/ice_onion.txt"
    echo "$ONION_ICE" > "$DASHBOARD_DATA_DIR/ice_onion.txt"
    success "ICE/TURN onion address ready: $ONION_ICE"
fi

if [[ -n "$ONION_AUDITOR" ]]; then
    echo "$ONION_AUDITOR" > "$TOR_DIR/auditor_onion.txt"
    echo "$ONION_AUDITOR" > "$DASHBOARD_DATA_DIR/auditor_onion.txt"
    success "Auditor onion address ready: $ONION_AUDITOR"
fi

# Write secret for Go server (already generated above)
echo "$ICE_TURN_SECRET" > "$TOR_DIR/ice_turn_secret.txt" 2>/dev/null || true

# Generate self-signed TLS cert for the ICE .onion (for turns://)
generate_ice_cert() {
    local onion=$1
    local cert_dir="$SCRIPT_DIR/coturn"
    mkdir -p "$cert_dir"
    if [ ! -f "$cert_dir/turn_cert.pem" ] || [ ! -f "$cert_dir/turn_key.pem" ]; then
        openssl req -x509 -newkey rsa:2048 -keyout "$cert_dir/turn_key.pem" -out "$cert_dir/turn_cert.pem" \
            -days 3650 -nodes -subj "/CN=$onion" -addext "subjectAltName=DNS:$onion" 2>/dev/null || \
        openssl req -x509 -newkey rsa:2048 -keyout "$cert_dir/turn_key.pem" -out "$cert_dir/turn_cert.pem" \
            -days 3650 -nodes -subj "/CN=$onion"
        chmod 600 "$cert_dir/turn_key.pem" 2>/dev/null || true
        chmod 644 "$cert_dir/turn_cert.pem" 2>/dev/null || true
    fi
}
if [[ -n "$ONION_ICE" ]]; then
    generate_ice_cert "$ONION_ICE"
    # Copy cert to data dir so it can be served or distributed
    [ -f "$SCRIPT_DIR/coturn/turn_cert.pem" ] && cp -f "$SCRIPT_DIR/coturn/turn_cert.pem" "$DASHBOARD_DATA_DIR/turn_cert.pem" 2>/dev/null || true
    [ -f "$SCRIPT_DIR/coturn/turn_key.pem" ] && cp -f "$SCRIPT_DIR/coturn/turn_key.pem" "$DASHBOARD_DATA_DIR/turn_key.pem" 2>/dev/null || true
    success "ICE/TURN TLS cert generated for $ONION_ICE"
    # Restart coturn to pick up new cert and updated secret in conf
    $COMPOSE restart coturn >/dev/null 2>&1 || true
fi

# === Real QR PNGs (for dashboard /static/ serving) + ASCII in terminal ===
section "Generating QR codes"

if command -v qrencode >/dev/null 2>&1; then
    [ -n "$SMP_CLIENT" ]  && qrencode -s 6 -o "$DASHBOARD_DATA_DIR/qr-smp.png"  "$SMP_CLIENT"  2>/dev/null || true
    [ -n "$XFTP_CLIENT" ] && qrencode -s 6 -o "$DASHBOARD_DATA_DIR/qr-xftp.png" "$XFTP_CLIENT" 2>/dev/null || true
    [ -n "$ONION_DASHBOARD" ] && qrencode -s 6 -o "$DASHBOARD_DATA_DIR/qr-dashboard.png" "http://$ONION_DASHBOARD" 2>/dev/null || true
    [ -n "$ONION_ICE" ] && qrencode -s 6 -o "$DASHBOARD_DATA_DIR/qr-ice.png" "turn:$ONION_ICE:3478?transport=tcp" 2>/dev/null || true
    [ -n "$ONION_AUDITOR" ] && qrencode -s 6 -o "$DASHBOARD_DATA_DIR/qr-auditor.png" "http://$ONION_AUDITOR/auditor" 2>/dev/null || true
    success "PNG QR codes written to $DASHBOARD_DATA_DIR"
else
    warn "qrencode not installed — PNG QR will be missing (install: sudo apt install qrencode)"
fi

# === addresses.json for dashboard (matches /api/addresses response format) ===
CONTACT_LINK=$(cat "$DASHBOARD_DATA_DIR/island_contact_link.txt" 2>/dev/null || echo "")
cat > "$DASHBOARD_DATA_DIR/addresses.json" <<EOF
{
  "smp": "${SMP_CLIENT:-}",
  "xftp": "${XFTP_CLIENT:-}",
  "ice": "${ONION_ICE:-}",
  "auditor": "${ONION_AUDITOR:-}",
  "contact": "${CONTACT_LINK:-}"
}
EOF
cp -f "$DASHBOARD_DATA_DIR/addresses.json" "$SCRIPT_DIR/addresses.json" 2>/dev/null || true

# Copy the beautiful full dashboard (the one with exact lock flow, truncation, real QR, rotation button)
cp -f "$SCRIPT_DIR/dashboard.html" "$DASHBOARD_DATA_DIR/dashboard.html" 2>/dev/null || true
chmod 644 "$DASHBOARD_DATA_DIR/dashboard.html" 2>/dev/null || true
chown "$(id -u):$(id -g)" "$DASHBOARD_DATA_DIR/dashboard.html" 2>/dev/null || true

# === client-connection-info.txt (the file people actually copy from) ===
{
    echo "═══════════════════════════════════════════════════════════════════════════════"
    echo "  SimpleX Node — ГОТОВЫЕ АДРЕСА ДЛЯ КЛИЕНТА"
    echo "  Создано: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "═══════════════════════════════════════════════════════════════════════════════"
    echo ""
    echo "SMP (сообщения):"
    echo "  $SMP_CLIENT"
    echo ""
    echo "XFTP (файлы и медиа):"
    echo "  $XFTP_CLIENT"
    echo ""
    echo "───────────────────────────────────────────────────────────────────────────────"
    echo "📞 ГОЛОСОВЫЕ И ВИДЕО ЗВОНКИ (WebRTC через скрытый TURN)"
    echo "  ICE/TURN onion: ${ONION_ICE:-<не готов>}"
    echo ""
    echo "  НАСТРОЙКА В КЛИЕНТЕ SimpleX (обязательно для реальных звонков):"
    echo "    1. Настройки → Аудио и видео звонки → WebRTC ICE серверы"
    echo "    2. Включи тумблер «Настроить ICE серверы»"
    echo "    3. Вставь строки (по одной). Бери СВЕЖИЕ из дашборда:"
    echo "         http://127.0.0.1:8080   или через dashboard onion"
    echo "       (карточка ICE — кнопки COPY tcp / turns с актуальными кредами ~12ч)"
    echo "    tcp версия (без сертификата) обычно достаточно:"
    echo "       turn:<user>:<cred>@${ONION_ICE:-<onion>}:3478?transport=tcp"
    echo ""
    echo "  Чтобы звонки использовали ЭТОТ узел:"
    echo "    - Создавай новые контакты с SMP адресом этого узла (выше)."
    echo "    - Или в существующем чате: информация о контакте → Change receiving address"
    echo "      и укажи smp адрес этого сервера."
    echo "    Call signaling (offer/answer) идёт через SMP, медиа — через наш TURN."
    echo "───────────────────────────────────────────────────────────────────────────────"
    echo "Как добавить в SimpleX Chat:"
    echo "  Настройки → Сеть и серверы → Добавить сервер"
    echo "  Вставьте оба адреса выше (SMP + XFTP)."
    echo "═══════════════════════════════════════════════════════════════════════════════"
} > "$SCRIPT_DIR/client-connection-info.txt"
chown "$(id -u):$(id -g)" "$SCRIPT_DIR/client-connection-info.txt" 2>/dev/null || true
success "client-connection-info.txt обновлён"

# === Append Island / silver funnel / royal / TRON vision (A0+ deep integration) ===
cat >> "$SCRIPT_DIR/client-connection-info.txt" << 'VIS'
───────────────────────────────────────────────────────────────────────────────
🏝️  THE ISLAND PROJECT / SAINT MARY LIBERTY ISLAND  +  SILVER BLACK HOLE FUNNEL
  Нода — цифровая инфраструктура суверенного микрогосударства (stmaria.org).
  • Приватные коммуникации граждан (SimpleX + скрытые сервисы + TURN для голоса/видео).
  • Глобальная библиотека/медиа (Vault 2GB + будущие радио-стримы из vault + анонсы раундов + anon channels).
  • Экономика: USDT TRC20 (люди несут на treasury-адрес в tron_treasury.txt) → накопление на оплату брокеру → поступление физ. серебра в резерв Острова → токенизация партии (нанограммы серебра) → 20% в казначейство Острова, 80% pro-rata дивиденды в ng серебра держателям зарегистрированных физических банкнот Mark Bank (банкноты = АКЦИИ / equity shares).
  • Королевская нода (royal.enabled): особенная, с полным контролем над токенизацией всего (rounds, RWA, dividends) для сети нод. Саб-ноды подчиняются (будущие signed SMP команды от royal).
  • Математическая воронка / чёрная дыра для мирового серебра: каждый держатель банкнот-акций получает ng с каждой новой партии. Чем больше народу держит/регистрирует банкноты ради yield — тем больше капитала (USDT) притекает для покупки следующей партии у брокера → больше серебра в резерв → больше будущих ng-выплат → ещё больший спрос на банкноты как инвестицию с реальным серебряным yield'ом. Самоусиление: воронка засасывает серебро непрерывно.
  • Инструмент "токенизации всего" (когда публика/инвесторы): серебряные ng из резерва как backing ("заморока") под ценность физических предметов. Первые: серебряные монеты, физические банкноты Mark Bank (как shares), токены дающие право на физический паспорт Острова со встроенным NFC-чипом оплаты/идентификации (island_nfc_passport).
  • Treasury % с каждого выпуска — на нужды Острова (казначейство).
  • Позднее: wallet TLR/silver_ng, billing (цены в ng/TLR за сервисы), radio из vault, монетизируемые каналы, fractional real estate paid in silver/TLR, citizenship reg, company reg, science data и т.д.
  См. дашборд (кнопки Sim/Init/Refresh/Register), /api/treasury/* , /api/rwa/* , silver_reserve_ng.txt, banknotes_registry.json (shares), rwa_registry.json, silver_rounds.log, vault/dividend-*.txt + share-*.txt , royal.enabled, tron_treasury.txt, docs/PLAN-A0.md + docs/ISLAND.md .
VIS
chown "$(id -u):$(id -g)" "$SCRIPT_DIR/client-connection-info.txt" 2>/dev/null || true
success "client-connection-info.txt + Island silver funnel vision appended"

# Append the fresh Island Services contact (if the bot setup produced the real or placeholder link + magic description)
if [ -f "$DASHBOARD_DATA_DIR/island_services_contact.txt" ]; then
  echo "" >> "$SCRIPT_DIR/client-connection-info.txt"
  cat "$DASHBOARD_DATA_DIR/island_services_contact.txt" >> "$SCRIPT_DIR/client-connection-info.txt"
  success "Island Services contact (SimpleX entry point to the whole Treasure) appended to client-connection-info.txt"
fi

# === Island Royal Services Bot (SimpleX Chat protocol as transport for ALL services) ===
# The magic of the Soul of the Treasure Island becomes accessible to citizens.
# One contact in stock SimpleX app → wallet, radio (vault anns + round news), vault, market, tokenizer of everything,
# digital ID/NFC passports, channels, full E2EE private access to the silver black-hole economy.
# Uses official simplex-chat CLI (bot profile + WS gateway) + bridge (later in this run or Go).
# Called after our SMP is known so the bot can be bound to the Island's own relay.
log "Preparing Island Royal Services bot (the living voice of the Treasure)..."
if [ -x "$SCRIPT_DIR/scripts/island-bot-setup.sh" ]; then
  # Pass through env so it finds our SMP address file written above
  SMP_CLIENT_FOR_BOT="${SMP_CLIENT:-}" "$SCRIPT_DIR/scripts/island-bot-setup.sh" || warn "island-bot-setup had issues (CLI download or tor?) — will retry on next launch or via bot cmd"
  success "Island bot setup attempted (profile + CLI WS gateway). Real contact link will appear in dashboard + client-info after bridge."
else
  warn "island-bot-setup.sh not found or not executable — create it per client Phase 6 plan"
fi

# === Health ===
section "Health checks"
$COMPOSE ps --format "table {{.Service}}\t{{.Status}}"

# More patient final health check for smp-server (it can be slow to fully stabilize)
if $COMPOSE ps smp-server 2>/dev/null | grep -q "Up"; then
    success "smp-server running"
else
    # Give it one more chance
    sleep 5
    if $COMPOSE ps smp-server 2>/dev/null | grep -q "Up"; then
        success "smp-server running"
    else
        error "smp-server not running"
    fi
fi

$COMPOSE ps | grep -q "xftp-server.*Up"  && success "xftp-server running"  || error "xftp-server not running"
$COMPOSE ps | grep -q "tor.*Up"          && success "tor running"          || error "tor not running"

# === THE BEAUTIFUL FINAL BLOCK (what user sees and loves) ===
echo ""
echo "╔════════════════════════════════════════════════════════════════════════════╗"
echo "║               SIMPLEX NODE — ГОТОВЫЕ АДРЕСА ДЛЯ КЛИЕНТА                    ║"
echo "╚════════════════════════════════════════════════════════════════════════════╝"
echo ""

if [[ -n "$ONION_DASHBOARD" ]]; then
    echo "🖥️   Дашборд (через Tor):  http://$ONION_DASHBOARD"
    echo "    Локально:              http://localhost:8080"
    echo ""
fi

if [[ -n "$SMP_CLIENT" ]]; then
    echo "✅  SMP СЕРВЕР (сообщения)"
    echo "   $SMP_CLIENT"
    echo ""
    if command -v qrencode >/dev/null 2>&1; then
        echo "📱  QR-код SMP (отсканируй в SimpleX):"
        qrencode -t UTF8 "$SMP_CLIENT"
        echo ""
    fi
fi

if [[ -n "$XFTP_CLIENT" ]]; then
    echo "✅  XFTP СЕРВЕР (файлы и медиа)"
    echo "   $XFTP_CLIENT"
    echo ""
    if command -v qrencode >/dev/null 2>&1; then
        echo "📱  QR-код XFTP (отсканируй в SimpleX):"
        qrencode -t UTF8 "$XFTP_CLIENT"
        echo ""
    fi
fi

if [[ -n "$ONION_ICE" ]]; then
    echo "📞  ICE/TURN ДЛЯ ГОЛОСОВЫХ И ВИДЕО ЗВОНКОВ (WebRTC)"
    echo "   Onion TURN: $ONION_ICE"
    echo "   Полные строки с кредами (обновляются ~12 часов) — бери в дашборде:"
    echo "     http://127.0.0.1:8080  (или dashboard onion) → карточка ICE → COPY tcp"
    echo "   Пример (tcp без cert):"
    echo "     turn:<user>:<cred>@$ONION_ICE:3478?transport=tcp"
    echo "   В SimpleX: Настройки → Аудио и видео звонки → WebRTC ICE серверы"
    echo "   (включи настройку и вставь 1-2 строки). Используй tcp-версию в первую очередь."
    echo ""
fi

echo "──────────────────────────────────────────────────────────────────────────────"
echo "📄  Полная информация для копирования:"
echo "   $SCRIPT_DIR/client-connection-info.txt"
echo ""
echo "🔄  Чтобы сгенерировать НОВЫЕ адреса: нажмите кнопку в дашборде или"
echo "   вручную:  rm -rf tor/hidden_services/smp tor/hidden_services/xftp && ./startup.sh"
echo "══════════════════════════════════════════════════════════════════════════════"
echo ""

if [[ $ERRORS -eq 0 ]]; then
    success "Startup завершён успешно. Onion адреса persistent на диске."
else
    error "$ERRORS ошибок — смотри вывод выше."
fi

# === Start the Go dashboard (the web UI) ===
section "Starting Go Dashboard"
if [[ -x "$DASHBOARD_BIN" ]]; then
    if pgrep -f "ParanoidX.*8080" >/dev/null 2>&1; then
        success "Go dashboard already running"
    else
        mkdir -p "$DASHBOARD_DATA_DIR/logs"
        nohup "$DASHBOARD_BIN" -listen "$DASHBOARD_LISTEN" -data "$DASHBOARD_DATA_DIR" \
            > "$DASHBOARD_DATA_DIR/logs/dashboard.log" 2>&1 &
        sleep 1.2
        if pgrep -f "ParanoidX.*8080" >/dev/null 2>&1; then
            success "Go dashboard started on $DASHBOARD_LISTEN"
        else
            warn "Go dashboard failed to start (see $DASHBOARD_DATA_DIR/logs/dashboard.log)"
        fi
    fi
else
    warn "Go binary not found at $DASHBOARD_BIN — build it with: go build -o $DASHBOARD_BIN ./cmd/ParanoidX"
    echo "   After build, re-run this script or start manually:"
    echo "   $DASHBOARD_BIN -listen 0.0.0.0:8080 -data $DASHBOARD_DATA_DIR"
fi

echo ""
echo "Готово. Откройте дашборд: http://localhost:8080   или через onion выше."
echo ""

exit $ERRORS
