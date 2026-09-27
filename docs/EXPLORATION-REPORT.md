# simplex-node — Full Exploration Report

**Date:** 2026-06-10 | **Version:** A3.2 (Cycle 38) | **Lines:** ~1889 (main.go)
**Codename:** "The Isle" / "Остров" | **Language:** Go 1.25 | **License:** MIT

---

## 1. Overview

Simplex-node is a sovereign digital nation server — a single Go binary that combines a **silver-backed digital economy**, **private messaging infrastructure (SimpleX/Tor)**, **AI Steward governance**, **POS merchant tools**, **multi-chain crypto bridges (TRON/TON)**, and a **Telegram bot fleet** — all running on port 8080 behind Tor onion services.

---

## 2. Project Structure

```
/home/tomas/simplex-node/
├── cmd/simplex-node/main.go    # 1889 LOC — HTTP mux, cron, bot loops, embedded apps
├── internal/                    # 23 Go packages (18 with tests)
│   ├── ai/          — Ollama client (gemma4:latest), Steward AI, moderation
│   ├── api/         — 35+ extracted HTTP handlers (treasury, economy, POS, steward, etc.)
│   ├── billing/     — Payment recording, service pricing
│   ├── bot/         — 4 Telegram bots: AskSteward, DarkPushkin, Torquemada, Inquisitor
│   ├── bridge/      — SimpleX CLI WebSocket bridge
│   ├── channels/    — Anonymous channels (CRUD skeleton)
│   ├── config/      — JSON config file loader
│   ├── dockerutil/  — Docker container helpers
│   ├── economy/     — Core: ledger, registry, auction, buyback, P2P, pack, POS, oracle
│   ├── fileutil/    — Atomic file writes (.tmp → rename)
│   ├── gateway/     — Unified notification sender (Telegram + SimpleX) [NEW Cycle 38]
│   ├── health/      — Health monitoring + alerts
│   ├── lock/        — bcrypt PIN lock + rate limiting
│   ├── middleware/  — Token bucket rate limiter, local/onion access control
│   ├── press/       — Banknote PDF templates, Ed25519 double-signature
│   ├── radio/       — Radio announcements (skeleton)
│   ├── royal/       — Royal→Sub Ed25519 protocol (register, command, heartbeat)
│   ├── status/      — Node status, reputation, tier system
│   ├── steward/     — AI Steward: constitution, monitor, analyzer, decision engine
│   ├── ton/         — ARGENTUM TON Jetton constants + swap stubs
│   ├── treasury/    — TRON USDT monitor, silver round, proof of reserve
│   ├── vault/       — 2GB E2EE file store
│   └── webrtc/      — WebRTC signaling state, ICE/TURN credentials
├── apps/                       # Flutter app stubs (write-only, no SDK)
│   ├── royal_app/              # Royal Node Control (admin)
│   ├── isle_app/               # The Isle (citizen app)
│   └── shared/                 # Shared Dart packages (api_client, models, widgets)
├── docs/                       # 10+ documents (PLAN, EVOLUTION, PRODUCTION-CYCLE, etc.)
├── docker/                     # Docker Compose: smp-server, xftp-server, tor, coturn
├── scripts/                    # send-to-inquisitor.sh, launch-node.sh
├── systemd/                    # simplex-node-dashboard.service
└── testdata/                   # Test fixtures
```

### Key file: `cmd/simplex-node/main.go` (1889 LOC)

All-in-one entrypoint with:
- **80+ HTTP routes** covering the full API surface
- **7 goroutine loops** (AI warmup, 3 bots, mining payout, POS cleanup, disk alerts, health monitor, bridge, stale nodes, steward evaluation)
- **5 cron tickers** (1h mining payout, 15min POS cleanup, 30min disk alert, 60s health check, 5min steward eval)
- **3 embedded Telegram bots** (AskSteward, DarkPushkin, Torquemada)
- **2 embedded web apps** (Mini App at `/app/`, POS Dashboard at `/pos`)
- **Graceful shutdown** (SIGTERM/SIGINT handler)
- **7 external package imports**: gorilla/websocket, golang.org/x/crypto, rsc.io/qr

