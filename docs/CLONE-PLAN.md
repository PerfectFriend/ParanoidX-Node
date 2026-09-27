# План полного клона ParanoidX через GitHub

Цель: перенести ноду на новое устройство (цель — Lenovo Ser9) и повторять цикл обновлений
без потери данных и без ручной работы.

---

## 0. Что блокирует клонирование сейчас

Прежде чем клонировать, нужно закрыть три вещи. Иначе клон будет неполным или утечёт.

| # | Блокер | Что сделать | Почему важно |
|---|---|---|---|
| 1 | **Токен в git remote** | Ревокать старый токен на GitHub, создать новый, записать в `~/.git-credentials` или SSH | Токен в `.git/config` утечёт при копировании репозитория, бэкапе на USB и в любом CI |
| 2 | **20 незакоммиченных изменений** | Закоммитить: `node-monitor.py`, `node-control.py`, `xray-sub.py`, `deploy/`, `apps/`, `docs/`, `cmd/ParanoidX/control_api.go`, изменения экономики | Без этого клон не содержит ни монитора, ни подписок, ни приложений |
| 3 | **Секреты вне git** | Убедиться, что `simplex-node.json`, `*.token`, ключи — в `.gitignore` (проверено) | Правильно, оставляем |

Проверка перед клонированием:

```bash
cd ~/simplex-node
git status --porcelain              # должно быть пусто
git remote -v | grep -q '@github' && echo "ВНИМАНИЕ: токен в remote"
grep -rn 'ghp_\|github_pat' . --include='*.go' --include='*.py' --include='*.sh' 2>/dev/null
```

---

## 1. Подготовка репозитория (один раз)

```bash
cd ~/simplex-node

# 1. Убрать токен из remote
git remote set-url origin git@github.com:DarkPushkin/ParanoidX.git
# или с credential helper:
git remote set-url origin https://github.com/DarkPushkin/ParanoidX.git

# 2. Закоммитить всё, что накопилось
git add -A
git status --porcelain | head -30      # проверить, что попадает
git commit -m "Node Control: polkit, 12h refresh timer, dashboard UI, apps/ в репозиторий, deploy-скрипты, аудит"
git push origin main
```

**Проверка, что секреты не уедут:**

```bash
git diff --cached --name-only | grep -E 'simplex-node\.json|\.token$|\.key$|state\.json' \
  && echo "СТОП: секрет в индексе" || echo "OK: секретов нет"
```

> `deploy/simplex-node.json.template` содержит только плейсхолдеры вида `ЗАПОЛНИ:` —
> это безопасно, он и должен быть в репозитории.

---

## 2. Клонирование на новое устройство

```bash
# На новой машине
git clone git@github.com:DarkPushkin/ParanoidX.git ~/simplex-node
cd ~/simplex-node

# Проверить, что всё на месте
echo "Go-пакеты:   $(find . -name '*.go' -not -path './vendor/*' | wc -l) файлов"
echo "Dart:        $(find apps -name '*.dart' | wc -l) файлов"
echo "Скрипты:     $(ls deploy/ | wc -l)"
```

Ожидаемо: 283 `.go`, 61+ `.dart`, 6–7 файлов в `deploy/`.

---

## 3. Установка одной командой

```bash
cd ~/simplex-node
./deploy/px-install.sh
```

Скрипт идемпотентен (можно запускать повторно для обновления) и делает 7 шагов:

| Шаг | Что делает | Нужны права |
|---|---|---|
| 1 | Пакеты: Go, Docker, Python, Tor, xray | sudo (внутри скрипта) |
| 2 | `go build -o ~/bin/ParanoidX ./cmd/ParanoidX/` | — |
| 3 | Xray: проверяет бинарь, ставит из apt если нет | sudo |
| 4 | Docker-стек: `docker compose up -d` (tor, coturn, smp, xftp) | sudo для docker |
| 5 | Polkit-правило + wrapper (подставляет имя пользователя) | sudo |
| 6 | user-юниты: монитор + 12-часовой таймер (подставляет пути) | — |
| 7 | Flutter: собирает `isle_app` и `royal_app` из `apps/` | — |

