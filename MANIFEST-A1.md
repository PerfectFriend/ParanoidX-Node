# МАНИФЕСТ ПРОЕКТА simplex-node (A1)

> **Дата:** 2026-06-06
> **Статус:** Активная разработка, A1 (пре-альфа)
> **Версия ядра:** Go 1.24+ / Docker Compose / Tor Hidden Services / SimpleX Chat

---

## 1. ИДЕЯ

### 1.1 Что это такое?

**simplex-node** — это суверенная цифровая инфраструктура микрогосударства Saint Mary Liberty Island (stmaria.org). Представляет собой экономический узел («Королевская нода»), объединяющий:

- **Приватные E2EE-коммуникации** граждан через протокол SimpleX (аналог Signal/Tor, но без идентификаторов)
- **Экономику серебряной воронки** (Silver Black Hole Funnel) — математическая модель привлечения мирового серебра через токенизацию реальных активов
- **Облачное хранилище** (Vault) для доказательств RWA, медиа, аудио
- **Токенизацию всего** — от серебряных монет и банкнот Mark Bank (акции) до NFC-паспортов Острова
- **Peer-to-Peer рынок** токенизированными активами с аукционами и байбеком
- **Анонимные медиа-каналы** с монетизацией трафика в нанограммах серебра (ng)

### 1.2 Ключевая метафора

**«Серебряная воронка / Чёрная дыра»** — самоусиливающийся цикл:

```
USDT TRC20 (внешние инвесторы) → Брокер → Физическое серебро в резерв Острова
  → Токенизация (ng silver) → 20% казначейство / 80% дивиденды держателям банкнот-акций
  → Банкноты дают yield → спрос на банкноты → ещё больше USDT притекает
  → больше серебра закупается → больше будущих дивидендов → **воронка засасывает серебро**
```

### 1.3 Почему SimpleX?

- **Нулевая мета-информация** — нет ID, номеров, username'ов; только E2EE-контакты
- **Double-ratchet** поверх SMP (SimpleX Messaging Protocol)
- **Onion-маршрутизация** через собственный Tor Hidden Service
- **Один контакт** в stock SimpleX Chat → все сервисы Острова

---

## 2. МЕТОД

### 2.1 Транспорт

```
[Пользователь SimpleX Chat]
        │ E2EE (Double Ratchet)
        ▼
[Собственный SMP-сервер] ← Tor HS (onion:5223)
        │
[SimpleX CLI] ← WS API (localhost:5230)
        │
[Go Bridge (simplex-node)] ← HTTP API (0.0.0.0:8080)
        │
[Файловое JSON-хранилище] ← dataDir
        │
[Telegram Bot (админ)]
```

### 2.2 Инфраструктура Docker

```
4 контейнера:
  • smp-server — официальный SimpleX Messaging Protocol сервер (порт 5223)
  • xftp-server — официальный SimpleX File Transfer Protocol сервер (порт 443)
  • tor — кастомный Tor с non-anonymous hidden services (однохоповые)
  • coturn — TURN/STUN для WebRTC голос/видео звонков (только TCP через Tor)
```

### 2.3 Tor Hidden Services (5 штук)

| Сервис | Адрес .onion | Назначение |
|--------|-------------|------------|
| SMP | `7czed3rx...ryz4zxlo7wiwgz36yfmdwvu6ylv5wkby3trei3qsuw4lnqd.onion:5223` | Пересылка сообщений SimpleX |
| XFTP | `fv3pfzxi...h5sjf33jmusfbskmd2i3lywaaaysh6tijc7df7k6sijq3yyd.onion:443` | Файлы и медиа |
| Dashboard | `q273p7co...au3uvzeddexvdgv6andorfzvplstztheso2qcsj4yqvfzzad.onion:80` | Веб-интерфейс |
| ICE/TURN | `rigx5uuq...k5bgvcikjfbtqenw5qn3fra34nkynrrrfp2sijophhqu4pqd.onion:3478` | WebRTC звонки |
| Auditor | `aytiwnc5...xwrrqnvxduychtzezfq77omtxbje5elmwmwsaa6zmfidtkid.onion:80` | Панель аудиторов |