---

## 3. Infrastructure (Docker — 4 Containers)

| Container | Image | Port | Purpose |
|-----------|-------|------|---------|
| `smp-server` | `simplexchat/smp-server` | 5223 | SimpleX message relay |
| `xftp-server` | `simplexchat/xftp-server` | 443 | SimpleX file transfer |
| `tor` | Custom Alpine | — | 5 hidden services (smp, xftp, dashboard, ice, auditor) |
| `coturn` | `coturn/coturn` | 3478/5349 | TURN/STUN relay for WebRTC voice/video |

All containers have health checks. Tor onions are persistent via bind-mount.

---

## 4. Complete API Surface (80+ Endpoints)

### Status & System
| Route | Method | Description |
|-------|--------|-------------|
| `/api/status` | GET | Full node status (containers, disk, uptime, vault) |
| `/api/addresses` | GET | Onion addresses + contact link |
| `/api/disk-check` | GET | Disk usage per mount |
| `/api/health` | GET | Health report with alerting |
| `/api/dashboard-onion` | GET | Dashboard hidden service URL |

### Lock System
| Route | Method | Description |
|-------|--------|-------------|
| `/api/lock-status` | GET | Lock state |
| `/api/lock` | POST | Lock the dashboard |
| `/api/unlock` | POST | Unlock with bcrypt PIN (rate-limited) |
| `/api/change-lock-code` | POST | Change PIN code |

### Vault (E2EE File Store)
| Route | Method | Description |
|-------|--------|-------------|
| `/api/vault/list` | GET | List files with sizes |
| `/api/vault/upload` | POST | Upload file (max 50MB, 2GB quota) |
| `/api/vault/download` | GET | Download file by name |
| `/api/vault/delete` | POST | Delete file |
| `/api/vault/save-note` | POST | Save text note |

### WebRTC / ICE
| Route | Method | Description |
|-------|--------|-------------|
| `/api/ice-config` | GET | TURN credentials (12h HMAC-SHA1) |
| `/api/call-signal` | GET/POST | WebRTC signaling per room |

### Economy (Silver-Backed Ledger)
| Route | Method | Description |
|-------|--------|-------------|
| `/api/economy/state` | GET | Full economy state (supply, reserve, banknotes) |
| `/api/economy/tokenomics` | GET | Tokenomics constants |
| `/api/economy/holdings` | GET | User portfolio (balance, banknotes, dividends) |
| `/api/economy/oracle` | GET | Live silver spot price |
| `/api/economy/deflate` | POST | Deflation burn mechanism |
| `/api/economy/wheel` | POST | Golden Wheel daily spin |
| `/api/economy/auto-mint` | POST | Auto-mint by treasury tier |
| `/api/economy/crafting` | POST | 5→1 banknote crafting |
| `/api/economy/reinvest` | POST | Auto-reinvest dividends |
| `/api/economy/onboarding` | GET | Onboarding status |
| `/api/economy/onboarding/welcome` | POST | Welcome banknote |
| `/api/economy/onboarding/starter` | POST | Starter pack purchase |
| `/api/economy/onboarding/guide` | POST | Guided tour completion |

### Wallet
| Route | Method | Description |
|-------|--------|-------------|
| `/api/wallet/create` | POST | Create Ed25519 keypair + mnemonic |
| `/api/wallet/balance` | GET | Liquid Taler balance + frozen ng |

### Treasury (Silver Rounds + USDT)
| Route | Method | Description |
|-------|--------|-------------|
| `/api/treasury/state` | GET | Treasury state |
| `/api/treasury/proof-of-reserve` | GET | Proof of Reserve |
| `/api/treasury/usdt-deposits` | GET | USDT deposit log |
| `/api/treasury/init-silver-round` | POST | Initiate silver round |
| `/api/treasury/claim-dividends` | POST | Claim banknote dividends |
| `/api/treasury/auto-round` | POST | Trigger auto-round |
| `/api/treasury/register-banknote` | POST | Register banknote |
| `/api/treasury/simulate-deposit` | POST | Simulate USDT deposit |

