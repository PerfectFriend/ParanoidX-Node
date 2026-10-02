# ParaNodeX — App Transport

Производное от `ParanoidX` (CYCLE-63, `f1eba82`). Цель: перенести в ParanoidX
заготовку транспорта из заброшенного `KiloParanoidX` и починить в ней
найденные дефекты.

Ветка: `feature/app-transport`. Оригинальный ParanoidX не тронут.

## Что перенесено

| Файл | Источник | Состояние |
|------|----------|-----------|
| `internal/apptransport/envelope.go` | KiloParanoidX | перенесён, **исправлен** |
| `internal/apptransport/queue.go` | KiloParanoidX | перенесён как есть |
| `internal/apptransport/replay.go` | KiloParanoidX | перенесён как есть |
| `internal/apptransport/signal.go` | KiloParanoidX | перенесён как есть |
| `internal/apptransport/transport.go` | KiloParanoidX | перенесён, путь модуля исправлен |
| `internal/apptransport/types.go` | KiloParanoidX | перенесён, схема полей расширена |
| `internal/socks5/` | KiloParanoidX | перенесён как есть |
| `internal/apptransport/crypto.go` | **новое** | шифрование payload |
| `internal/apptransport/keyring.go` | **новое** | per-app ключи |
| `internal/apptransport/crypto_test.go` | **новое** | 7 тестов |
| `internal/apptransport/keyring_test.go` | **новое** | 5 тестов |

НЕ перенесено (в ParanoidX есть свои версии): `cmd/*`, `internal/bridge`,
`internal/vpn`, `internal/paranoid`, `internal/paranoidx`, UI/`web/`.

## Дефекты исходника и что с ними

### 1. Payload только подписывался, не шифровался

KiloParanoidX подписывал конверт HMAC-SHA256, но payload шёл открытым.
Приложение с тем же ключом читало трафик другого. Для «изоляции
приложений» это не работает — это только защита от подделки снаружи.

**Исправлено:** `crypto.go`, XChaCha20-Poly1305 по образцу `internal/container`.
Plaintext и шифротекст разнесены по разным полям (`Payload` / `Ciphertext`),
так что отправить незашифрованный payload нельзя структурно.

### 2. Срок жизни конверта не ограничен сверху

```go
// было
if e.TTL > 0 && e.Timestamp+e.TTL < now.Unix() { return ErrExpired }
```

Отбрасывались только просроченные. При `TTL == 0` проверка не выполнялась
вовсе, а метка «далеко в будущем» оставалась валидной бесконечно — окно
перехвата не ограничено, что обессценивает replay-guard.

**Исправлено:** `CheckTimestamp` — двусторонняя граница, `MaxClockSkew = 5 мин`.

### 3. Один ключ на процесс вместо одного на приложение

`Secret: []byte` — единственный ключ на весь Transport. Поле `channel_id`
на изоляцию не влияло: это метка, которую можно подделать, имея общий ключ.

**Исправлено:** `keyring.go`. Каждое приложение регистрируется отдельно и
получает свои `Secret`, `EncKey`, `AuthKey`. Изоляция проверяется тестом
`TestAppCannotReadAnotherAppsTraffic`.

### 4. Подпись не покрывала payload

В исходнике `canonicalEnvelope` включал `Payload`, но шифротекста не
существовало. После добавления шифрования подпись обязана покрывать оба
поля, иначе атакующий подменит запечатанные байты, не трогая MAC.

**Исправлено:** в `canonicalEnvelope` входят `Payload`, `Ciphertext`,
`Encrypted`. Проверено мутацией.

### 5. Случайный секрет генерировался на каждый запуск

`normalizeConfig` при пустом `Secret` вызывал `RandomSecret()` — после
рестарта подпись переставала работать.

**Оставлено как есть, но помечено:** это поведение нужно для одноразовых
инстансов. Для постоянного транспорта секрет обязан поступать извне через
`Keyring.Register`/`Adopt`.

## Как пользоваться

```go
kr := apptransport.NewKeyring()
cards, _ := kr.Register("cards")          // свои ключи
back, _ := kr.Register("backgammon")      // другие ключи

env, _ := cards.NewEnvelopeFor(apptransport.PublishRequest{
    Type: "message", ChannelID: "game-1",
    Payload: []byte(`{"hand":["A","K"]}`),
})

// отправитель
_ = transport.PublishEnvelope(ctx, env)

// получатель — только владелец ключа расшифрует
payload, err := cards.Open(env, time.Now().UTC(), replayGuard)
// back.Open(env, ...) вернёт ошибку подписи
```

Онбординг второго узла: экспортировать публичную половину
(`MarshalPublic` — только имя и соль, секрет не экспортируется), передать
секрет вне полосы, вызвать `Adopt`.

## Проверка

```bash
go build ./cmd/ParanoidX/
go vet ./internal/apptransport/ ./internal/socks5/
go test ./internal/apptransport/ ./internal/socks5/ -count=1
```

15 тестов в apptransport (8 перенесённых + 7 новых), 2 в socks5. Каждый новый
тест содержит отрицательное утверждение: он обязан падать, когда защита снята.

Известный красный тест в проекте, **не связанный с этим переносом**:
`TestChatArchiveLifecycle` в `internal/api` (`expected 1 archived message, got 2`)
— был красным до переноса.

## Что не сделано

- **Нет интеграционного теста с двумя независимыми экземплярами** по сети.
  Изоляция проверена на уровне ключей, но не сквозным сценарием двух процессов.
- **Нет сервисного слоя**: `PublishEnvelope`/`Open` не соединены с
  HTTP-роутами ParanoidX. Транспорт пока библиотека, а не работающий сервис.
- **Аутентификация входа не трогалась.** В KiloParanoidX HTTP-API не имел
  аутентификации при `-listen 0.0.0.0` — это отдельная проблема ParanoidX,
  и она не входит в этот перенос.
- **Захардкоженный onion не переносился** — в ParanoidX адреса уже в конфиге.