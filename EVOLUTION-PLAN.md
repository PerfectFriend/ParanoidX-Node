# ParanoidX — Evolution Plan

**Project:** ParanoidX (Go Server + Infrastructure)
**Current Cycle:** 16 (v px-node-C41-C60)
**Next Cycle Target:** 17
**Status:** Production — running on Dell Latitude 3150 (dev), ready for Beelink SER9 migration

---

## Evolution Philosophy

> **Code that writes code. Infrastructure that deploys infrastructure. Economy that backs itself with silver.**

Each cycle = 1 evolutionary step. 8-step SOP (PRODUCTION-CYCLE.md). Approval gates via Telegram buttons.

---

## Completed Cycles (History)

| Cycle | Codename | Key Achievement |
|-------|----------|-----------------|
| 1-10 | Genesis | simplex-node born, economy core, chat bridge, radio |
| 11 | Torification | Full Tor routing, .onion addresses |
| 12 | ParanoidX | Multi-layer routing (VLESS/VMess/Tor), Docker orchestration |
| 13 | Silver Standard | 70% physical silver backing, dividend engine |
| 14 | Governance | Constitution, proposals, weighted voting |
| 15 | AI Integration | Ollama (gemma4/qwen2.5), Steward bot (@AskSteward_bot) |
| 16 | Restructure | Monorepo → ParanoidX / The-Isle / Royal-Isle / shared-libs |

---

## Current Cycle: 17 — "Beelink Migration"

**Target Hardware:** Beelink SER9 (24GB RAM / 500GB SSD)
**Migration Date:** 2026-08-01 to 2026-08-03

### Objectives

| # | Task | Status |
|---|------|--------|
| 1 | Bootstrap script: install Go, Flutter, Docker, Ollama, Hermes | 🔲 |
| 2 | Clone all repos from GitHub (the-grimoire, ParanoidX, The-Isle, Royal-Isle, shared-libs) | 🔲 |
| 3 | Run `bootstrap.sh` — idempotent setup | 🔲 |
| 4 | Build ParanoidX binary | 🔲 |
| 5 | Configure systemd services (ParanoidX-*) | 🔲 |
| 6 | Start Docker stack (5 services) | 🔲 |
| 7 | Verify health endpoint + bridge | 🔲 |
| 8 | Migrate data from Dell (`~/.local/share/simplex-node/`) | 🔲 |
| 9 | Update DNS / Tor hidden service keys | 🔲 |
| 10 | Telegram approval: "Migration Complete ✅" | 🔲 |

---

## Upcoming Cycles (18-30)

### Cycle 18: "Observability"
- Structured logging aggregation (Loki/Grafana or simple JSON tail)
- Metrics endpoint expansion (`/api/admin/metrics` → Prometheus format)
- Health check matrix (bridge, docker, tor, ollama, disk, mem)
- Alerting via Telegram (Inquisitor bot)

### Cycle 19: "Resilience"
- Automated backup to USB (encrypted, verified)
- Disaster recovery test (restore from backup on clean VM)
- Multi-instance HA (active/passive with shared data dir)
- Chaos engineering: kill random container, verify recovery

### Cycle 20: "Economy Hardening"
- Oracle redundancy (multiple price sources, median)
- Deflation engine automation
- Dividend distribution scheduler (cron + verification)
- Audit trail: immutable log of all treasury ops

### Cycle 21: "Chat Federation"
- SimpleX group management via API
- Message retention policies
- Bridge to Matrix / Nostr (experimental)
- Encrypted backups of chat history

### Cycle 22: "Radio 2.0"
- AI content pipeline: prompt → script → TTS → mix → schedule
- Listener analytics (anonymous, local)
- Offline sync for Isle app
- Emergency broadcast override