### 2.4 Экономическая модель (двухуровневая)

1. **Liquid Taler (нг/ng, 1 TLR = 31 103 480 000 ng = 1 тройская унция серебра)**
   - Ed25519-аккаунты с балансами
   - Переводы, минт, история (через Ledger)
   - Привязка к спотовой цене серебра ($75/oz)

2. **Банкноты Mark Bank (BanknoteV2 — NFT с редкостью)**
   - Служат акциями (equity shares) — держатели получают дивиденды из каждого раунда токенизации
   - Редкость: Common / Rare / Epic / Legendary / Golden / Genesis
   - Бустер-паки (5 банкнот, гарантирован 1 Rare+)
   - P2P-обмен, аукционы (27h proxy-bidding), выкуп казной (buyback)
   - Аудиторы (топ-10 держателей) с привилегиями

---

## 3. СТРУКТУРА КОДА

### 3.1 Go-бинар (6 289 строк, 9 файлов, 72 API эндпоинта, 127 функций)

```
simplex-node/
├── cmd/
│   ├── simplex-node/main.go       (3675 строк) — HTTP-сервер + SimpleX WS-мост
│   └── banknote-press/main.go     (246 строк)  — Микросервис рендеринга банкнот
├── internal/
│   ├── economy/
│   │   ├── ledger.go             (208 строк) — Liquid Taler Ledger (Ed25519)
│   │   ├── auction.go            (329 строк) — Аукцион (27ч proxy-bidding)
│   │   ├── buyback.go            (170 строк) — Выкуп казной
│   │   ├── p2p.go                (238 строк) — P2P-предложения
│   │   ├── pack.go               (234 строк) — Бустер-паки
│   │   └── registry.go           (627 строк) — Реестры, пре-минт, аудиторы
│   └── press/
│       └── templates.go          (201 строк) — Шаблоны банкнот
└── scripts/
    ├── demo-auditor.go           (17 строк)
    └── genesis/genesis-init.go   (344 строк) — Инициализация экономики
```

### 3.2 Shell-скрипты (2 654 строк, 18 файлов)

```
scripts/
├── royal-common.sh               (64 строки)  — Централизованные пути
├── launch-node.sh                (92 строки)  — Канонический лаунчер
├── island-bot-setup.sh           (335 строк)  — Настройка SimpleX-бота
├── launch-bot-listener.sh        (48 строк)   — Запуск Telegram-слушателя
├── royal-telegram-command-listener.sh (338 строк) — Ядро Telegram-бота (~30 команд)
├── send-to-torquemada.sh         (46 строк)   — Отправка сообщений Telegram
├── signal_step_done.sh           (18 строк)   — Сигнал завершения шага
├── version-checkpoint.sh         (76 строк)   — Контроль версий
├── regenerate-qr.sh              (54 строки)  — Регенерация QR-кодов
├── test-royal.sh                 (186 строк)  — Тестовый харнес
├── island-bot-init.py            (117 строк)  — Pexpect-инициализация бота
├── opencode-tg-listener.py       (336 строк)  — Мост Telegram ↔ opencode
├── launch-opencode-tg.sh         (25 строк)   — Запуск opencode-моста
└── build.sh                      (8 строк)    — Сборка Go-бинарника

docker/
├── startup.sh                    (742 строки) — Оркестратор Docker-стека
├── restore-tor-keys.sh           (61 строка)  — Восстановление onion-ключей
└── tor/entrypoint.sh             (48 строк)   — Точка входа Tor-контейнера
```

### 3.3 Дашборд (HTML/JS)

```
docker/dashboard.html             (2274 строки) — SPA с 22 карточками
  • ~40 вызовов API
  • Весь UI на vanilla JS (без фреймворков)
  • Единственный entry: / → отдаётся из dataDir

docker/auditor-dashboard.html     (314 строк) — SPA для аудиторов
  • 5 карточек (логин, экономика, аудиторы, холдеры, P2P)
  • Требует auditor_token для доступа
```

### 3.4 Данные (~2.1 GB)

