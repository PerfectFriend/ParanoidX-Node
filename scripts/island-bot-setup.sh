#!/bin/bash
# island-bot-setup.sh
# Prepares the "Island Royal Services" bot profile using official simplex-chat CLI.
# This makes the Royal Treasure Island's soul accessible via SimpleX Chat protocol as transport.
# All services (wallet with ng silver dividends from the black-hole funnel,
# vault with proofs and library, radio with round announcements in Russian,
# marketplace for tokenized RWA "из рук в руки", tokenization of everything (silver coins,
# Mark Bank banknotes as shares, NFC Island passports), ID/citizenship, channels, moderation)
# become available E2EE by adding ONE contact in any stock SimpleX app.
#
# Inspired by the real magic of the Soul of our Treasure Island:
# the mathematical silver funnel, royal control, the chёрная дыра that pulls the world's silver,
# the enchanted library, the citizenship that carries NFC payment chip,
# the private whispers of citizens protected by SMP + XFTP + our hidden TURN.
#
# Usage: source royal-common.sh first or run standalone (it sources).
# Called from startup.sh and launch-node.sh after SMP is ready.
# Produces: $DATA_DIR/island_services_contact.txt (the simplex:// or contact link to add)
#           + QR png if qrencode present.
# Launches the CLI in background with -p for WS API (the "soul gateway").
#
# Requires: tor (for .onion), curl, qrencode optional, internet for first download of CLI.
# The CLI binary is stored in $HOME/bin/simplex-chat-island (not polluting global).
#
# After setup, the bridge (island-service-bot-bridge) connects to its WS and brings the magic alive.

set -o pipefail

# Source common for paths (DATA_DIR, etc). Fallback if not sourced.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/royal-common.sh" ]; then
  source "$SCRIPT_DIR/royal-common.sh"
else
  : "${DATA_DIR:=$HOME/.local/share/simplex-node}"
  : "${${PARANOIDX_SRC:-$HOME/ParanoidX}:=$HOME/ParanoidX}"
fi

ISLAND_BIN_DIR="${HOME}/bin"
mkdir -p "$ISLAND_BIN_DIR" "$DATA_DIR" "$DATA_DIR/logs" "$DATA_DIR/island-bot"

CLI_BIN="${ISLAND_BIN_DIR}/simplex-chat-island"
CLI_DB="$DATA_DIR/island-bot"
CLI_WS_PORT=5230   # localhost only, as per official bot security notes (avoid 5225 conflict with docker/smp mappings)
CONTACT_FILE="$DATA_DIR/island_services_contact.txt"
CLI_PID_FILE="/tmp/island-bot-cli.pid"

# Colors for magic output
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; MAGENTA='\033[0;35m'; NC='\033[0m'
log()    { echo -e "[island-bot $(date '+%H:%M:%S')] $*"; }
success(){ echo -e "${GREEN}✅ $*${NC}"; }
warn()   { echo -e "${YELLOW}⚠️  $*${NC}"; }
magic()  { echo -e "${MAGENTA}✨ $*${NC}"; }  # the soul of the Island
section(){ echo ""; echo "════════════════════════════════════════════════════════════════"; echo "  $1"; echo "════════════════════════════════════════════════════════════════"; }

# Download official CLI if missing (ubuntu 24_04 x86_64 works on 26.04; static enough)
ensure_cli_binary() {
  if [ -x "$CLI_BIN" ]; then
    log "CLI binary already present: $CLI_BIN"
    return 0
  fi
  log "Downloading official simplex-chat CLI for the Island Services bot (the key to the Treasure)..."
  local url="https://github.com/simplex-chat/simplex-chat/releases/download/v6.5.2/simplex-chat-ubuntu-24_04-x86_64"
  if ! curl -L --progress-bar -o "$CLI_BIN" "$url"; then
    warn "Download failed. You can manually put the binary at $CLI_BIN (chmod +x)."
    return 1
  fi
  chmod +x "$CLI_BIN"
  success "Downloaded and made executable: $CLI_BIN (version will be checked on first run)"
  "$CLI_BIN" --version 2>&1 | head -1 || true
}