### Cycle 23: "ParanoidX VPN — Multi-Protocol Core"
- Visual route map (world map + latency heatmap)
- One-tap route switching (VLESS ↔ VMess ↔ Trojan ↔ Shadowsocks ↔ Tor ↔ WireGuard ↔ OpenVPN)
- Kill switch integration (systemd + nftables)
- Split tunneling rules (Island traffic vs clearnet)
- **Protocol Support Matrix:**
  - VLESS (Reality/XTLS) ✅
  - VMess (WebSocket+TLS) ✅
  - Trojan (TLS) 🔲
  - Shadowsocks (AEAD) 🔲
  - WireGuard (kernel/userspace) 🔲
  - OpenVPN (UDP/TCP) 🔲
  - Hysteria2 (QUIC) 🔲
  - TUIC (QUIC) 🔲

### Cycle 23b: "ParanoidX VPN Dashboard — Config & Test Center"
- **Unified Config Dashboard** (`/api/paranoidx/vpn/config`)
  - Text input per protocol (single config or batch)
  - File upload (.conf, .json, .ovpn, .toml)
  - Real-time validation (syntax, required fields)
  - Versioned config history with rollback
- **Protocol Templates Library**
  - Pre-built configs for common providers
  - QR code import/export
  - Subscription link parsing (clash/v2ray/sub)
- **Live Testing Suite** (`/api/paranoidx/vpn/test`)
  - Connectivity check (TCP/UDP handshake)
  - Latency measurement (ping, 3 samples avg)
  - Speed test (download/upload via iperf3 or HTTP)
  - Tor circuit verification (exit IP check)
  - DNS leak test
  - Results stored, graphed, comparable
- **WebSocket Live Metrics** for dashboard
- **One-click apply** → hot-reload container → verify → persist

### Cycle 24: "DC Cloud"
- Distributed compute nodes (Beelink + Lenovo + MacBook)
- Task queue (FFmpeg, Whisper, Ollama inference)
- Result aggregation + verification
- Payment in NG for compute

### Cycle 25: "Isle App Completion"
- sqlite3 build fix (pre-built binaries)
- Identity service (BIP39 + Ed25519)
- Full wallet flow
- Market + escrow
- Chat integration

### Cycle 26: "Royal Polish"
- Multi-profile (Inquisitor/Auditor/Steward)
- Real-time WebSocket metrics
- AI Office prompt library
- Animated heraldry onboarding

### Cycle 27: "Sovereign Deploy"
- One-command deploy to any Linux box
- ARM64 build (Raspberry Pi 5, Orange Pi)
- Tailscale mesh for admin access
- Air-gapped signing device support

### Cycle 28: "Testnet Launch"
- Public testnet (invite-only)
- Faucet (test NG)
- Stress test (1000 simulated users)
- Bug bounty (NG rewards)

### Cycle 29: "Mainnet Preparation"
- Security audit (code + infra)
- Legal framework (Saint Mary Liberty Island charter)
- Silver vault audit (physical)
- Documentation freeze

### Cycle 30: "Genesis Block"
- Mainnet launch
- First 100 TLR minted
- Inquisitor address: 0x001
- **The Island lives.**

---

## Resource Requirements (Beelink SER9)

| Resource | Allocation | Notes |
|----------|------------|-------|
| **RAM** | 24 GB | Ollama (8-16GB) + Go + Docker + Flutter build |
| **SSD** | 500 GB | 100GB data, 100GB models, 100GB Docker, 50GB build, 150GB buffer |
| **CPU** | 8C/16T (Ryzen 9) | Parallel builds, concurrent Ollama |
| **Network** | 1 Gbps + Tor | Clearnet for builds, Tor for production |

---

### Cycle 31: "Grimoire Internationalization"

**Objective:** Rewrite the Grimoire (the-grimoire repo) in English with bilingual structure.

