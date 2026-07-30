# simplex-node — Installation Pack for Ubuntu Linux (Beelink SER9)

Автоматический установщик серверной части simplex-node: транспортный хаб,
релейный узел SimpleX, экономическая платформа, AI стюард, радио и P2P облако.

## Состав пакета

```
install-pack/
├── install.sh                 # Главный скрипт установки (запускать от пользователя)
├── bin/simplex-node           # Go бинарник (статический, ~21MB)
├── docker/                    # Docker compose стек (Tor, SMP, XFTP, Coturn)
│   ├── docker-compose.yml
│   ├── tor/
│   ├── coturn/
│   ├── smp_configs/
│   └── xftp_configs/
├── scripts/                   # Вспомогательные скрипты
│   ├── launch-node.sh
│   ├── backup-to-usb.sh
│   └── restore-from-usb.sh
├── node-monitor.py            # Демон авто-восстановления
└── README.md                  # Этот файл
```

## Требования

- **Железо**: Beelink SER9 (Intel N150) или любой x86_64 с 8GB+ RAM, 128GB+ SSD
- **OS**: Ubuntu 24.04+ (Linux)
- **Интернет**: 100Mbps+ (рекомендуется 500Mbps как на Beelink SER9)
- **Пользователь**: обычный (не root), в группе `docker`

## Быстрая установка

```bash
# Распаковать архив
tar xzf simplex-node-install-pack.tar.gz
cd simplex-node-install-pack

# Запустить установку
bash install.sh
```

Скрипт сделает всё автоматически:
1. Установит зависимости (docker, python3, curl, jq и др.)
2. Создаст директории
3. Установит бинарник simplex-node
4. Установит xray (V2Ray-совместимый прокси)
5. Развернёт Docker стек (Tor, SMP-сервер, XFTP, Coturn)
6. Установит systemd user-сервисы
7. Скопирует скрипты и конфиги
8. Запустит все сервисы

## После установки

### Проверить статус

```bash
# Статус сервера
systemctl --user status simplex-node

# API здоровье
curl -s http://127.0.0.1:8080/api/health

# Бридж (соединение с SimpleX)
curl -s http://127.0.0.1:8080/api/chat/bridge-health

# Docker контейнеры
cd ~/simplex-node-docker && docker compose ps
```

### Зарегистрировать приложение

```bash
curl -s -X POST -H "Content-Type: application/json" \
  -d '{"name":"my-app"}' \
  http://127.0.0.1:8080/api/transport/v1/register
```

### Логи

```bash
journalctl --user -u simplex-node -f
journalctl --user -u simplex-node-monitor -f
```

## API Эндпоинты

| Endpoint | Описание |
|----------|----------|
| `GET /api/health` | Здоровье ноды |
| `GET /api/version` | Версия и аптайм |
| `POST /api/transport/v1/register` | Регистрация приложения |
| `GET /api/transport/v1/ws` | WebSocket real-time |
| `POST /api/transport/v1/send` | Отправка сообщения |
| `GET /api/transport/v1/stats` | Статистика транспорта |
| `GET /api/transport/v1/config` | Конфигурация для удалённых приложений |
| `GET /api/chat/history` | История чата |
| `GET /api/chat/stream` | SSE поток сообщений |
| `POST /api/chat/send` | Отправить сообщение через чат |
| `GET /api/admin/info` | Полная информация о системе |
| `GET /api/inquisitor/report` | Отчёт инквизитора |

Всего **98+ эндпоинтов**.

## Структура директорий после установки

```
~/.local/share/simplex-node/
├── simplex-node.json           # Конфиг
├── chat_history.json           # История чата
├── chat.db                     # SQLite чата
├── transport_apps.json         # Зарегистрированные приложения
├── logs/
│   ├── dashboard.log
│   └── xray.log
├── config/
└── backups/

~/simplex-node-docker/
├── docker-compose.yml
├── tor/
├── coturn/
├── smp_configs/
├── smp_state/
├── xftp_configs/
└── xftp_state/
```

## Порты

| Порт | Сервис |
|------|--------|
| 8080 | HTTP API (simplex-node) |
| 5223 | SMP сервер |
| 5225 | XFTP сервер |
| 17001 | P2P Transfer |
| 17225 | WebSocket Bridge (CLI) |
| 9050 | Tor SOCKS5 |
| 10810 | XRay SOCKS5 |

## Безопасность

- Все системные сервисы используют `PrivateTmp`, `ProtectSystem=strict`, `NoNewPrivileges=yes`
- API доступен на всех интерфейсах (`0.0.0.0:8080`) — настройте firewall при необходимости
- SimpleX трафик идёт через Tor (Onion-адреса)
- WebSocket транспорт использует API-ключи для аутентификации

## Лицензия

Saint Mary Liberty Island — Sovereign Network