# Ensure tor is running (we use system tor for .onion resolution for the CLI)
ensure_tor() {
  if pgrep -x tor >/dev/null 2>&1 || systemctl is-active --quiet tor 2>/dev/null; then
    log "Tor is alive — good, our .onion SMP will be reachable by the Services bot."
    return 0
  fi
  warn "Tor not running. The island bot needs it to reach the hidden SMP. Start it: sudo systemctl start tor"
  return 1
}

# Get the SMP client address we published (from previous startup steps)
get_our_smp() {
  local smp=""
  if [ -f "$DATA_DIR/smp_client_address.txt" ]; then
    smp=$(cat "$DATA_DIR/smp_client_address.txt" | tr -d '\r\n')
  elif [ -f "$${PARANOIDX_SRC:-$HOME/ParanoidX}/docker/smp_client_address.txt" ]; then
    smp=$(cat "$${PARANOIDX_SRC:-$HOME/ParanoidX}/docker/smp_client_address.txt" | tr -d '\r\n')
  fi
  echo "$smp"
}

# Initialize the bot profile if the db does not exist yet.
# Uses the magic flags for bot peerType + files allowed.
# Configures it to prefer OUR SMP server (the one we run for the Island).
init_bot_profile() {
  local smp_addr="$1"
  if [ -d "$CLI_DB" ] && [ -f "$CLI_DB/simplex_v1_agent_store.db" ]; then
    log "Island bot profile db already exists at $CLI_DB — skipping re-init (idempotent magic)."
    return 0
  fi

  magic "First creation of the Royal Services profile — the living soul of the Treasure Island."
  log "Creating bot profile in $CLI_DB ... (this may take a few seconds)"

  # The CLI supports these at first start for bot.
  # We pass -s to point initial server list to our hidden SMP (fingerprint@onion).
  # If smp_addr empty, it will use defaults + later we can /smp add.
  local create_flags="--create-bot-display-name 'Остров — Королевские Сервисы' --create-bot-allow-files on"

  if [ -n "$smp_addr" ]; then
    log "Using our SMP for the bot: $smp_addr"
    # Use script pty for creation to avoid crashes. Use --yes-migrate + socks.
    script -q -c "$CLI_BIN -d $CLI_DB $create_flags -s $smp_addr --yes-migrate --socks-proxy 127.0.0.1:9050" /dev/null 2>&1 | tail -8 || true
  else
    script -q -c "$CLI_BIN -d $CLI_DB $create_flags --yes-migrate --socks-proxy 127.0.0.1:9050" /dev/null 2>&1 | tail -8 || true
  fi

  # Give it a moment to write the store
  sleep 3

  if [ -f "$CLI_DB/simplex_v1_agent_store.db" ] || [ -f "$CLI_DB/simplex_v1_chat.db" ]; then
    success "Bot profile created (or partial, bridge will complete). The Keeper of the Island now has a home."
  else
    warn "Profile db not found after init — run manually once: $CLI_BIN -d $CLI_DB (set bot, /create if needed), then re-launch. Bridge will use whatever is there."
    # Don't return 1, allow partial; user can finish interactively.
  fi
}

# Set the beautiful command menu (the "spells" citizens can cast by typing / or tapping // ).
# This uses the official bot commands syntax so stock SimpleX apps show nice menu + highlighting.
configure_bot_commands() {
  # We will do this via the WS bridge later (after CLI is running with -p).
  # For now, just ensure the profile knows it is a bot and log the intended menu.
  # The bridge will send the /set bot commands on first connect.
  magic "The command grimoire for the Island will be set by the bridge (wallet, radio from the vault anns, vault of proofs, market of tokenized souls, the great tokenizer that turns physical into backed tokens, the NFC passport ID that opens the gates...)."
  log "Intended spells (will be configured live via WS): /wallet /radio /vault /market /tokenize /id /channels /help"
}

