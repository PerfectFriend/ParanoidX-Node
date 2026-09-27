# ParanoidX — полный аудит проекта

Дата: 2026-09-27 · Commit: `244b666` (2026-08-16) + 20 незакоммиченных изменений
Машина аудита: Ubuntu, Go 1.27.1, Docker 29.8.1, Python 3.14.4, Xray 26.3.27, Flutter 3.44.1

---

## 1. Что это

**ParanoidX** — серебряная цифровая экономика поверх SimpleX/Tor. Не «нода», а узел из четырёх
независимых слоёв, которые можно гасить по отдельности:

| Слой | Что делает |
|---|---|
| **Экономика** | Чеканка TLR/NT, обеспечение физическим серебром, казна, аукцион, комитет |
| **Транспорт** | 4-слойный прокси-мост SimpleX ↔ WebSocket, TURN, onion |
| **Приложения** | The Isle (остров) и Royal Isle (королевский) — Flutter-клиенты |
| **Наблюдение** | GTK-монитор с треем, Telegram-алерты, ротация подписок |

Философия совпадает с девизом ноды: **работает только то, что проверено, а не то, что
должно работать**. Каждый слой имеет кнопки ON/OFF/TEST, и TEST проверяет фактическое
поведение, а не наличие процесса.

---

## 2. Состав кода

| Метрика | Значение |
|---|---|
| Go-файлов | 283 |
| Строк Go | 67 772 |
| Пакетов | 44 (в `go list ./...` — с подпакетами) |
| Прямых зависимостей Go | 12 |
| Dart-файлов (приложения) | 61 (18 562 строки) |
| Python (монитор/control/подписки) | ~2 100 строк |
| Тестов | 36 пакетов зелёные, 0 падений |

### 2.1 Ключевые пакеты

| Пакет | Строк | Назначение |
|---|---|---|
| `internal/api` | 23 694 | HTTP-обработчики и все эндпоинты. Крупнейший пакет |
| `internal/economy` | 10 317 | Чеканка, резерв, токеномика, аукцион |
| `internal/radio` | 2 256 | Радио-стриминг |
| `internal/container` | 826 | Управление контейнерами (2-тысячные строки с 23 535 в тестах) |
| `internal/registry` | 1 035 | Обнаружение нод, региональная маршрутизация |
| `internal/transport` | 1 790 | WebSocket-транспорт |
| `internal/dc` | 1 952 | P2P-распространение контейнеров |
| `internal/paranoidx` | 2 097 | Многослойный прокси |
| `internal/gateway` | 1 064 | Интеграция с мессенджерами |
| `internal/bot` | 2 129 | Telegram-боты (DarkPushkin) |
| `internal/treasury` | 798 | Управление казной |
| `internal/store` | 2 000 | Хранилище (SQLite) |
| `internal/crypto/*` | — | BIP39/BTC/ETH — подписи, деривация ключей |
| `internal/webrtc` | 372 | WebRTC-сигналинг и TURN |
| `internal/press` | 499 | «Банкнотная печать» острова |
| `internal/isle` / `internal/royal` | 323 / 1 170 | Сети острова и королевства |
| `internal/ai` / `internal/steward` | 1 137 / 676 | Ollama-клиент и ИИ-управляющий |

### 2.2 Зависимости Go

Все — прямые, без CGO, без системных библиотек:

| Зависимость | Версия | Зачем |
|---|---|---|
| `modernc.org/sqlite` | 1.52.0 | SQLite **на чистом Go** (без cgo — критично для portable-сборки) |
| `github.com/gorilla/websocket` | 1.5.3 | WebSocket-мост |
| `golang.org/x/crypto` | 0.52.0 | Криптография |
| `github.com/google/uuid` | 1.6.0 | Идентификаторы |
| `rsc.io/qr` | 0.2.0 | QR-коды для кошельков |
| `github.com/dustin/go-humanize` | 1.0.1 | Форматирование чисел для UI |
| `github.com/ncruces/go-strftime` | 1.0.0 | SQLite-регистрация |
| `modernc.org/{libc,mathutil,memory}` | 1.72/1.7/1.11 | Транзитивные для sqlite |
| `github.com/remyoudompheng/bigfft` | 2023… | Транзитивная |
| `github.com/mattn/go-isatty` | 0.0.20 | Определение TTY |
| `golang.org/x/sys` | 0.45.0 | Системные вызовы |