```
~/.local/share/simplex-node/
├── vault/                         (2 GB квота) — файлы пользователей
│   ├── .reserved                  (2 GB)
│   ├── announcement-round-*.txt   (7 шт)
│   ├── dividend-*.txt             (22 шт)
│   └── island-bot-audit.log       (64 KB)
├── *.json                         (12 файлов) — состояния (registry, ledger, rwa...)
├── *.txt                          (20+ файлов) — адреса, логи, конфиги
├── *.png                          (9 шт) — QR-коды
├── *.db                           (2 шт, ~2.3 MB) — SQLite бота
├── logs/                          (9 лог-файлов)
├── tor-keys-backup/               (5 директорий) — бекап onion-ключей
└── island-bot/                    — Runtime SimpleX-бота
```

### 3.5 API Endpoints (полный список)

```
Группа                    | Количество
--------------------------|-----------
Status/Info               | 7
Vault                     | 5
Treasury/Economy          | 10
Market/Auction/Buyback   | 7
P2P                       | 6
Pack/Genesis              | 4
Auditor                   | 4
RWA/Billing               | 4
Radio/Channels            | 8
Royal/Control             | 2
Lock/Security             | 4
Role/Send                 | 2
SimpleX Bridge (WS)       | — (goroutine)
--------------------------|-----------
ИТОГО:                    | 66
```

---

## 4. РЕШЕНИЯ (архитектурные и технические)

### 4.1 Файловое хранение вместо БД

**Решение:** Все состояния — JSON-файлы в `dataDir`.  
**Почему:** Простота, прозрачность, возможность ручного редактирования, отсутствие зависимости от внешних СУБД.  
**Цена:** Нет транзакционности, риск потери данных при crash между write и sync.  
**Компенсация:** `sync` после каждого write (через `json.MarshalIndent` + `os.WriteFile`).

### 4.2 Однохоповые Tor Hidden Services

**Решение:** `HiddenServiceSingleHopMode 1` + `HiddenServiceNonAnonymousMode 1`.  
**Почему:** Нам не нужна анонимность — нужна приватность и стабильные .onion адреса. Однохоповые HS в 3× быстрее.  
**Цена:** Нельзя скрыть IP ноды. Для транспорта SimpleX это приемлемо (сам SimpleX не использует IP в модели угроз).

### 4.3 Non-anonymous Tor

**Решение:** Тор не скрывает IP сервера.  
**Почему:** Наши угрозы — это цензура и блокировки на уровне ISP/DNS, не юридическое преследование оператора ноды. .onion адреса дают устойчивость к DNS-блокировкам.

### 4.4 Мост к SimpleX через CLI + WebSocket

**Решение:** Не используем smp-library напрямую, а подключаемся через WebSocket API официального `simplex-chat` CLI.  
**Почему:** API клиента проще и стабильнее, чем реализация SMP-протокола с нуля.  
**Цена:** Зависимость от бинарного CLI, race conditions при перезапуске, ограниченный WS API.

### 4.5 Plaintext PIN в lock.json

**Решение:** Код блокировки хранится в открытом виде.  
**Почему:** Защита от случайного доступа к дашборду (физическая безопасность), не от атак.  
**Риск:** Не защищает от чтения файла.

### 4.6 Два Telegram-бота

**Решение:** Отдельные боты для администрирования ноды (royal-bot) и для общения с opencode LLM.  
**Почему:** Разделение ответственности. Админ-бот — 30+ команд управления нодой. opencode-бот — интерфейс к AI.  
**Цена:** Два набора токенов, две polling-петли.

### 4.7 Бекап ключей Tor вне Git

**Решение:** `restore-tor-keys.sh` хранит копии в `~/.local/share/simplex-node/tor-keys-backup/`.  
**Почему:** `.gitignore` исключает `hidden_services/*`, git clean уничтожит ключи. Бекап вне репозитория гарантирует сохранность.  
**Дополнительно:** Post-extraction backup в `startup.sh` + hourly cron.

### 4.8 Одноразовый флаг `.profile_image_set`

**Решение:** Триггер отправки аватара при первом подключении моста.  
**Почему:** CLI не отправляет profile image при каждом `/p`, только при изменении. Временное изменение имени форсирует broadcast. Флаг предотвращает повторение.