### Market / Escrow
| Route | Method | Description |
|-------|--------|-------------|
| `/api/market/list` | GET | Marketplace listings |
| `/api/market/sell` | POST | List item for sale |
| `/api/market/buy` | POST | Buy listed item |
| `/api/escrow/create` | POST | Create escrow |
| `/api/escrow/release` | POST | Release escrow |
| `/api/escrow/cancel` | POST | Cancel escrow |
| `/api/escrow/list` | GET | List escrows |
| `/api/escrow/buy` | POST | Buy via escrow |
| `/api/escrow/auto-resolve` | POST | Auto-resolve escrow |

### Auction
| Route | Method | Description |
|-------|--------|-------------|
| `/api/auction/active` | GET | Active auctions |
| `/api/auction/list` | POST | List banknote for auction |
| `/api/auction/bid` | POST | Place bid |
| `/api/auction/my` | GET | User's auction listings |

### Buyback
| Route | Method | Description |
|-------|--------|-------------|
| `/api/buyback/quote` | GET | Buyback price quote |
| `/api/buyback/sell` | POST | Sell banknote to treasury |

### Packs
| Route | Method | Description |
|-------|--------|-------------|
| `/api/pack/buy` | POST | Buy booster pack |
| `/api/pack/open` | POST | Open sealed pack |
| `/api/pack/list` | GET | User's packs |

### POS Terminal
| Route | Method | Description |
|-------|--------|-------------|
| `/api/pos` | GET/POST | POS actions (create, pay, list, stats, merchants, vouchers) |
| `/api/pos/qr` | GET | QR code PNG for invoice |
| `/pos/pay` | GET | POS payment page (HTML) |
| `/pos` | GET | POS dashboard (HTML SPA) |

### Subscription & Mining
| Route | Method | Description |
|-------|--------|-------------|
| `/api/subscription` | GET/POST | Subscription tiers |
| `/api/mining` | GET/POST | Vault mining |

### Advertising
| Route | Method | Description |
|-------|--------|-------------|
| `/api/advertising` | POST | Deflationary ad tags |

### Genesis ICO
| Route | Method | Description |
|-------|--------|-------------|
| `/api/genesis/ico` | GET/POST | Genesis ICO |
| `/api/genesis/lock` | GET/POST | Genesis lock (frozen dividends) |
| `/api/genesis/info` | GET | Genesis card info |

### Audit & P2P
| Route | Method | Description |
|-------|--------|-------------|
| `/api/auditor/grant` | POST | Grant auditor role |
| `/api/auditor/list` | GET | List auditors |
| `/api/auditor/refresh` | POST | Refresh auditor list |
| `/auditor` | GET | Auditor dashboard (HTML) |
| `/api/p2p/explore` | GET | Explore holders (auditor only) |

### AI Steward
| Route | Method | Description |
|-------|--------|-------------|
| `/api/ai/chat` | POST | Ask AI Steward (Ollama) |
| `/api/ai/explain-silver` | GET | Silver standard explanation |
| `/api/ai/suggest-treasury` | POST | Treasury action suggestion |
| `/api/ai/health` | GET | AI availability |
| `/api/ai/economy-summary` | POST | Economy AI summary |
| `/api/ai/moderation` | POST | Content moderation |
| `/api/ai/constitution` | GET | Steward constitution |
| `/api/ai/monitor` | GET | Steward metrics |

### Steward (Dynamic Governance)
| Route | Method | Description |
|-------|--------|-------------|
| `/api/steward` | GET/POST | Steward status, evaluate, enable/disable |

### Arbitration
| Route | Method | Description |
|-------|--------|-------------|
| `/api/arbitration` | POST | Dispute lifecycle (open, respond, analyze, rule, appeal) |

### Franchise
| Route | Method | Description |
|-------|--------|-------------|
| `/api/franchise/licenses` | GET/POST | License CRUD |
| `/api/franchise/earmarks` | GET/POST | Earmarked accounts |
| `/api/franchise/mint-auth` | POST | Mint authorization |
| `/api/franchise/templates` | GET/POST | National templates |
| `/api/franchise/settlements` | POST | Cross-franchise settlement |
| `/api/franchise/royalties` | POST | Royalty payments |