Проверить без установки (ничего не меняет):

```bash
DRY_RUN=1 ./deploy/px-install.sh
```

### Что скрипт НЕ делает намеренно

- **Не генерирует секреты.** Токены Telegram, кошельки, адреса — только через шаблон.
- **Не запускает ноду.** Шаг 8 отдельный: сначала заполни конфиг.
- **Не удаляет данные.** `docker compose down` без `-v`, `docker system prune` не вызывается.

---

## 4. Конфигурация новой машины

```bash
mkdir -p ~/.local/share/simplex-node
cp ~/simplex-node/deploy/simplex-node.json.template \
   ~/.local/share/simplex-node/simplex-node.json
chmod 600 ~/.local/share/simplex-node/simplex-node.json
$EDITOR ~/.local/share/simplex-node/simplex-node.json
```

Заполнить:

| Поле | Источник | Обязательно |
|---|---|---|
| `ask_steward_token` | @BotFather | Нет |
| `dark_pushkin_token` | @BotFather | Нет |
| `torquemada_token` | @BotFather | Нет |
| `inquisitor_token` | @BotFather | Нет |
| `*_chat_id` | id чата (положительное) или супергруппы (отрицательное) | Нет |
| `radio_dir` | путь к медиа на этой машине | Да, если используется |
| `proxy_sub_urls` | свои подписки (URL не должен попасть в логи) | Нет |
| `onion_address` | пусто — сгенерируется при первом старте | Нет |

> **Приватная подписка.** Когда дашь ссылку на закрытую подписку, её нужно добавить
> в `proxy_sub_urls`. URL не должен попадать в git, логи или Telegram — все три места
> сейчас фильтруются, но при добавлении проверь вывод `xray-sub.py status`.

---

## 5. Запуск

```bash
# 5.1 Убедиться, что xray живой (иначе нода без интернета)
~/bin/v2ray/xray version
# Если конфига ещё нет — первый прогон подписки создаст его:
python3 ~/simplex-node/xray-sub.py update

# 5.2 Запуск ноды
cd ~/simplex-node && ./scripts/launch-node.sh

# 5.3 Проверка
curl -s http://127.0.0.1:8080/api/health | jq .
python3 ~/simplex-node/node-control.py cli state
# Ожидаемо: daemon=running docker=4 up monitor=running xray=running
```

### systemd-юнит вместо ручного запуска

На новой машине системного юнита может не быть. Создать:

```bash
sudo tee /etc/systemd/system/ParanoidX-dashboard.service > /dev/null <<'EOF'
[Unit]
Description=ParanoidX node daemon
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
User=%u
WorkingDirectory=%h/simplex-node
ExecStart=%h/bin/ParanoidX -config %h/.local/share/simplex-node/simplex-node.json
Restart=always
RestartSec=5
# Stop может занять до 90 с (TimeoutStopUSec) — node-control это учитывает.
TimeoutStartSec=1800

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload
sudo systemctl enable --now ParanoidX-dashboard.service
```

Затем `sudo systemctl enable --now ParanoidX.target` — чтобы нода поднималась
вместе со стеком.

---

## 6. Flutter-приложения

```bash
# Проверить наличие SDK
flutter --version        # нужен Flutter 3.44+ или новее

# Тулчейн для сборки desktop. Без него Flutter пишет
# "CMake was unable to find a build program corresponding to Ninja"
# и НЕ собирает приложение.
sudo apt install clang cmake ninja-build pkg-config libgtk-3-dev g++

# Сборка из репозитория
cd ~/simplex-node/apps/royal_app
flutter pub get
flutter build linux --release
# → build/linux/x64/release/bundle/
```

Собираются из `apps/`, все path-зависимости (`apps/shared/models`, `api_client`,
`widgets`) лежат в репозитории. `px-install.sh` делает это сам на шаге 7, включая
установку тулчейна.

**Структура в репозитории:**

```
apps/
├── isle_app/      The Isle
├── royal_app/     Royal Isle
└── shared/
    ├── models/        ← импортируется обоими приложениями
    ├── api_client/    ← импортируется isle_app
    └── widgets/       ← импортируется isle_app
```