---

## 5. ГОТОВЫЕ ЧАСТИ (что работает прямо сейчас)

### 5.1 Инфраструктура ✅

- [x] Docker-стек: smp-server, xftp-server, tor, coturn — 4 контейнера работают
- [x] 5 Tor Hidden Services со стабильными .onion адресами
- [x] Бекап и восстановление onion-ключей
- [x] Self-signed TLS сертификаты для SMP и ICE
- [x] QR-коды для всех адресов
- [x] `client-connection-info.txt` с инструкциями на русском

### 5.2 SimpleX-бот (Island Royal Services) ✅

- [x] CLI WS gateway на порту 5230
- [x] Go-мост с gorilla/websocket
- [x] Авто-принятие входящих запросов контакта (`/set accept_requests on`)
- [x] Обработка контактов: `contactConnected` + `receivedContactRequest`
- [x] Авто-регистрация первого контакта как «king»
- [x] Приветственное сообщение со списком команд
- [x] Диспетчеризация команд: citizen (`/wallet`, `/radio`, `/vault`, `/market`, `/tokenize`, `/id`, `/help`) + king (status, plan, build, disk, launch, kill, gobuild, backup)
- [x] Reconnect при разрыве WS (до 20 попыток)
- [x] Фильтрация echo (corrId)
- [x] Аватар (герб stmaria.org 160×160 JPEG)
- [x] Логирование всех типов событий

### 5.3 Веб-дашборд ✅

- [x] Статус узла (SMP, XFTP, storage, disk, uptime)
- [x] Адреса серверов (SMP, XFTP, ICE, Auditor) с COPY/QR
- [x] Hero QR — контакт Острова для добавления в SimpleX
- [x] Сервисы Острова — карточка с контактом
- [x] Treasury — состояние резерва, раунды, дивиденды
- [x] Radio / Library — список анонсов и аудио
- [x] Глобальный Vault — загрузка, скачивание, удаление, заметки
- [x] Pack Shop — покупка/открытие бустеров
- [x] Buyback — выкуп казной с ценой по редкости
- [x] Auction House — 27h аукционы с proxy-bidding
- [x] Market — P2P-торговля RWA
- [x] Anonymous Channels — создание, доступ по ng, просмотр, посты
- [x] ICE/TURN — WebRTC звонки
- [x] Dashboard Onion — копия onion-адреса дашборда
- [x] Lock System — PIN-блокировка
- [x] Rotate — кнопка пересоздания SMP/XFTP ключей
- [x] Auditor Dashboard (отдельная SPA)

### 5.4 Экономика (A2 Ledger) ✅

- [x] Ed25519-аккаунты с Liquid Taler балансами
- [x] Реестр банкнот BanknoteV2 (серийные номера, редкость, холдеры)
- [x] Пре-минт с Genesis-зарезервированными банкнотами
- [x] Аудиторы (топ-10 система, refresh)
- [x] P2P-предложения (офферы между холдерами)
- [x] Аукцион (27 часов, proxy-bidding, авто-продление, 5% комиссия)
- [x] Buyback (Common -2.28%, Rare/Epic/Legendary — par, Golden/Genesis — аукцион)
- [x] Pack Manager (бустеры 5 карт, гарантия Rare+)
- [x] Treasury Split (калькулятор 75/25, 50/25/25, 20/30/50, 10/20/30/40)
- [x] Долг первого инвестора (debt tracking, repayment)

### 5.5 Telegram-боты ✅

- [x] Royal admin bot (~30 команд: управление нодой, экономика, план, версии)
- [x] Opencode AI bridge (каждый чат → отдельная opencode сессия)
- [x] Логирование всех сообщений в `bot_full_prompt.log`

### 5.6 Прочее ✅

- [x] `test-royal.sh` — тестовый харнес с изолированным окружением
- [x] `version-checkpoint.sh` — контроль версий с бекапом
- [x] `regenerate-qr.sh` — кроновый регенератор QR (каждые 5 мин)
- [x] `island-bot-setup.sh` — полная настройка бота (pexpect)
- [x] Disk preflight checks при запуске
- [x] Система ротации ключей с бекапом старых