### Royal Node Control
| Route | Method | Description |
|-------|--------|-------------|
| `/api/royal/register` | POST | Register sub-node |
| `/api/royal/nodes` | GET | List sub-nodes |
| `/api/royal/command` | POST | Send signed command |
| `/api/royal/heartbeat` | POST | Sub-node heartbeat |
| `/api/royal/key` | GET | Royal public key |
| `/royal/control` | POST | Royal control command |
| `/royal/sync` | POST | Royal sync |

### Services & RWA
| Route | Method | Description |
|-------|--------|-------------|
| `/api/services/registry` | GET/POST | Service registry |
| `/api/services/marketplace` | GET | Service marketplace |
| `/api/rwa/register` | POST | Register RWA (silver-backed) |
| `/api/rwa/list` | GET | List RWA items |

### Billing
| Route | Method | Description |
|-------|--------|-------------|
| `/api/billing/prices` | GET | Service prices |
| `/api/billing/payments` | GET | Payment history |

### Radio & Channels
| Route | Method | Description |
|-------|--------|-------------|
| `/api/radio/list` | GET | Audio library + announcements |
| `/api/radio/play` | GET | Stream audio file |
| `/api/channels/list` | GET | List channels |
| `/api/channels/create` | POST | Create channel |
| `/api/channels/view` | GET | View channel |
| `/api/channels/access` | POST | Channel access control |
| `/api/channels/post` | POST | Post to channel |
| `/api/channels/posts` | GET | Channel posts |

### ARGENTUM TON
| Route | Method | Description |
|-------|--------|-------------|
| `/api/argentum` | GET/POST | ARGENTUM Jetton market + swap |

### Rotation
| Route | Method | Description |
|-------|--------|-------------|
| `/api/rotate` | POST | Rotate Tor onion keys |

### Role Chat
| Route | Method | Description |
|-------|--------|-------------|
| `/api/set_role_chat` | GET | Set role→chat mapping |
| `/api/send_to_role` | GET | Send message to role |

### Static & Web Apps
| Route | Method | Description |
|-------|--------|-------------|
| `/static/` | GET | Static files (QR, etc.) |
| `/` | GET | Dashboard SPA |
| `/app/` | GET | ARGENTUM Mini App |
| `/pos` | GET | POS Dashboard |
| `/pos/pay` | GET | POS Pay Page |

---

## 5. Telegram Bot Fleet (4 Bots)

| Bot | Token | Type | Purpose |
|-----|-------|------|---------|
| **@AskSteward_bot** | `<BOT_TOKEN>` | Polling | AI assistant, economy queries, inline menus |
| **@DarkPushkin_bot** | `<BOT_TOKEN>` | Polling | Creative lore generation |
| **@torquemada878_bot** | `<BOT_TOKEN>` | Polling | Admin notifications, build/deploy commands |
| **Inquisitor** | `<BOT_TOKEN>` | Script | Automated cycle reports to chat 143293811 |

All bots use native Go (`net/http`), `InlineKeyboardMarkup` menus, callback query routing.

---