В `pubspec.yaml` приложений стоит `path: ../shared/...` — относительный путь от
`apps/<app>/` до `apps/shared/`. Если переносишь приложения в другую структуру,
эти пути надо поправить, иначе `pub get` скажет
`could not find package models`.

**Версии должны совпадать во всех пакетах.** `models` использует `DateFormat`
(`intl`) и `SharedPreferences`, поэтому объявляет `intl`, `shared_preferences`
и `pinenacl`. `isle_app` раньше требовал `intl: ^0.19.0`, а `models` — `^0.20.2`;
pub отказывался решать:

```
Because isle_app depends on intl from path which doesn't exist ...
Try upgrading your constraint on intl: flutter pub add intl:^0.20.2
```

Сейчас `intl: ^0.20.2` во всех трёх манифестах. Если добавляешь пакет в `apps/shared/`
— синхронизируй версии во всех `pubspec.yaml` сразу, иначе сборка одного
приложения сломает другое.

**Проверка разрешимости зависимостей без сборки:**

```bash
cd ~/simplex-node/apps/shared/models && flutter pub get && dart analyze
# 0 errors — манифест полон и импорты разрешаются
```

**Desktop-файлы для меню:**

```bash
mkdir -p ~/.local/share/applications
# the-island.desktop и royal-island.desktop
# Exec=/home/<user>/.local/bin/the-isle/isle_app
```

> Сборка desktop занимает 5–8 минут. `px-install.sh` проверяет комплектность
> bundle (`libapp.so` + `libflutter_linux_gtk.so` + бинарник) и **не** устанавливает
> недособранное приложение.

**`pubspec.lock` обязателен в git.** Он фиксирует точные версии зависимостей;
без него клон подтянет другие версии, и сборка, проверенная на старой машине,
сломается на новой. В `.gitignore` есть `*.lock`, но ниже стоит
`!apps/*/pubspec.lock`. Для `apps/shared/*` (библиотеки) lock не коммитится —
версии там задаёт lock приложения, которое их подключает.

Проверить после клона:

```bash
ls apps/isle_app/pubspec.lock apps/royal_app/pubspec.lock   # оба должны быть
flutter pub get --offline                                  # должен пройти без сети
```

---

## 7. Обновления (регулярный цикл)

```bash
cd ~/simplex-node

# Получить
git pull origin main

# Пересобрать и перезапустить
go build -o ~/bin/ParanoidX ./cmd/ParanoidX/
sudo systemctl restart ParanoidX-dashboard.service

# Проверить
go test ./... -short -count=1 -timeout 120s
python3 node-control.py cli state
curl -s http://127.0.0.1:8080/api/health | jq .
```

Или одной командой — скрипт идемпотентен:

```bash
git pull && ./deploy/px-install.sh && sudo systemctl restart ParanoidX-dashboard.service
```

---

## 8. Миграция данных (если нужна нода с историей)

Код клонируется, а состояние — нет. Если нужна история кошельков, транзакций иsilver-обеспечения:

```bash
# На старой машине
tar czf ~/migration-state.tar.gz \
    ~/.local/share/simplex-node/     # SQLite, конфиг, кошельки
    ~/bin/v2ray/xray-last-good.json   # последний рабочий прокси
    ~/bin/v2ray/servers.json          # рейтинг серверов

# Перенос
scp ~/migration-state.tar.gz newhost:~/

# На новой машине
sudo systemctl stop ParanoidX-dashboard.service
tar xzf ~/migration-state.tar.gz -C /
sudo systemctl start ParanoidX-dashboard.service
```

**Что переносить, а что нет:**

| Переносим | Не переносим |
|---|---|
| `~/.local/share/simplex-node/` (БД, кошельки) | Скомпилированные бинари (собираются) |
| `xray-last-good.json` | `servers.json` (пересобирается за 90 с) |
| Конфиг с секретами (вручную, защищённо) | Docker volume'ы (создаются автоматически) |

---

## 9. Проверка клона (acceptance criteria)