---

## 6. ПЛАН ЗАДАЧ (TO-DO)

### 6.1 Критические баги (немедленно)

| # | Задача | Где | Описание |
|---|--------|-----|----------|
| **B1** | **Data race: `knownRoles` без мьютекса** | `main.go:2267`, `main.go:392` | HTTP handler (`/api/set_role_chat`) и WS goroutine одновременно пишут/читают map без блокировки |
| **B2** | **Data race: `islandWS` разблокировка до использования** | `main.go:385-388` | `sendToIslandRole` отпускает `islandWSMu` до вызова `WriteJSON`, другой goroutine может закрыть/заменить `conn` |
| **B3** | **Connection leak: `r.Body` не закрывается** | `main.go:888-909` и 10+ других мест | После `Decode(&req)` тело запроса никогда не закрывается — утечка файловых дескрипторов при HTTP-нагрузке |
| **B4** | **nil-dereference в `f.WriteString`** | `main.go:811-816` + 6 мест | `os.OpenFile` возвращает `nil, error`, ошибка игнорируется, затем `f.WriteString` паникует |
| **B5** | **Эхо-луп: фильтрация `corrId`** | `main.go:3373-3379` | (исправлено) — но надо убедиться, что `accept_requests on` работает |
| **B6** | **Stale addresses.json с неверным XFTP** | `docker/addresses.json` | Содержит старый XFTP onion (`62o2qarj...` вместо `fv3pfzx...`) |

### 6.2 Баги средней важности (ближайшие дни)

| # | Задача | Где | Описание |
|---|--------|-----|----------|
| **M1** | `restore-tor-keys.sh` — `set -e` с `cp` без `|| true` | `line 44-45` | Если в бекапе нет public key или hostname, `cp` упадёт, скрипт выйдет |
| **M2** | `chmod 600 "$BIND_DIR"/*` на пустой директории | `restore-tor-keys.sh:47` | Bash передаст `*` как аргумент, `chmod` упадёт, `set -e` убьёт скрипт |
| **M3** | **Plaintext PIN в lock.json** | `lock.json` | Хотя бы SHA-256 с солью |
| **M4** | **Нет rate-limiting на `/api/unlock`** | `main.go:880-898` | Брутфорс 6-значного PIN без блокировки |
| **M5** | **Горутина ротации не контролируется** | `main.go:955-958` | Каждый вызов `/api/rotate` создаёт новую горутину, которые накапливаются |
| **M6** | **Баг: `island-bot-init.py` и `start_cli_ws` запускают CLI дважды на одном порту** | `island-bot-setup.sh` | `-p 5230` передаётся и pexpect, и постоянному процессу — "Address already in use" |
| **M7** | **Shell injection в Python heredoc** | `royal-telegram-command-listener.sh:274` | `chat = "'"$CHAT_ID"'"` — если CHAT_ID содержит кавычки, ломается Python |

### 6.3 Архитектурные улучшения

| # | Задача | Где | Описание |
|---|--------|-----|----------|
| **A1** | **Вынести хардкодные пути в конфиг / env** | Весь код | 16+ хардкодных `/home/tomas/simplex-node/...` путей в main.go |
| **A2** | **Добавить graceful shutdown** | `main.go` | Нет обработки SIGTERM/SIGINT — горутины и соединения рвутся принудительно |
| **A3** | **Добавить healthcheck'и Docker** | `docker-compose.yml` | Ни один контейнер не имеет `healthcheck:`, несмотря на 90-секундные ожидания в startup.sh |
| **A4** | **Переписать lock систему на bcrypt/scrypt** | `main.go` | Хранение bcrypt-хэша вместо plaintext |
| **A5** | **Добавить мутекс на `knownRoles`** | `main.go` | Закончить то, что начато (уже есть `welcomeMu`, нужен `rolesMu`) |
| **A6** | **Сделать ICE TLS сертификат чувствительным к смене onion** | `startup.sh` | Сейчас генерируется один раз и никогда не пересоздаётся при смене адреса |
| **A7** | **Добавить мониторинг/алертинг на диск** | `main.go` | Критично после инцидента 100% заполнения диска |