Всего в `go list -m all` — **32 модуля**. Внешних сервисов Go-код не требует: Ollama
подключается по HTTP и деградирует до отключённого ИИ при недоступности.

---

## 3. Зависимости времени выполнения

### 3.1 Docker-стек (4 контейнера)

| Контейнер | Образ | Порт (host→container) | Роль |
|---|---|---|---|
| `ParanoidX-tor` | сборка из репозитория | через SOCKS | Tor для скрытых сервисов |
| `ParanoidX-coturn` | `coturn/coturn:latest` | STUN/TURN | TURN для WebRTC |
| `ParanoidX-smp-server` | `simplexchat/smp-server:latest` | `127.0.0.1:5223` | SimpleX messaging server |
| `ParanoidX-xftp-server` | `simplexchat/xftp-server:latest` | `127.0.0.1:5225:443` | Файловая передача |

Все порты слушают **только localhost** — наружу не выставлены. Compose: `docker/docker-compose.yml`.

### 3.2 Порты ноды

| Порт | Процесс | Назначение |
|---|---|---|
| 8080 | ParanoidX | HTTP API + веб-дашборд (localhost + docker-bridge `172.17.0.1`) |
| 17001 | ParanoidX | Основной сервис |
| 10810 | xray | SOCKS5-мост (единственный рабочий путь в интернет) |
| 5223–5226 | docker | SimpleX SMP/XFTP |
| 9050 | tor | SOCKS Tor |
| 11434 | ollama | Локальный ИИ (внешний по LAN, не часть ноды) |

### 3.3 systemd

**Системные (нужен root):**

| Юнит | Роль |
|---|---|
| `ParanoidX-dashboard.service` | Основной демон, `Restart=always`, `RestartSec=5s`, `TimeoutStopUSec=90s` |
| `ParanoidX.target` | Якорь для whole-stack |
| `ParanoidX-post-reboot-restore.service` | Восстановление после ребута |
| `ParanoidX-bridge.service`, `ParanoidX-admin-bot.service`, `ParanoidX-royal-bot.service`, `ParanoidX-poet-bot.service` | Отключены (устаревшие пути) |

**Пользовательские:**

| Юнит | Роль |
|---|---|
| `node-monitor.service` | GTK-монитор + трей, `Restart=on-failure`, `RestartSec=30` |
| `px-xray-refresh.timer` | Обновление подписок каждые 12 ч, `Persistent=true` |
| `px-xray-refresh.service` | oneshot: протокол подписки. **`KillMode=process`** |
| `simplex-backup.timer` | Резервное копирование на USB |

`Linger=yes` у пользователя — user-юниты работают без сессии.

### 3.4 Управление без пароля root

Тонкий путь: `polkit` разрешает **только** программу `/usr/local/bin/px-node-control`
и **только** активную локальную сессию конкретного пользователя. Wrapper проверяет, что
юнит — ровно `ParanoidX-dashboard.service`. Никакого `NOPASSWD: ALL`.

Файлы: `deploy/10-px-node-control.rules`, `deploy/px-node-control`.

### 3.5 Внешние зависимости

| Зависимость | Обязательна? | Поведение при отсутствии |
|---|---|---|
| Ollama (ИИ) | Нет | Нода работает, ИИ-эндпоинты отдают ошибку |
| Telegram | Нет | Алерты молчат, нода работает |
| SimpleX-серверы | Нет | Работает локально, без P2P |
| Интернет | Да | Без интернета не работает SOCKS-мост |

---

## 4. Потоки данных

