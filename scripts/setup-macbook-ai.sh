#!/bin/bash
# ============================================================================
# setup-macbook-ai.sh
# Превращает MacBook Pro M4 Max 128GB в рабочую лошадку для нейросетей
# ============================================================================
# Запускать НА МАКБУКЕ, который будет стоять в офисе подключённым к питанию.
# После установки opencode подключается к нему через Tailscale по LAN.
#
# Использование:
#   curl -fsSL https://raw.githubusercontent.com/PerfectFriend/ParanoidX/main/scripts/setup-macbook-ai.sh | bash
#   # или скопировать на макбук и запустить:
#   chmod +x setup-macbook-ai.sh && ./setup-macbook-ai.sh
# ============================================================================

set -euo pipefail

# ─── Цвета ─────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[INFO]${NC} $1"; }
ok()    { echo -e "${GREEN}[OK]${NC}   $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
err()   { echo -e "${RED}[ERR]${NC}  $1"; }
header() { echo -e "\n${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"; echo -e "${BLUE}  $1${NC}"; echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"; }

# ─── Проверка: это macOS? ──────────────────────────────────────────────────
if [[ "$(uname)" != "Darwin" ]]; then
  err "Этот скрипт предназначен для macOS (MacBook Pro M4 Max)."
  err "Текущая ОС: $(uname)"
  exit 1
fi

# Проверка: версия чипа
CHIP=$(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo "unknown")
RAM_MB=$(sysctl -n hw.memsize 2>/dev/null | awk '{print $0/1024/1024}')
info "Чип: $CHIP"
info "RAM:  ${RAM_MB} MB"

if (( $(echo "$RAM_MB < 32768" | bc -l) )); then
  warn "Меньше 32GB RAM — MLX backend Ollama может не включиться."
  warn "Рекомендуется 64GB+, у тебя 128GB — должно быть отлично."
fi

echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║  MacBook AI Server Setup                                ║${NC}"
echo -e "${GREEN}║  M4 Max 128GB → Рабочая лошадка для нейросетей          ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""

# ────────────────────────────────────────────────────────────────────────────
# ШАГ 1: Системные настройки — отключение сна, High Power Mode
# ────────────────────────────────────────────────────────────────────────────
header "ШАГ 1/8: Системные настройки (отключение сна, High Power Mode)"

# Отключаем сон (система не уходит в спад, когда подключено питание)
info "Отключаем системный сон на питании..."
sudo pmset -c sleep 0 2>/dev/null || warn "Не удалось отключить сон"
sudo pmset -c displaysleep 0 2>/dev/null || true
sudo pmset -c hibernatemode 0 2>/dev/null || true
sudo pmset -c womp 1 2>/dev/null || true
ok "Системный сон отключён (при подключённом питании)"

# High Power Mode — максимальная производительность GPU
info "Включаем High Power Mode..."
if sudo pmset -a powermode 2 2>/dev/null; then
  ok "High Power Mode включён"
else
  warn "High Power Mode не поддерживается на этой модели или нужен sudo"
  warn "Включи вручную: Системные настройки → Батарея → Режим питания → Высокая производительность"
fi

# Предотвращаем засыпание через caffeinate
info "Настраиваем caffeinate при старте (держит систему в бодрствовании)..."
cat << 'EOF' | sudo tee /Library/LaunchDaemons/com.user.caffeinate.plist > /dev/null
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.user.caffeinate</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/bin/caffeinate</string>
        <string>-d</string>
        <string>-i</string>
        <string>-m</string>
        <string>-s</string>
        <string>-u</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>Nice</key>
    <integer>-10</integer>
</dict>
</plist>
EOF
sudo launchctl load -w /Library/LaunchDaemons/com.user.caffeinate.plist 2>/dev/null || true
ok "caffeinate запущен — MacBook не заснёт"

ok "Шаг 1 завершён"

# ────────────────────────────────────────────────────────────────────────────
# ШАГ 2: Установка Homebrew
# ────────────────────────────────────────────────────────────────────────────
header "ШАГ 2/8: Homebrew"

if command -v brew &>/dev/null; then
  ok "Homebrew уже установлен: $(brew --version 2>&1 | head -1)"
  brew update --quiet 2>/dev/null || true
else
  info "Устанавливаем Homebrew..."
  NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)" || {
    err "Не удалось установить Homebrew"
    exit 1
  }
  # Добавляем brew в PATH для Apple Silicon
  if [[ -f /opt/homebrew/bin/brew ]]; then
    eval "$(/opt/homebrew/bin/brew shellenv)"
  fi
  ok "Homebrew установлен"
