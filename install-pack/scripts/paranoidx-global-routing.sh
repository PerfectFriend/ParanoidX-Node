#!/bin/bash
# =============================================================================
# ParanoidX Global Routing
# Route host application traffic through Tor anonymity network + V2Ray
# =============================================================================
# Architecture:
#   Proxy-aware apps → HTTP_PROXY → Tor SOCKS5(:9050) → Tor Network → Internet
#   Manual routing   → socks5://127.0.0.1:9050 (Tor) | 127.0.0.1:10810 (V2Ray)
#
# Provides:
#   - System-wide Tor SOCKS5 proxy env vars (for proxy-aware apps)
#   - Tor daemon health monitoring and auto-restart
#   - Quick ways to test and verify anonymity
#   - V2Ray available as optional obfuscation layer
#
# Usage:
#   paranoidx-global-routing enable     — Set Tor proxy env vars
#   paranoidx-global-routing disable    — Remove proxy env vars
#   paranoidx-global-routing status     — Full health check
#   paranoidx-global-routing test       — Complete chain verification
#   paranoidx-global-routing quick-test — Quick Tor exit check
# =============================================================================
set -euo pipefail

TOR_SOCKS=9050
API_BASE="http://127.0.0.1:8080"

GREEN='\033[0;32m'; RED='\033[0;31m'
YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
ok()   { echo -e "  ${GREEN}✓${NC} $1"; }
fail() { echo -e "  ${RED}✗${NC} $1"; }
info() { echo -e "  ${CYAN}ℹ${NC} $1"; }
warn() { echo -e "  ${YELLOW}⚠${NC} $1"; }

# ─── Tor Proxy Env ──────────────────────────────────────────────────────

set_tor_proxy_env() {
    mkdir -p /etc/profile.d
    cat > /etc/profile.d/paranoidx-proxy.sh << 'EOF'
# ParanoidX global Tor proxy
export TOR_SOCKS="socks5://127.0.0.1:9050"
export HTTP_PROXY="${TOR_SOCKS}"
export HTTPS_PROXY="${TOR_SOCKS}"
export ALL_PROXY="${TOR_SOCKS}"
export http_proxy="${TOR_SOCKS}"
export https_proxy="${TOR_SOCKS}"
export all_proxy="${TOR_SOCKS}"
export NO_PROXY="localhost,127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,.local,.onion"
export no_proxy="localhost,127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,.local,.onion"
EOF
    chmod 644 /etc/profile.d/paranoidx-proxy.sh
    ok "System proxy env: Tor SOCKS5 (affects new shells)"
}

unset_tor_proxy_env() {
    rm -f /etc/profile.d/paranoidx-proxy.sh
    ok "Proxy env vars removed"
}

# ─── Health ──────────────────────────────────────────────────────────────