```
                    ┌──────────────────┐
   Telegram ◄──────►│  Боты (bot)      │
                    └────────┬─────────┘
                             │ HTTP API :8080
   ┌─────────┐   ┌───────────▼───────────┐   ┌──────────────┐
   │ Flutter │──►│  ParanoidX (демон)    │──►│  economy     │
   │ Isle /  │   │  api, registry,      │   │  чеканка,    │
   │ Royal   │   │  transport, gateway  │   │  казна,      │
   └─────────┘   └───────┬───────────────┘   │  серебро     │
                         │ SOCKS 5            └──────────────┘
   ┌─────────┐   ┌───────▼───────────┐   ┌──────────────┐
   │ Монитор │──►│  xray :10810      │   │  SQLite +    │
   │ + трей  │   │  4-слойный прокси │   │  файлы (35 МБ)│
   └─────────┘   └───────┬───────────┘   └──────────────┘
                         │
   ┌─────────┐   ┌───────▼───────────┐   ┌──────────────┐
   │ Подписки │──►│  Tor + SimpleX    │──►│  coturn/     │
   │ (12 ч)   │   │  5223–5225        │   │  WebRTC      │
   └─────────┘   └───────────────────┘   └──────────────┘
```

Монитор и дашборд **не являются источником правды** — они вызывают `node-control.py`,
который проверяет фактическое состояние (процесс, юнит, реальный HTTP-запрос через прокси).

---

## 5. Контроль ноды

### 5.1 Четыре компонента

| Компонент | Проверка состояния | TEST проверяет |
|---|---|---|
| **daemon** | `systemctl is-active` | HTTP 200 от `/api/health` |
| **xray** | процесс + порт 10810 | **реальная загрузка через прокси** (не только TCP-connect) |
| **docker** | 4 контейнера | health-статус каждого |
| **monitor** | user-unit | MainPID, NRestarts |

### 5.2 Принцип «состояние, а не код возврата»

Критичное архитектурное решение. `systemctl stop` не возвращает ошибку при отказе polkit —
он **висит до таймаута и печатает «Method call timed out» с кодом 0**. Наивный код читал бы
код возврата и рапортовал успех. Здесь вердикт принимается по **наблюдаемому состоянию**
(`_settle`), а таймаут выставлен 100 с > `TimeoutStopUSec=90s`.

То же касается `suspend`: маркер ставится **только после** фактической остановки, и
`status_text` показывает «suspended» только если демон реально не запущен.

### 5.3 Протокол подписки xray

Двухфазный, 3 500 кандидатов, ~90 секунд:

1. **Фаза 1 (RTCP):** все кандидаты пингуются **параллельно** (64 потока), напрямую мимо
   моста. Кандидаты с локальными адресами (`127.0.0.1`, `0.0.0.0`) отбрасываются на входе —
   иначе заглушка побеждает все реальные серверы по RTT (0.8 мс против 30 мс).
2. **Фаза 2 (скорость):** шортлист 40 быстрейших проверяется **изолированным xray** на
   отдельном loopback-порту. Main-мост `:10810` не трогается. Порог 200 КБ/с.
3. **Сортировка** по реальной скорости, не по RTT.
4. **Топ-100** (или меньше, если прошли не все) → генерация конфига → `xray -test`.
5. **Apply:** бэкап → запись → перезапуск xray → проверка SOCKS. При неудаче — откат.

Измеренный факт: из 3 511 кандидатов TCP-живых 2 018, но реальную скорость прошли
**6–40**. TCP-connect не доказывает работоспособность прокси.

---

## 6. Известные проблемы и долги

### 6.1 Критичные

| # | Проблема | Влияние | Статус |
|---|---|---|---|
| 1 | **Токен GitHub в remote** | `https://<token>@github.com/...` в `.git/config`. Утечёт при любом клонировании, пробросе конфига, бэкапе на USB | Не исправлено — требует ротации токена |
| 2 | **Flutter-приложения вне git** | Были 207 МБ на `/mnt/data/Export`, вне репозитория. Клон не содержал приложений | **Исправлено**: 2.5 МБ исходников в `apps/` |
| 3 | **20 незакоммиченных изменений** | Вся работа этого цикла (монитор, xray, экономика) не в git | Не исправлено |
| 4 | **Экономика не мигрирована в live-демон** | Код переписан и протестирован, но работающий демон со старой логикой: oracle ≈ 64.4, покрытие ≈ 0.045, `healthy:false` | Требует отдельного migration decision |