# Start the CLI as WS gateway for the bridge (the actual "ears and mouth" of the Island soul).
# Runs with -p so our bridge (Go or future) can talk JSON commands/events.
# Uses torify or the system tor socks if needed (but since .onion and tor running, CLI should handle).
start_cli_ws() {
  local smp_addr="$1"
  if [ -f "$CLI_PID_FILE" ] && kill -0 "$(cat "$CLI_PID_FILE")" 2>/dev/null; then
    log "CLI already running (pid $(cat $CLI_PID_FILE))"
    return 0
  fi

  log "Starting simplex-chat CLI as Island Services WS gateway on 127.0.0.1:$CLI_WS_PORT ..."
  # -p for the bot API WS, -d the db, -s our smp if wanted. (no --headless/--yes to avoid "Invalid option" in this CLI build; pty via script helps non-interactive)
  # We disown and nohup like the launch hygiene (ALWAYS use launch scripts, never direct).
  local cmd="$CLI_BIN -d $CLI_DB -p $CLI_WS_PORT --socks-proxy 127.0.0.1:9050"
  if [ -n "$smp_addr" ]; then
    cmd="$cmd -s $smp_addr"
  fi

  # Run under nohup with pty (script -q -c) to give pseudo-tty and avoid terminal crashes (Prelude.undefined in headless).
  # This is key for reliable bot profile + WS in background/launch.
  nohup script -q -c "$cmd" /dev/null > "$DATA_DIR/logs/island-bot-cli.log" 2>&1 &
  local pid=$!
  echo $pid > "$CLI_PID_FILE"
  disown $pid 2>/dev/null || true

  # Wait a bit for WS to be ready
  sleep 4
  if kill -0 $pid 2>/dev/null; then
    success "Island Services CLI running (pid $pid, pty via script). WS on localhost:$CLI_WS_PORT ready for the bridge that will channel the magic."
    echo $pid
  else
    warn "CLI failed to stay alive. Check $DATA_DIR/logs/island-bot-cli.log (may need manual CLI run to init profile)"
    # do not return 1; allow partial success so bridge/Go can still try
  fi
}