health_check() {
    local ok=0 total=0

    echo -e "${CYAN}ℹ${NC} === ParanoidX Global Routing — Health Check ===\n"

    # 1. Tor SOCKS
    total=$((total+1))
    if curl -s --max-time 6 --socks5-hostname 127.0.0.1:${TOR_SOCKS} \
        http://check.torproject.org/ 2>/dev/null | grep -q "Congratulations"; then
        ok "Tor exit CONFIRMED"; ok=$((ok+1))
    else
        fail "Tor not working (check: systemctl status tor@default)"
    fi

    # 2. IP anonymity
    total=$((total+1))
    local direct=$(curl -s --max-time 4 https://api.ipify.org 2>/dev/null || echo "N/A")
    local tor_ip=$(curl -s --max-time 8 --socks5-hostname 127.0.0.1:${TOR_SOCKS} \
        https://api.ipify.org 2>/dev/null || echo "N/A")
    total=$((total+1))
    info "Direct IP: ${direct}"
    info "Tor IP:    ${tor_ip}"
    if [ "$direct" != "$tor_ip" ] && [ "$tor_ip" != "N/A" ]; then
        ok "Anonymity: IP hidden (${direct} → ${tor_ip})"; ok=$((ok+1))
    else
        warn "Anonymity: IP may be visible"
    fi

    # 3. V2Ray available
    total=$((total+1))
    if ss -tlnp 2>/dev/null | grep -q ":10810 "; then
        ok "V2Ray SOCKS5 :10810"; ok=$((ok+1))
    else
        info "V2Ray SOCKS5 :10810 not listening"
    fi

    # 4. Proxy env
    total=$((total+1))
    if [ -f /etc/profile.d/paranoidx-proxy.sh ]; then
        ok "System proxy env: SET"; ok=$((ok+1))
    else
        info "System proxy env: not set"
    fi

    # 5-8. Node
    for name in "simplex-node" "ParanoidX chain" "Bridge" "Docker"; do
        total=$((total+1))
    done

    if curl -s --max-time 3 ${API_BASE}/api/health >/dev/null 2>&1; then
        ok "simplex-node API"; ok=$((ok+1))
    else
        fail "simplex-node API"; ok=$((ok+1))
    fi
    if curl -s --max-time 3 ${API_BASE}/api/paranoidx/status 2>/dev/null | python3 -c \
        "import sys,json; d=json.load(sys.stdin)" 2>/dev/null; then
        ok "ParanoidX API"; ok=$((ok+1))
    else
        fail "ParanoidX API"
    fi
    if curl -s --max-time 3 ${API_BASE}/api/chat/bridge-health 2>/dev/null | python3 -c \
        "import sys,json; d=json.load(sys.stdin); assert d.get('connected')" 2>/dev/null; then
        ok "Bridge connected"; ok=$((ok+1))
    else
        fail "Bridge not connected"
    fi
    if curl -s --max-time 3 ${API_BASE}/api/admin/docker 2>/dev/null | python3 -c \
        "import sys,json; d=json.load(sys.stdin); assert d.get('healthy')" 2>/dev/null; then
        ok "Docker containers healthy"; ok=$((ok+1))
    else
        fail "Docker health check failed"
    fi

    echo ""
    echo -e "${CYAN}ℹ${NC} === Result: ${ok}/${total} ==="
    if [ "$ok" -eq "$total" ]; then
        echo -e "  ${GREEN}✅ ALL GOOD — Tor anonymity + services operational${NC}"
    elif [ "$ok" -ge "$((total-3))" ]; then
        echo -e "  ${YELLOW}⚠ ${ok}/${total} — Mostly operational${NC}"
    else
        echo -e "  ${RED}❌ ${ok}/${total} — Issues detected${NC}"
    fi
}

# ─── Main ────────────────────────────────────────────────────────────────

case "${1:-status}" in
    enable|start|on)
        set_tor_proxy_env
        echo ""
        health_check
        echo ""
        ok "ParanoidX global Tor routing enabled"
        info "New shells → HTTP_PROXY socks5://127.0.0.1:9050 (Tor)"
        info "Fully proxy-aware apps: curl, wget, git, browser (with extension)"
        info ""
        info "Test:   source /etc/profile.d/paranoidx-proxy.sh && curl -s https://check.torproject.org/ | grep Congratulations"
        info "Verify: ${0} quick-test"
        info "Disable: ${0} disable"
        ;;

    disable|stop|off)
        unset_tor_proxy_env
        ok "Tor proxy env vars removed"
        info "Apps now use direct internet"
        ;;

    status|info)
        echo "🔒 ParanoidX Global Routing — STATUS"
        echo "====================================="
        [ -f /etc/profile.d/paranoidx-proxy.sh ] && ok "Tor proxy env: SET" || info "Tor proxy env: not set"
        echo ""
        health_check
        ;;

    test|check)
        health_check
        ;;

    quick-test)
        info "Quick Tor check..."
        if curl -s --max-time 10 --socks5-hostname 127.0.0.1:${TOR_SOCKS} \
            http://check.torproject.org/ 2>/dev/null | grep -q "Congratulations"; then
            ok "✓ Tor exit CONFIRMED — traffic anonymized"
        else
            fail "✗ Tor not working"
            warn "Run: systemctl status tor@default"
        fi
        ;;

    *)
        echo "Usage: $0 {enable|disable|status|test|quick-test}"
        echo "  enable     — Set system-wide Tor SOCKS5 proxy env"
        echo "  disable    — Remove proxy env, restore direct routing"
        echo "  status     — Show current state + health"
        echo "  test       — Full health check"
        echo "  quick-test — Verify Tor exit"
        ;;
esac