### 6.2 Средние

| # | Проблема | Влияние |
|---|---|---|
| 5 | `internal/steward/steward.go` может выставить `SilverBackingRatio=0.50` | Перезаписывает канонические 0.70 при неудачном расчёте |
| 6 | `silver_reserve_ng.txt` и `silver_assets.json` расходятся ≈ в 22 раза | Два источника правды по физическому серебру |
| 7 | Legacy `BalanceNg` / `TotalSupply` / `total_supply_ng` ещё в коде | Два представления денежной массы |
| 8 | Устаревшие юниты `ParanoidX-bridge/admin-bot/royal-bot/poet-bot.service` | Отключены, но в `list-unit-files` занимают место |
| 9 | Имена в коде: `NGPerTLR` (ng, унция) и `NTPerTLR` (нит, монета) | Два представления денежной единицы; `NGPerTLR` помечен deprecated |
| 10 | Xray — нативный процесс, не user-юнит | Управление по PID, гонки при рестарте |
| 11 | 5 предупреждений `dart analyze` в `apps/shared/models` | Неиспользуемые импорты (`dart:convert`, `bip32`, `crypto`, `pointycastle`) и неиспользуемая переменная `edKey`. На сборку не влияют |

### 6.3 Исправлено в этом цикле (Flutter)

| Дефект | Симптом |
|---|---|
| **Дубликат `getIdentity`** в `identity_service.dart` — сломанная копипаста из `getAllIdentities` возвращала `List<Identity>` из функции с типом `Future<Identity?>` | `flutter build` падал: `A value of type 'List<Identity>' can't be returned from an async function` |
| **`apps/shared/models/pubspec.yaml` не объявлял `intl`, `shared_preferences`, `pinenacl`**, хотя код их импортирует | `dart analyze`: `uri_does_not_exist`, `undefined_class SharedPreferences`, `undefined_method DateFormat` |
| **`path: ../../shared/models` в обоих приложениях** не разрешался после переноса в `apps/` | `pub get`: `could not find package models at "../../shared/models"` |
| **Каталоги `linux/`, `macos/`, `web/` не были в копии** | `No Linux desktop project configured` |
| **Отсутствовал тулчейн** `clang`, `cmake`, `ninja-build`, `libgtk-3-dev` | `CMake Error: CMake was unable to find a build program corresponding to "Ninja"` |
| **`bip39.Mnemonic` в `isle_app` — класса не существует.** Пакет `bip39: ^1.0.6` отдаёт верхнеуровневые функции `generateMnemonic(strength:)` и `mnemonicToSeed()`, а не класс | `Error: Undefined name 'Mnemonic'`. Дополнительно передавалось число слов вместо бит энтропии — исправлено на `strength: wordCount.entropyBits` |
| **Конфликт версий `intl`**: `isle_app` требовал `^0.19.0`, `models` — `^0.20.2` | `pub get`: `Try upgrading your constraint on intl: flutter pub add intl:^0.20.2` |

Итог: `dart analyze` в `apps/shared/models` — **0 ошибок** (было 8), 5 предупреждений.
Оба приложения собираются из репозитория.

### 6.4 Монитор не стартовал после перезагрузки (исправлено)

Симптом: после ребута `node-monitor.service` оставался мёртвым
(`monitor=stopped` в CLI), при этом юнит был `enabled`.

Три независимые причины, все подтверждены в журнале:

1. **Старт до графической сессии.** Юнит был `WantedBy=default.target`; вместе с
   `Linger=yes` это поднимало его ДО входа в GNOME, когда X ещё не существует:
   ```
   Can't create a GtkStyleContext without a display connection
   Main process exited, code=dumped, status=6/ABRT
   ```
   systemd делал 5 попыток за 5 минут, упирался в `StartLimitBurst=5` и
   оставлял монитор мёртвым. Исправлено: `WantedBy=graphical-session.target`.