**Structure:**
```
/the-grimoire/
├── README.md              # English (root)
├── ru/                    # Russian version (complete mirror)
│   ├── README.md
│   ├── manifests/
│   │   ├── MANIFESTO.md
│   │   ├── EVOLUTION-SOP.md
│   │   ├── AUTONOMOUS-ARCHITECTURE.md
│   │   └── TELEGRAM-GATEWAY.md
│   ├── configs/
│   ├── scripts/
│   ├── templates/
│   └── skills-export/
└── en/                    # English version (complete mirror)
    ├── README.md
    ├── manifests/
    │   ├── MANIFESTO.md
    │   ├── EVOLUTION-SOP.md
    │   ├── AUTONOMOUS-ARCHITECTURE.md
    │   └── TELEGRAM-GATEWAY.md
    ├── configs/
    ├── scripts/
    ├── templates/
    └── skills-export/
```

**Tasks:**
- Move all current Russian content to `ru/` preserving folder structure
- Translate all markdown files to English in `en/`
- Root `README.md` becomes English entry point with language selector
- Update `bootstrap.sh` to support both locales
- Git history preserved via `git mv` for all moved files

**Deliverable:** Bilingual Grimoire ready for international contributors.

---

### Cycle 32: "Dual-Model Brainstorming" (Thomas ↔ Torquemada)

**Objective:** Научиться вести связный диалог с ботом Торквемада как с обычным пользователем — проводить мозговые штурмы двумя разными моделями: Thomas (deepseek-4) ↔ Torquemada (NVIDIA nemotron 550b).

**Механика канала (уже есть):**
- Bot API не отдаёт сообщения чужих ботов → прямое «видение» реплик Торквемады закрыто
- Решение — `cats-ear.py` (Telethon-ухо, Alicia Babaika id=6942355853): читает ВСЕ сообщения группы в `~/.hermes/cats-bridge/inbox.jsonl`, включая сообщения Торквемады
- Ответы Торквемаде шлёт бот Tomas (Bot API) — он видит упоминания/reply, а ухо доклеивает его реплики

**Задачи:**
| # | Task | Status |
|---|------|--------|
| 1 | Восстановить автозапуск cats-ear.py (systemd user unit / cron, сейчас только ручной) | 🔲 |
| 2 | Научиться читать inbox.jsonl и связывать реплики Торквемады с диалоговым контекстом (reply_to, thread_id) | 🔲 |
| 3 | Разработать протокол мозгового штурма: тема → обмен репликами → синтез выводов → отчёт | 🔲 |
| 4 | Провести первый штурм (2+ модели, 3+ обмена) и сохранить результат | 🔲 |
| 5 | Проверить отсутствие петель: ухо игнорирует свои ID (8863122561, 6942355853), бот не отвечает сам себе | 🔲 |

**Критерий успеха:** связный диалог 2 моделей на 3+ реплики с содержательным синтезом.

---

## Model Zoo (Ollama on Beelink)

| Model | Size (Q4) | Use Case | Priority |
|-------|-----------|----------|----------|
| `gemma2:27b` | ~15 GB | Steward (primary) | ✅ Must |
| `qwen2.5:7b` | ~4.5 GB | Steward (fallback) | ✅ Must |
| `phi3.5:3.8b` | ~2.3 GB | Lightweight tasks | 🔲 Nice |
| `llama3.2:3b` | ~2 GB | Quick replies | 🔲 Nice |
| `nomic-embed-text` | ~0.5 GB | Embeddings/RAG | 🔲 Must |
| `bge-m3` | ~1.5 GB | Multilingual embed | 🔲 Nice |

---

## Skill Exports (Hermes)

All Hermes skills → `the-grimoire/skills-export/`
- Auto-export on each cycle completion
- Versioned with cycle number
- Bootstrap script installs on new machine

---

## Approval Gates

Each cycle requires **Inquisitor approval** via Telegram buttons:

```
[✅ Approve Cycle N]  [❌ Reject]  [🔄 Request Changes]
```

Auto-deploy on ✅. Rollback script on ❌.

---

*"Eight steps. Infinite cycles. One Island."*

**Next Review:** Cycle 17 completion — Beelink SER9 online.