### 6.4 Функциональные доработки

| # | Задача | Описание |
|---|--------|----------|
| **F1** | **Реальные WebRTC звонки через ICE** | Сейчас signaling работает, но медиа не тестировано |
| **F2** | **SimpleX Channels — подписка граждан** | Механизм монетизации каналов есть, но нет авто-подписки |
| **F3** | **Генерация банкнот (banknote-press)** | Микросервис есть, но не интегрирован в дашборд |
| **F4** | **TRON USDT реальный мониторинг** | `tron_treasury.txt` существует, но нет live-сканирования блокчейна |
| **F5** | **Динамический ICE credentials (~12h)** | HMAC-подпись времени + username для TURN |
| **F6** | **Sub-node синхронизация (SMP-команды от royal)** | Есть заглушка `/royal/sync` |
| **F7** | **Мобильное приложение / fork SimpleX** | Rich UI с кастомизированными командами |
| **F8** | **Физический NFC-паспорт Острова** | Интеграция с чипом, регистрация, верификация |

### 6.5 Тестирование

| # | Задача | Описание |
|---|--------|----------|
| **T1** | Регрессия: запустить `test-royal.sh` | Проверить, что всё API отвечает |
| **T2** | Тест подключения нового контакта SimpleX | Сквозной E2E тест: добавить контакт → получить welcome → отправить команду → получить ответ |
| **T3** | Тест reconnect при падении WS | Убить CLI, проверить переподключение моста |
| **T4** | Тест восстановления Tor ключей | Удалить hidden_services/smp, запустить restore-tor-keys.sh, проверить hostname |
| **T5** | Go race detector | `go run -race` на тестовой ноде |

---

## 7. СТАТИСТИКА ПРОЕКТА

```
Метрика                  | Значение
-------------------------|---------
Строк Go-кода           | 6 289
Всего Go-файлов         | 9
Всего функций           | ~127
API эндпоинтов          | 66
Shell-скриптов          | 14 (2 654 строк)
Docker-контейнеров      | 4
Tor Hidden Services     | 5
Файлов данных           | ~57 в dataDir
Лог-файлов              | 9
QR-кодов                | 9
Telegram-ботов          | 2 (royal + opencode)
Размер vault            | 2 GB квота / ~70 KB занято
Банкнот в реестре       | 2 (demo-citizen1, demo-citizen2)
Резерв серебра          | 37 103 480 000 ng (~37 g)
Дашборд                 | 2 274 строк (vanilla JS SPA)
```

---

## 8. КЛЮЧЕВЫЕ ФАЙЛЫ (шпаргалка)

```
Путь                                          | Назначение
----------------------------------------------|----------------------------------------
~/simplex-node/cmd/simplex-node/main.go       | Go-сервер + SimpleX-мост
~/simplex-node/docker/startup.sh              | Оркестратор Docker-стека
~/simplex-node/docker/docker-compose.yml      | Определения сервисов
~/simplex-node/docker/dashboard.html          | SPA-дашборд
~/simplex-node/docker/restore-tor-keys.sh     | Восстановление onion-ключей
~/simplex-node/scripts/launch-node.sh         | Запуск ноды (канонический)
~/simplex-node/scripts/island-bot-setup.sh    | Настройка SimpleX-бота
~/simplex-node/scripts/royal-common.sh        | Конфигурация путей
~/.local/share/simplex-node/                  | Рабочие данные (DB, логи, vault)
~/.local/share/simplex-node/addresses.json    | Текущие адреса
~/.local/share/simplex-node/island_contact_link.txt | Ссылка на контакт SimpleX
~/.local/share/simplex-node/tor-keys-backup/  | Бекап onion-ключей
~/.local/share/simplex-node/vault/            | Файлы хранилища
```

---

*Манифест создан 2026-06-06 на основе полного аудита кодовой базы. Все 72 API эндпоинта, 22 карточки дашборда, 4 Docker-контейнера и ~2 500 строк скриптов документированы выше.*
