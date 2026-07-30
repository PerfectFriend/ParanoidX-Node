#!/bin/sh
# Tor entrypoint for ParanoidX.
#
# This entrypoint starts as root (we removed "user:" from compose for tor).
# It fixes ownership on the bind-mounted HiddenServiceDirs (they often arrive as root:root
# because Docker creates missing bind targets as root), prepares /tmp/tor-data,
# then uses su-exec to drop privileges and run tor as your normal desktop user.
# Result: the .onion keys on your host are always owned by you (not root).

set -e

PERSISTENT="/var/lib/tor"
RUNTIME="/tmp/tor-data"

# Target UID/GID for the tor process and all created files (set by startup.sh)
TARGET_UID="${SIMPLEX_UID:-1000}"
TARGET_GID="${SIMPLEX_GID:-1000}"

echo "[tor-entrypoint] Running as root (uid $(id -u)). Will drop to ${TARGET_UID}:${TARGET_GID}"

# 1. Force-correct the persistent HiddenService directories (bind mounts).
# Being root here makes this reliable no matter what state the host dirs are in.
mkdir -p "$PERSISTENT/smp" "$PERSISTENT/xftp" "$PERSISTENT/dashboard" "$PERSISTENT/ice" "$PERSISTENT/auditor"

chown -R "${TARGET_UID}:${TARGET_GID}" "$PERSISTENT" 2>/dev/null || true
chmod 755 "$PERSISTENT" 2>/dev/null || true

for d in smp xftp dashboard ice auditor; do
    chown -R "${TARGET_UID}:${TARGET_GID}" "$PERSISTENT/$d" 2>/dev/null || true
    chmod 700 "$PERSISTENT/$d" 2>/dev/null || true
done

# Tighten key files if they exist
chmod 600 "$PERSISTENT"/smp/hs_ed25519_secret_key \
           "$PERSISTENT"/xftp/hs_ed25519_secret_key \
           "$PERSISTENT"/dashboard/hs_ed25519_secret_key \
           "$PERSISTENT"/ice/hs_ed25519_secret_key \
           "$PERSISTENT"/auditor/hs_ed25519_secret_key 2>/dev/null || true

# 2. Ephemeral DataDirectory for tor runtime (must be writable by the target user)
mkdir -p "$RUNTIME"
chown -R "${TARGET_UID}:${TARGET_GID}" "$RUNTIME" 2>/dev/null || true
chmod 700 "$RUNTIME" 2>/dev/null || true

echo "[tor-entrypoint] Directories fixed. Dropping privileges and starting tor as ${TARGET_UID}:${TARGET_GID}..."

# 3. Drop to the normal user and exec tor (clean, no shell left behind)
exec su-exec "${TARGET_UID}:${TARGET_GID}" tor -f /etc/tor/torrc "$@"
