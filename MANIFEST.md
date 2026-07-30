# ParanoidX — Manifest

**Project:** ParanoidX (ex. simplex-node)
**Type:** Go HTTP Server + Docker Infrastructure
**Role:** Core backbone of Saint Mary Liberty Island
**Status:** Production (v px-node-C41-C60)
**Location:** `/home/tomas/ParanoidX/`

---

## Purpose

Sovereign Go HTTP server providing:
- Silver-backed digital economy API (TLR/NG)
- SimpleX Chat bridge (port 17225) over Tor
- Radio scheduler & AI content generator
- ParanoidX multi-layer routing (VLESS + VMess + Tor)
- Docker orchestration (5 services)
- Telegram bots: Royal, Poet, Inquisitor, Admin

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    ParanoidX Core (Go)                       │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────────────┐  │
│  │ Economy  │ │  Chat    │ │  Radio   │ │  ParanoidX     │  │
│  │ Treasury │ │  Bridge  │ │ Scheduler│ │  Routing     │  │
│  └──────────┘ └──────────┘ └──────────┘ └────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
        ┌──────────┐   ┌──────────────┐ ┌──────────┐
        │  Tor     │   │   Docker     │ │  SimpleX │
        │  (9050)  │   │  (5 svcs)    │ │  Chat    │
        └──────────┘   └──────────────┘ └──────────┘
```

---

## Key Endpoints (283+)

| Category | Endpoints |
|----------|-----------|
| **Wallet** | `/api/wallet/*` (create, balance, send, mint, redeem, banknotes, dividends) |
| **Market** | `/api/market/*` (listings, create, buy, escrow, orders) |
| **Treasury** | `/api/treasury/*` (state, reserve, oracle, deflation, auto-mint, dividends) |
| **Governance** | `/api/gov/*` (constitution, proposals, vote, delegate) |
| **Chat** | `/api/chat/*` (history, send, edit, delete, pin, react, broadcast, search) |
| **Radio** | `/api/radio/*` (schedule, content, AI generation) |
| **ParanoidX** | `/api/paranoidx/*` (status, config, build, test, VPN) |
| **Admin** | `/api/admin/*` (info, metrics, docker, service, backup, config) |

---

## Economic Parameters

```
NGPerTLR = 31,103,480,000 ng
SilverSpotUSDperOZ = 75.0
SilverBackingRatio = 0.70
UtilityPremiumPct = 0.30
Issuance (70 oz → 100 TLR): Investor 70% | Treasury 4.2% | DividendPool 12.9% | SilverBuy 3% | AuctionPool 6.6% | Buyback 3.3%
```

---

## Deployment

```bash
# Build
go build ./cmd/ParanoidX/

# Run (systemd)
sudo systemctl start ParanoidX-dashboard.service

# Health
curl http://127.0.0.1:8080/api/health
```

---

## Data Persistence

- **Config:** `~/.local/share/simplex-node/simplex-node.json` (unchanged)
- **Chat History:** `~/.local/share/simplex-node/chat_history.json`
- **Radio:** `~/.local/share/simplex-node/radio/`
- **Docker Volumes:** `/home/tomas/ParanoidX/docker/`