## 6. Internal Service Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                       TCP :8080 (HTTP)                          │
├──────────────────────────────────────────────────────────────────┤
│  Router (main.go) → 80+ handlers                                │
│    ├── middleware.DenyIfNotLocalOrOnion                          │
│    ├── middleware.IsLocalOrOnionAccess                           │
│    └── middleware.IsStrictLocalOnly                              │
├──────────────────────────────────────────────────────────────────┤
│  Background Goroutines (10+):                                    │
│    ├── AI warm-up (once)                                         │
│    ├── 3 Telegram bots (long-poll)                               │
│    ├── Mining auto-payout (1h)                                   │
│    ├── POS cleanup (15min)                                       │
│    ├── Disk alert (30min)                                        │
│    ├── TRON USDT monitor (60s)                                  │
│    ├── Auto silver round (trigger)                              │
│    ├── Silver spot oracle (5min)                                 │
│    ├── Steward evaluation loop (5min)                           │
│    ├── Stale node check (5min)                                  │
│    ├── Health monitor (60s)                                     │
│    └── SimpleX bridge (WebSocket)                                │
├──────────────────────────────────────────────────────────────────┤
│  Economy Engine (internal/economy/):                             │
│    ├── Ed25519 Ledger (accounts, transfers, mint)                │
│    ├── Banknote Registry (5 rarities, serials, frozen ng)       │
│    ├── Auction Manager (listing, bidding, close)                │
│    ├── Buyback Manager (quote, execute, re-mint)                │
│    ├── P2P Offers                                                │
│    ├── Pack Manager (booster packs, sealed → open)              │
│    ├── POS Manager (invoices, vouchers, merchant revenue)       │
│    ├── DynamicParams (9 adjustable economy params)              │
│    ├── Onboarding (welcome, starter pack, guide)                │
│    ├── Arbitration (dispute lifecycle)                          │
│    └── Silver Oracle (live spot polling)                        │
├──────────────────────────────────────────────────────────────────┤
│  AI Steward (internal/steward/):                                 │
│    ├── Constitution (16 rules, min/max/target bounds)            │
│    ├── Monitor (60s ticker, 10+ metrics)                        │
│    ├── Analyzer (3-level deviation detection)                   │
│    └── Decision Engine (auto-adjust, notify, consensus)          │
├──────────────────────────────────────────────────────────────────┤
│  Blockchain Bridges:                                             │
│    ├── TRON (internal/treasury/) — USDT TRC20 monitor + grid    │
│    └── TON (internal/ton/) — ARGENTUM Jetton stubs + swap       │
├──────────────────────────────────────────────────────────────────┤
│  Gateway (internal/gateway/) — Unified notification:             │
│    ├── MultiSender (fans out to all backends)                   │
│    ├── TelegramSender (real Bot API)                            │
│    └── SimplexSender (stub, pending WS gateway)                 │
└──────────────────────────────────────────────────────────────────┘
```

---

## 7. Economic Engine Details

### Currency Units
- **NG** (nanogram): 1 ng = 1e-9 grams of silver digital equivalent
- **TLR** (Liquid Taler): 1 TLR = 31,103,480,000 ng = 1 troy oz of silver at spot
- **USDT**: 1 USDT ≈ 414,713,066 ng (at $75/oz silver)

### Tokenomics Constants
```
NGPerTLR               = 31,103,480,000
SilverSpotUSDperOZ     = 75.0
SilverBackingRatio     = 0.70 (70% physical-backed)
UtilityPremiumPct      = 0.30 (30% network utility)
TreasuryCommissionBPS  = 228 (2.28%)
MaxTotalFeeBPS         = 420 (4.20%)
POSCommissionBPS       = 100 (1.00%)
SwapFeeBPS             = 50  (0.50% for ARGENTUM)
```

### Issuance Split (70 oz → 100 TLR)
| Pool | % of Premium | ng per Issue |
|------|-------------|-------------|
| Investor | 70.0% of total | 2,177,243,600,000 |
| Treasury | 2.4% | 74,648,352,000 |
| Dividend Pool | 14.1% | 438,559,068,000 |
| Silver Buy | 3.0% | 93,310,440,000 |
| Auction Pool | 4.5% | 139,965,660,000 |
| Buyback Pool | 6.0% | 186,620,880,000 |

### Banknote Rarities & Dividend Weights
| Rarity | Weight | Multiplier |
|--------|--------|------------|
| Common | 1× | Base |
| Rare | 2× | 2× dividends |
| Epic | 5× | 5× dividends |
| Legendary | 10× | 10× dividends |
| Genesis | 20× | 20× dividends |

### Subscription Tiers
| Tier | Price/mo | Benefits |
|------|----------|----------|
| Colonist | Free (5 TLR) | 2.28% P2P fee |
| Citizen | 10 TLR (~$24) | 2GB vault, 0% P2P, voting |
| Aristocrat | 100 TLR (~$240) | 4× dividends, auto-bid, early access |

### Fee Structure
- **Auction listing**: 0.5% of start price (min 1M ng)
- **Auction close**: 2.28% treasury + 1.92% dividends = 4.20% max
- **POS processing**: 1.0% merchant fee
- **ARGENTUM swap**: 0.5% TON ↔ ng
- **P2P**: 2.28% (Colonist), 0% (Citizen+)

---

## 8. Security Model

| Layer | Implementation |
|-------|---------------|
| **Dashboard lock** | bcrypt PIN hash + constant-time comparison |
| **Rate limiting** | Token bucket (5/60s per IP on unlock) |
| **Access control** | Onion-only + localhost for all /api/ endpoints |
| **File writes** | Atomic: `.tmp` → `Sync()` → `rename` |
| **Signing** | Ed25519: banknotes, royal commands, arbitration |
| **Request limits** | `http.MaxBytesReader` on POST endpoints |
| **Graceful shutdown** | SIGTERM/SIGINT drain + flush |
| **Treasury** | USDT deposits via TronGrid API polling |

---

## 9. Steward AI Constitution (16 Rules)

| Rule | Metric | Target | Minor | Major | Critical |
|------|--------|--------|-------|-------|----------|
| silver_reserve_ratio | reserve / total_supply | 0.70 | ≥0.65 | ≥0.50 | <0.50 |
| treasury_balance_ng | treasury pool | 50T | ≥40T | ≥20T | <20T |
| total_supply_ng | total minted | 500T | ≥400T | ≥200T | <200T |
| active_banknotes | count | 1000 | ≥750 | ≥500 | <500 |
| active_auctions | count | 10 | ≥5 | ≥3 | <3 |
| daily_transactions | tx count | 50 | ≥30 | ≥10 | <10 |
| dividend_pool_ng | pool size | 10T | ≥5T | ≥2T | <2T |
| pos_volume_ng | volume | 1T | ≥500B | ≥100B | <100B */
| treasury_commission_bps | fee rate | 228 | 200-250 | 150-300 | <150 or >350 |
| max_total_fee_bps | cap | 420 | 380-460 | 350-500 | <300 or >550 |
| auction_fee | avg | 133 | 100-166 | 80-200 | <50 or >250 |
| account_count | registered | 100 | ≥50 | ≥20 | <10 |
| subscription_count | active | 20 | ≥10 | ≥5 | <3 */
| silver_round_frequency | rounds/day | 1 | 0.5-2 | 0.2-5 | <0.1 or >10 |
| network_health | containers ok | 4 | ≥3 | ≥2 | <2 |
| wallet_transfers_24h | count | 20 | ≥10 | ≥5 | <3 */

---

## 10. Test Coverage (18/18 Packages)

| Package | Tests | Status |
|---------|-------|--------|
| ai | 6 | ✅ |
| api | ~35 | ✅ |
| billing | 8 | ✅ |
| bot | ~15 | ✅ |
| bridge | 6 | ✅ |
| config | 8 | ✅ |
| dockerutil | 4 | ✅ |
| economy | ~20 | ✅ |
| fileutil | 6 | ✅ |
| gateway | 4 | ✅ NEW |
| health | 6 | ✅ |
| lock | 14 | ✅ |
| middleware | 5 | ✅ |
| press | 11 | ✅ |
| status | 18 | ✅ |
| steward | 12 | ✅ |
| vault | 11 | ✅ |

Empty (no tests): channels, radio, royal, ton, treasury, webrtc

---

## 11. Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/gorilla/websocket` | v1.5.3 | WebSocket bridge to SimpleX CLI |
| `golang.org/x/crypto` | v0.52.0 | Ed25519, bcrypt |
| `rsc.io/qr` | v0.2.0 | QR code generation (pure Go) |

Zero CGO. No external Telegram library. No database driver yet.

---

## 12. Configuration

File: `~/.local/share/simplex-node/simplex-node.json`
Loaded via `-config` flag. Fields: listen, data_dir, vault_quota_mb, billing_prices_ng,
island_bot_url, alert_url, tron_treasury_addr, tron_grid_api_key,
ollama_url, ollama_model, ask_steward_token, dark_pushkin_token,
torquemada_token, torquemada_chat_id.