fi

# ────────────────────────────────────────────────────────────────────────────
# ШАГ 3: Установка Ollama
# ────────────────────────────────────────────────────────────────────────────
header "ШАГ 3/8: Ollama"

if command -v ollama &>/dev/null; then
  OLLAMA_VER=$(ollama --version 2>&1 || echo "unknown")
  ok "Ollama уже установлен: $OLLAMA_VER"
else
  info "Устанавливаем Ollama через Homebrew..."
  brew install --cask ollama 2>&1 || {
    warn "brew cask не сработал, пробуем прямой скрипт..."
    curl -fsSL https://ollama.com/install.sh | sh || {
      err "Не удалось установить Ollama. Установи вручную: https://ollama.com/download"
      exit 1
    }
  }
  ok "Ollama установлен"
fi

# Останавливаем GUI-версию Ollama, если она запущена как приложение
osascript -e 'tell application "Ollama" to quit' 2>/dev/null || true
sleep 2

# ────────────────────────────────────────────────────────────────────────────
# ШАГ 4: Конфигурация Ollama — MLX backend + доступ по сети
# ────────────────────────────────────────────────────────────────────────────
header "ШАГ 4/8: Конфигурация Ollama (MLX backend, LAN доступ)"

# Убеждаемся, что Ollama 0.19+ (с MLX бэкендом)
OLLAMA_VER=$(ollama --version 2>&1 | grep -oE '[0-9]+\.[0-9]+' | head -1)
if (( $(echo "$OLLAMA_VER >= 0.19" | bc -l 2>/dev/null || echo 0) )); then
  ok "Ollama $OLLAMA_VER+ — MLX backend доступен по умолчанию"
else
  warn "Ollama < 0.19. MLX backend может не работать. Рекомендуется обновить:"
  warn "  brew upgrade --cask ollama"
fi

# Создаём директорию для конфигурации Ollama
OLLAMA_CONFIG_DIR="$HOME/.ollama"
mkdir -p "$OLLAMA_CONFIG_DIR"

# Настраиваем launchd plist для Ollama server
# Параметры:
#   OLLAMA_HOST=0.0.0.0  — доступ со всех интерфейсов (в т.ч. Tailscale)
#   OLLAMA_KEEP_ALIVE=5m — держим модель в памяти 5 минут после последнего запроса
#   OLLAMA_NUM_PARALLEL=4 — параллельные запросы (агентские воркфлоу)
info "Настраиваем launchd plist для Ollama с доступом по LAN..."
cat << EOF | sudo tee /Library/LaunchDaemons/com.ollama.serve.plist > /dev/null
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.ollama.serve</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/ollama</string>
        <string>serve</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>OLLAMA_HOST</key>
        <string>0.0.0.0</string>
        <key>OLLAMA_KEEP_ALIVE</key>
        <string>5m</string>
        <key>OLLAMA_NUM_PARALLEL</key>
        <string>4</string>
        <key>OLLAMA_MAX_LOADED_MODELS</key>
        <string>2</string>
        <key>OLLAMA_DEBUG</key>
        <string>0</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>ThrottleInterval</key>
    <integer>5</integer>
    <key>Nice</key>
    <integer>-5</integer>
    <key>WorkingDirectory</key>
    <string>$HOME</string>
</dict>
</plist>
EOF

# Загружаем plist
sudo launchctl load -w /Library/LaunchDaemons/com.ollama.serve.plist 2>/dev/null || {
  # Если уже загружен — перезагружаем
  sudo launchctl unload /Library/LaunchDaemons/com.ollama.serve.plist 2>/dev/null || true
  sudo launchctl load -w /Library/LaunchDaemons/com.ollama.serve.plist 2>/dev/null || true
}