2. **Жёстко зашитый `DISPLAY=:0`.** Номер дисплея меняется между сессиями, а
   `~/.Xauthority` на этой машине не существует: сессия — Wayland
   (`wayland-0`), Xwayland есть лишь для совместимости. X-ориентированный
   launcher ждал бы cookie, которой не бывает. Исправлено: новый
   `scripts/node-monitor-launch.sh` сам определяет бэкенд — сначала Wayland
   (сокет в `$XDG_RUNTIME_DIR`), затем X11 как запасной путь, и выставляет
   `GDK_BACKEND`. Если сессии нет, launcher ждёт до 60 с и выходит с кодом 0,
   а не роняет процесс.

3. **Утечка `PYTHONPATH` (главная причина).** Монитор импортировал PIL из venv
   Hermes:
   ```
   ImportError: cannot import name '_imaging' from 'PIL'
   (/home/tomas/.hermes/hermes-agent/venv/lib/python3.11/site-packages/PIL)
   ```
   Модуль, собранный под 3.11, не грузится в системном 3.14, и весь процесс
   падал с `status=1`. Дефект невидим ни для `go test`, ни для ручного запуска
   внутри шелла Hermes — поэтому он и дожил до ребута. Исправлено:
   `unset PYTHONPATH` в launcher перед `exec`.

Побочно найдено: **`xdpyinfo` на несуществующем display не отказывает сразу,
а висит.** Измерено: `DISPLAY=:99` → отказ за 177 мс (сервера нет вовсе),
но `DISPLAY=:1` → **25 с** (X в системе есть, но дисплей не тот). Опасный
случай — именно «дисплей не тот», а не «сервера нет». Без `timeout 2` launcher
простаивал бы по 25 с на каждой попытке и не дождался бы сессии. Исправлено.

Проверено: имитация загрузки (stop + start) даёт `active` за 0 с, `MainPID`
живой, `NRestarts=0`, в логе 0 ошибок.

Примечание: автологин GDM (`AutomaticLoginEnable=True` в
`/etc/gdm3/custom.conf`) убирает саму гонку, но **не отменяет пункты 2 и 3** —
без фиксов монитор всё равно упал бы.

### 6.5 Мелкие

- `AGENTS.md` устарел: указывает `cmd/simplex-node/main.go` (не существует), «18 пакетов» (на самом деле 40), старые пороги токеномики.
- Имена в коде: `NGPerTLR` (ng, унция) и `NTPerTLR` (нит, монета) параллельны.
- Xray — нативный процесс, не user-юнит: управление по PID, гонки при рестарте.

---

## 7. Проверка состояния

```bash
# Каноническая проверка (36 пакетов)
go test ./... -short -count=1 -timeout 120s

# Сборка
go build ./cmd/ParanoidX/ && go vet ./...

# Нода жива?
curl -s http://127.0.0.1:8080/api/health | jq .

# Прокси реально ходит?
curl -sS --socks5-hostname 127.0.0.1:10810 -o /dev/null \
  -w '%{http_code} %{speed_download}\n' 'https://speed.cloudflare.com/__down?bytes=2000000'

# Все компоненты
python3 node-control.py cli state
# → daemon=running docker=4 up monitor=running xray=running
```

Фактическое состояние на момент аудита: `healthy: true`, прокси 1.47 МБ/с,
4 контейнера, 36/36 тестов, все 4 компонента `running`.

---

## 8. Что можно клонировать, а что нет

| Компонент | В git? | Комментарий |
|---|---|---|
| Go-исходники (68 К строк) | ✅ | Единственный источник правды |
| `node-monitor.py`, `node-control.py`, `xray-sub.py` | ⚠️ | В рабочей копии, но **не закоммичены** |
| `deploy/` (скрипты установки) | ⚠️ | Созданы, не закоммичены |
| Flutter-исходники | ✅ | Перенесены в `apps/` (2.5 МБ) |
| `docker/docker-compose.yml` | ✅ | В репозитории |
| Конфиг с секретами | ❌ | По правилам — в `.gitignore`. Создаётся из шаблона |
| Данные SQLite, кошельки, состояние | ❌ | Локальное состояние, не переносится |
| Бинарные ключи, токены, адреса | ❌ | Генерируются на месте |

**Вывод: клон воспроизводим на 100% функционально, но только после коммита
незакоммиченных изменений и замены токена в remote.**