```bash
# 1. Сборка
go build ./... && go vet ./... && echo "BUILD OK"

# 2. Тесты
go test ./... -short -count=1 -timeout 120s | grep -c '^ok'   # ожидаемо 36
go test ./... -short -count=1 -timeout 120s | grep -c '^FAIL'  # ожидаемо 0

# 3. Нода
curl -s http://127.0.0.1:8080/api/health | jq -e '.healthy == true'

# 4. Прокси реально ходит
curl -sS --socks5-hostname 127.0.0.1:10810 -o /dev/null \
     -w '%{http_code}\n' 'https://speed.cloudflare.com/__down?bytes=1000000'   # ожидаемо 200

# 5. Контейнеры
docker ps --format '{{.Names}} {{.State}}' | grep -c running                  # ожидаемо 4

# 6. Монитор
systemctl --user is-enabled node-monitor.service      # enabled
systemctl --user is-active node-monitor.service       # active
systemctl --user show node-monitor.service -p NRestarts -p MainPID
#   ожидаемо: NRestarts=0 (нет цикла падений), MainPID≠0.
#   Если NRestarts растёт — смотри journalctl, скорее всего упал PIL
#   (см. ниже про PYTHONPATH).
systemctl --user is-active px-xray-refresh.timer      # active

# launcher обязателен: он сам находит Wayland/X11 и чистит PYTHONPATH
test -x scripts/node-monitor-launch.sh && echo "launcher на месте"

# 7. Приложения
ls ~/.local/bin/the-isle/isle_app ~/.local/bin/the-royal/royal_app 2>/dev/null
```

**`Linger` и автологин.** Если у пользователя включён `loginctl enable-linger`,
user-юниты стартуют ДО входа в графическую сессию. Именно поэтому монитор
привязан к `graphical-session.target`, а не `default.target`, и запускается
через `scripts/node-monitor-launch.sh`. Не «упрощай» юнит, не убрав
`PartOf=graphical-session.target` — вернётся `status=6/ABRT` и мёртвый
монитор после каждого ребута.

**`PYTHONPATH` в окружении.** Если в системе задан `PYTHONPATH`, указывающий
на venv с Python другой версии, `node-monitor.py` возьмёт оттуда PIL и упадёт:

```
ImportError: cannot import name '_imaging' from 'PIL'
```

Launcher делает `unset PYTHONPATH` перед запуском. Не удаляй эту строку.

Клон считается рабочим, если все 7 пунктов проходят.

---

## 10. Проблемные места миграции

| Проблема | Влияние | Решение |
|---|---|---|
| **Токен в remote** | Утечка при клонировании | Ревокать, заменить на SSH или credential helper |
| **`isle_app` не собирается** | Приложение The Isle недоступно | Разобрать сборку; скрипт честно не ставит сломанное |
| **Путь к радио на USB** | На новой машине диска нет | Задать `radio_dir` на постоянный диск |
| **Docker на новой системе** | Ubuntu/Debian — `docker.io`, Fedora — `docker-ce` | `px-install.sh` ставит `docker.io`; для RHEL поправить |
| **Polkit требует активную сессию** | На headless-машине кнопки ON/OFF демона не сработают | Либо图形ческая сессия, либо управление через `sudo` вручную |
| **Flutter SDK ~1 ГБ** | Не на каждой машине | Ставить отдельно; приложения опциональны |
| **Экономика не мигрирована** | Демон работает на старой логике | Отдельное решение, не блокирует клонирование кода |

---

## 11. Порядок действий (TL;DR)

```bash
# Один раз, на старой машине:
cd ~/simplex-node
git remote set-url origin git@github.com:DarkPushkin/ParanoidX.git
git add -A && git commit -m "Node Control + apps + deploy + audit" && git push

# На новой машине:
git clone git@github.com:DarkPushkin/ParanoidX.git ~/simplex-node
cd ~/simplex-node
./deploy/px-install.sh
cp deploy/simplex-node.json.template ~/.local/share/simplex-node/simplex-node.json
$EDITOR ~/.local/share/simplex-node/simplex-node.json
python3 xray-sub.py update
./scripts/launch-node.sh
python3 node-control.py cli state
```

Дальше обновления — `git pull && ./deploy/px-install.sh`.