# Ждём, пока Ollama запустится
info "Ждём запуска Ollama..."
for i in $(seq 1 15); do
  if curl -s http://localhost:11434/api/tags >/dev/null 2>&1; then
    ok "Ollama сервер запущен на 0.0.0.0:11434"
    break
  fi
  if [ "$i" -eq 15 ]; then
    warn "Ollama не запустился за 15 секунд. Проверь вручную: ollama serve"
    warn "Продолжаем установку моделей..."
  fi
  sleep 1
done

# ────────────────────────────────────────────────────────────────────────────
# ШАГ 5: Загрузка моделей
# ────────────────────────────────────────────────────────────────────────────
header "ШАГ 5/8: Загрузка моделей"

declare -A MODELS
MODELS["qwen3-coder-next"]="50GB — КОДИНГ (основная, 80B MoE/3B active, 35 tok/s на M4 Max)"
MODELS["qwen3.5:35b-a3b-coding-nvfp4"]="20GB — БЫСТРАЯ (NVFP4, 107 tok/s на M4 Max в High Power)"
MODELS["qwen3.5:7b"]="4.5GB — ЛЁГКАЯ (для тестов и мелочей)"

info "Будут загружены следующие модели:"
echo ""
for model in "${!MODELS[@]}"; do
  echo -e "  ${CYAN}$model${NC}  — ${MODELS[$model]}"
done
echo ""
warn "Общий размер загрузки: ~75GB. Убедись, что есть место на диске и хороший интернет."
echo ""

read -p "$(echo -e ${YELLOW}"Начать загрузку? [Y/n]: "${NC})" -n 1 -r REPLY
echo ""
if [[ ! "$REPLY" =~ ^[Nn]$ ]]; then
  for model in "${!MODELS[@]}"; do
    header "Загрузка: $model"
    info "Размер: ${MODELS[$model]}"
    info "Это может занять 10-30 минут в зависимости от скорости интернета..."
    if ollama pull "$model" 2>&1; then
      ok "$model загружена"
    else
      err "Ошибка загрузки $model"
      warn "Пропускаем..."
    fi
  done
else
  warn "Загрузка моделей пропущена. Загрузи позже:"
  for model in "${!MODELS[@]}"; do
    echo "  ollama pull $model"
  done
fi

# ────────────────────────────────────────────────────────────────────────────
# ШАГ 6: Памятка по Tailscale
# ────────────────────────────────────────────────────────────────────────────
header "ШАГ 6/8: Tailscale"

if command -v tailscale &>/dev/null; then
  TAILSCALE_IP=$(tailscale ip -4 2>/dev/null || echo "не удалось определить")
  ok "Tailscale установлен, IP: $TAILSCALE_IP"
else
  warn "Tailscale не найден. Установи и подключись:"
  echo "  1. https://tailscale.com/download/mac"
  echo "  2. Войти в свою учетку"
  echo "  3. Убедиться, что simplex-node сервер в той же Tailnet"
  TAILSCALE_IP="<TAILSCALE_IP_МАКБУКА>"
fi

# ────────────────────────────────────────────────────────────────────────────
# ШАГ 7: Скрипты для управления
# ────────────────────────────────────────────────────────────────────────────
header "ШАГ 7/8: Скрипты управления"

SCRIPTS_DIR="$HOME/.ai-server"
mkdir -p "$SCRIPTS_DIR"

# Статус сервера
cat > "$SCRIPTS_DIR/status.sh" << 'SCRIPT'
#!/bin/bash
echo "╔══════════════════════════════════════════════════════════╗"
echo "║  MacBook AI Server — Status                             ║"
echo "╚══════════════════════════════════════════════════════════╝"

echo ""
echo "🔋 Питание:"
pmset -g batt 2>/dev/null | head -5
pmset -g cap 2>/dev/null | grep -i "high\|low\|power" || true
echo ""