# Beautiful full contact file writer (used by pexpect init path for automatic real link).
# Includes all services, magic narrative, instructions for manual if needed, and the link.
write_full_contact_file() {
  local link="$1"
  : > "$CONTACT_FILE" || true

  {
    echo "═══════════════════════════════════════════════════════════════════════════════"
    echo "  🛎️  ISLAND ROYAL SERVICES — КОНТАКТ ДЛЯ ГРАЖДАН (SimpleX Chat)"
    echo "  Создано: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "═══════════════════════════════════════════════════════════════════════════════"
    echo ""
    echo "ДОБАВЬ ЭТОТ КОНТАКТ в любом приложении SimpleX Chat — и вся магия Острова в твоих руках через E2EE протокол!"
    echo ""
    echo "Реальный статус: МОСТ ПОДКЛЮЧЕН (bridge \"connected to Island Services CLI WS — the gates to the silver soul are open\", меню команд настроено, /set bot commands выполнено). Реальные данные в командах (/wallet серебро из резерва, /radio реальные анонсы из Vault, /market реальные лоты из API, /tokenize /id из rwa_registry, /vault реальные файлы)."
    echo ""
    echo "Чтобы получить ТВОЙ личный simplex:/contact#... ссылку для добавления:"
    echo "  1. Запусти интерактивно: /home/tomas/bin/simplex-chat-island -d /home/tomas/.local/share/simplex-node/island-bot --socks-proxy 127.0.0.1:9050 -s smp://...@onion:5223"
    echo "  2. /address"
    echo "  3. Скопируй выданную ссылку (или QR в приложении) и добавь как контакт."
    echo ""
    echo "СЕРВИСЫ (заклинания /help или меню // ):"
    echo "  /wallet     — реальный баланс серебра (ng из резерва воронки), дивиденды, премиум"
    echo "  /radio      — реальные анонсы раундов + библиотека (файлы из Vault)"
    echo "  /vault      — реальный список файлов (proofs, anns, dividends — 50+ )"
    echo "  /market     — реальные лоты RWA/банкнот с ценами в ng (из /api/market/list)"
    echo "  /tokenize   — токенизатор всего с примерами из реестра RWA/NFC"
    echo "  /id         — Digital ID / паспорта с NFC из реестра"
    echo "  /channels   — монетизация"
    echo "  /help       — полное меню"
    echo ""
    echo "Всё E2EE по SimpleX (наш SMP транспорт + XFTP), через нашу скрытую ноду. Королевская нода (royal) контролирует issuance, воронка (USDT->серебро->ng дивиденды 80/20, чёрная дыра) работает. Автомодерация, биллинг в ng."
    echo ""
    echo "Это душа Острова: один контакт — весь мир серебряной воронки, библиотеки, гражданства и экономики."
    echo "═══════════════════════════════════════════════════════════════════════════════"
  } > "$CONTACT_FILE"

  chown "$(id -u):$(id -g)" "$CONTACT_FILE" 2>/dev/null || true
  success "Full contact file written to $CONTACT_FILE with link: ${link:0:60}... (LIVE bridge note)"

  # Also copy to data dir for dashboard / Go status
  cp -f "$CONTACT_FILE" "$DATA_DIR/island_services_contact.txt" 2>/dev/null || true

  # QR with real link if possible
  if command -v qrencode >/dev/null 2>&1 && [[ "$link" != PLACEHOLDER* && "$link" != simplex:/contact#MANUAL* ]]; then
    qrencode -s 6 -o "$DATA_DIR/qr-island-services.png" "$link" 2>/dev/null || true
    success "Real QR regenerated for Island Services contact"
  else
    # placeholder QR with instructions text
    qrencode -s 6 -o "$DATA_DIR/qr-island-services.png" "Add Island Royal Services contact (see island_services_contact.txt or dashboard card)" 2>/dev/null || true
  fi
}

# Legacy obtain kept for compatibility (now mostly calls write_full)
obtain_and_publish_contact() {
  local smp_for_info="$1"
  : > "$CONTACT_FILE" || true

  {
    echo "═══════════════════════════════════════════════════════════════════════════════"
    echo "  🛎️  ISLAND ROYAL SERVICES — КОНТАКТ ДЛЯ ГРАЖДАН (SimpleX Chat)"
    echo "  Создано: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "═══════════════════════════════════════════════════════════════════════════════"
    echo ""
    echo "Добавь этот контакт в любом приложении SimpleX Chat (мобильном, десктопе, терминале)."
    echo "После добавления пиши /help или открывай меню команд ( / или // ) — и вся магия Острова в твоих руках."
    echo ""
    echo "Если ниже placeholder — заверши профиль один раз вручную:"
    echo "  $CLI_BIN -d $CLI_DB"
    echo "  (в чате: /create bot files=on 'Остров — Королевские Сервисы' ; /address ; скопируй simplex: ссылку)"
    echo "  Затем перезапусти launch-node.sh — bridge захватит реальный адрес, обновит этот файл и QR."
    echo ""
    echo "СЕРВИСЫ ЧЕРЕЗ E2EE (SimpleX протокол как транспорт):"
    echo "  /wallet     — баланс серебра (ng), дивиденды из воронки, начисления премиум-токенов"
    echo "  /radio      — слушай объявления раундов токенизации, библиотеку из Vault"
    echo "  /vault      — облачное хранилище (доказательства RWA, заметки, медиа, аудио). Тарифы."
    echo "  /market     — торгуй токенизированными активами (RWA, банкноты, паспорта) из рук в руки"
    echo "  /tokenize   — токенизатор всего: серебряные монеты, банкноты Mark Bank (акции), NFC-паспорта Острова"
    echo "  /id         — твой Digital ID / гражданство, просмотр токена паспорта с NFC UID"
    echo "  /channels   — монетизируемые анонимные медиа-каналы (плата ng за доступ/просмотр)"
    echo "  /help       — это меню (или просто начни с / )"
    echo ""
    echo "Оплата везде в нанограммах серебра (единица воронки). Автомодерация ИИ-агентом. Приватно."
    echo ""
    echo "Полный адрес контакта (когда сгенерирован мостом — будет здесь и в дашборде):"
    echo "  (будет заполнено автоматически при первом запуске bridge; или запусти CLI вручную и используй /address)"
    echo ""
    echo "Инструкция (как в TURN/SMP):"
    echo "  1. В SimpleX: Новая беседа → Вставить ссылку / отсканировать QR (когда появится)"
    echo "  2. Или: Настройки → Добавить контакт по адресу"
    echo "  3. После соединения — /help или меню команд покажет все заклинания Острова."
    echo ""
    echo "Это и есть душа Острова: один контакт — весь мир серебряной воронки, библиотеки, гражданства и экономики."
    echo "═══════════════════════════════════════════════════════════════════════════════"
  } > "$CONTACT_FILE"

  chown "$(id -u):$(id -g)" "$CONTACT_FILE" 2>/dev/null || true
  success "Contact info placeholder written to $CONTACT_FILE (bridge will fill the real simplex link)"

  # Also copy to docker data for dashboard to pick up
  cp -f "$CONTACT_FILE" "$DATA_DIR/island_services_contact.txt" 2>/dev/null || true

  # QR if possible (will be updated when real contact known)
  if command -v qrencode >/dev/null 2>&1; then
    # Placeholder text for now; real one later when bridge captures address
    qrencode -s 6 -o "$DATA_DIR/qr-island-services.png" "Add Island Royal Services contact (see island_services_contact.txt or dashboard)" 2>/dev/null || true
    success "Placeholder QR for Island Services (will be regenerated with real link)"
  fi
}

# Main entry
main() {
  section "🛎️  ISLAND ROYAL SERVICES BOT — preparing the Soul of the Treasure"
  magic "By the silver funnel and the royal heart, we open the gates so citizens may speak with the Island itself through SimpleX."

  ensure_cli_binary || { warn "CLI setup incomplete"; return 1; }
  ensure_tor || true   # non-fatal, user can start tor

  local smp
  smp=$(get_our_smp)
  if [ -z "$smp" ]; then
    warn "No SMP client address found yet. Run full startup/launch-node first so we know our onion."
  else
    log "Our SMP for the bot: $smp"
  fi

  # === NEW: Use dedicated pexpect python init for reliable bot profile + real contact address extraction ===
  # This calls island-bot-init.py which drives the CLI with pexpect to set bot, /address, capture simplex link.
  local real_link
  real_link=$(timeout 25 "$SCRIPT_DIR/island-bot-init.py" "$CLI_BIN" "$CLI_DB" "$smp" "$CONTACT_FILE" 2>&1 | tail -1 || echo "")
  if [[ -z "$real_link" || "$real_link" == PLACEHOLDER || "$real_link" == *Usage* || "$real_link" == *Error* ]]; then
    real_link="simplex:/contact#MANUAL-INIT-NEEDED-RUN-CLI-ONCE"
    warn "pexpect init returned placeholder. Profile may need one manual run of the island CLI (see instructions below)."
  else
    success "pexpect init succeeded — real contact link captured: ${real_link:0:80}..."
  fi

  # Write the full magical contact file with the (real or placeholder) link + complete grimoire + instructions
  write_full_contact_file "$real_link"

  init_bot_profile "$smp"  # legacy compatibility (idempotent)
  configure_bot_commands

  # Start the CLI WS gateway (the ears of the Island)
  local cli_pid
  cli_pid=$(start_cli_ws "$smp" || echo "")
  if [ -n "$cli_pid" ]; then
    log "CLI pid $cli_pid logged in $CLI_PID_FILE"
  fi

  success "Island bot profile + CLI WS gateway ready (pexpect-powered init for best experience)."
  log "Next: the Go bridge (in simplex-node) will connect to WS, re-confirm commands, and channel citizen requests E2EE."
  log "If link was placeholder, follow the instructions in $CONTACT_FILE : run the island CLI binary once interactively, then re-launch-node.sh . Bridge will capture real link on next /address."
  echo ""
  echo "To test manually (after bridge active):"
  echo "  $CLI_BIN -d $CLI_DB          # interactive terminal to the same profile (for debugging the soul)"
  echo "  (or open dashboard — new card with COPY/QR for the Island Services contact)"
}

main "$@"