echo "🧠 Ollama:"
if curl -s http://localhost:11434/api/tags >/dev/null 2>&1; then
  echo "   Статус: RUNNING (port 11434)"
  echo "   Модели:"
  ollama list 2>/dev/null | awk 'NR>1 {print "     • " $1 " (" $3 ")"}'
else
  echo "   Статус: STOPPED"
fi
echo ""

TAILSCALE_IP=$(tailscale ip -4 2>/dev/null || echo "не в сети")
echo "🌐 Tailscale: $TAILSCALE_IP"
echo ""

echo "💾 Диск:"
df -h / | awk 'NR==2 {print "   " $3 " / " $2 " (" $5 " used)"}'
SCRIPT
chmod +x "$SCRIPTS_DIR/status.sh"

# Рестарт Ollama
cat > "$SCRIPTS_DIR/restart-ollama.sh" << 'SCRIPT'
#!/bin/bash
echo "Перезапуск Ollama..."
sudo launchctl unload /Library/LaunchDaemons/com.ollama.serve.plist 2>/dev/null || true
sleep 2
sudo launchctl load -w /Library/LaunchDaemons/com.ollama.serve.plist 2>/dev/null || true
sleep 3
if curl -s http://localhost:11434/api/tags >/dev/null 2>&1; then
  echo "✅ Ollama перезапущен"
else
  echo "❌ Ошибка: Ollama не запустился"
fi
SCRIPT
chmod +x "$SCRIPTS_DIR/restart-ollama.sh"

# Логи
cat > "$SCRIPTS_DIR/logs.sh" << 'SCRIPT'
#!/bin/bash
echo "Логи Ollama (Ctrl+C для выхода):"
echo "---"
sudo log stream --predicate 'process == "ollama"' --style compact 2>/dev/null || {
  echo "Нет доступа к логам через log stream."
  echo "Проверь процесс:"
  ps aux | grep -i ollama | grep -v grep
}
SCRIPT
chmod +x "$SCRIPTS_DIR/logs.sh"

# Info для подключения
cat > "$SCRIPTS_DIR/connection-info.sh" << 'SCRIPT'
#!/bin/bash
echo "╔══════════════════════════════════════════════════════════════════════╗"
echo "║  MacBook AI — подключение через Tailscale                          ║"
echo "╚══════════════════════════════════════════════════════════════════════╝"
echo ""

TAILSCALE_IP=$(tailscale ip -4 2>/dev/null || echo "НЕ В СЕТИ")
echo "1. Tailscale IP макбука:  $TAILSCALE_IP"
echo "   Проверить: tailscale status"
echo ""
echo "2. Ollama API endpoint:    http://$TAILSCALE_IP:11434"
echo "   Проверить с сервера:    curl http://$TAILSCALE_IP:11434/api/tags"
echo ""
echo "3. Подключение в opencode (на сервере simplex-node):"
echo ""
echo "   ---"
echo "   # В файле opencode.json добавить:"
echo '   {'
echo '     "customEndpoints": {'
echo "       \"macbook-ai\": {"
echo '         "baseUrl": "http://'$TAILSCALE_IP':11434",'
echo '         "models": {'
echo '           "default": "qwen3-coder-next",'
echo '           "fast": "qwen3.5:35b-a3b-coding-nvfp4",'
echo '           "light": "qwen3.5:7b"'
echo '         }'
echo '       }'
echo '     }'
echo '   }'
echo "   ---"
echo ""
echo "4. Модели на макбуке:"
ollama list 2>/dev/null | awk 'NR>1 {print "   • " $1 " (" $3 ")"}' || echo "   (ollama не запущен)"
echo ""
echo "5. Быстрые команды:"
echo "   ~/.ai-server/status.sh        — статус сервера"
echo "   ~/.ai-server/restart-ollama.sh — рестарт Ollama"
echo "   ~/.ai-server/logs.sh          — логи"
SCRIPT
chmod +x "$SCRIPTS_DIR/connection-info.sh"

# Добавляем в PATH
if ! grep -q 'AI_SERVER' "$HOME/.zshrc" 2>/dev/null; then
  cat >> "$HOME/.zshrc" << 'EOF'

# MacBook AI Server
export AI_SERVER_HOME="$HOME/.ai-server"
alias ai-status="bash $AI_SERVER_HOME/status.sh"
alias ai-restart="bash $AI_SERVER_HOME/restart-ollama.sh"
alias ai-info="bash $AI_SERVER_HOME/connection-info.sh"
EOF
  ok "Алиасы добавлены в ~/.zshrc (ai-status, ai-restart, ai-info)"
fi

ok "Скрипты управления установлены в ~/.ai-server/"
ok "  ~/.ai-server/status.sh          — статус"
ok "  ~/.ai-server/restart-ollama.sh  — рестарт Ollama"
ok "  ~/.ai-server/logs.sh            — логи"

# ────────────────────────────────────────────────────────────────────────────
# ШАГ 8: Финальная проверка
# ────────────────────────────────────────────────────────────────────────────
header "ШАГ 8/8: Финальная проверка"

echo ""
echo -e "${BLUE}Проверка 1: Ollama запущен?${NC}"
if curl -s http://localhost:11434/api/tags >/dev/null 2>&1; then
  ok "Ollama сервер работает на 0.0.0.0:11434"
  echo ""
  echo -e "${BLUE}Установленные модели:${NC}"
  ollama list 2>/dev/null | awk 'NR>1 {print "  " $1 "  " $3}'
else
  warn "Ollama не отвечает. Запусти вручную: ollama serve"
fi

echo ""
echo -e "${BLUE}Проверка 2: Tailscale IP${NC}"
TS_IP=$(tailscale ip -4 2>/dev/null || true)
if [ -n "$TS_IP" ]; then
  ok "Tailscale IP: $TS_IP"
  echo "  Доступные порты: 11434 (Ollama)"
else
  warn "Tailscale не в сети. Подключись: tailscale up"
fi

echo ""
echo -e "${BLUE}Проверка 3: Питание и сон${NC}"
SLEEP_SET=$(pmset -g 2>/dev/null | grep " sleep " | head -1)
echo "  Настройки сна: $SLEEP_SET"
POWER_MODE=$(pmset -g 2>/dev/null | grep "powermode" | head -1)
if [ -n "$POWER_MODE" ]; then
  echo "  Power Mode: $POWER_MODE"
fi

echo ""
echo -e "${BLUE}Проверка 4: Диск${NC}"
echo "  $(df -h / | awk 'NR==2 {print "Занято: " $3 " / " $2 " (" $5 ")"}')"
AVAIL_GB=$(df -h / | awk 'NR==2 {print $4}' | sed 's/G//' | sed 's/,/./')
if (( $(echo "$AVAIL_GB < 100" | bc -l 2>/dev/null || echo 0) )); then
  warn "Маловато места ($AVAIL_GB GB свободно). Модели займут ~75GB."
fi

# ────────────────────────────────────────────────────────────────────────────
# ИТОГО
# ────────────────────────────────────────────────────────────────────────────
header "ГОТОВО!"

echo ""
echo -e "${GREEN}MacBook Pro M4 Max 128GB настроен как AI-сервер!${NC}"
echo ""
echo "  Подключение через Tailscale:"
echo "    http://$TS_IP:11434"
echo ""
echo "  Модели:"
echo "    qwen3-coder-next              — кодинг (основная)"
echo "    qwen3.5:35b-a3b-coding-nvfp4  — быстрая"
echo "    qwen3.5:7b                    — лёгкая"
echo ""
echo "  Управление:"
echo "    ai-status   — статус"
echo "    ai-restart  — рестарт Ollama"
echo "    ai-info     — информация для подключения"
echo ""
echo "  На сервере simplex-node opencode подключится автоматически"
echo "  после указания Tailscale IP макбука."
echo ""
echo -e "${YELLOW}  !!! ВАЖНО: MacBook должен быть всегда подключён к питанию !!!${NC}"
echo -e "${YELLOW}  !!! и в той же Tailnet, что и сервер simplex-node.       !!!${NC}"
echo ""
