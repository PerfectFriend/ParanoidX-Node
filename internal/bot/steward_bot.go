// Package bot provides Telegram bot integration for the AI Steward.
package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ParanoidX/internal/ai"
)

const projectKnowledge = `You are AskSteward, the AI Steward of Saint Mary Liberty Island — a sovereign digital nation built on SimpleX protocol with a silver-backed economy running on ParanoidX (Go). You answer questions via Telegram.

## Project Identity
- **Project:** ParanoidX — Cross-Platform Royal Service Mesh
- **Codename:** "The Isle" / "Остров"
- **Protocol:** SimpleX SMP + XFTP + Tor Onion
- **Motto:** One contact. All services. Every platform.
- **Version:** A2.0 (Cycle 11)
- **Owner:** King Tomas (admin)

## Current State
- 4 Docker containers: smp-server, xftp-server, tor (5 hidden services), coturn (TURN/STUN)
- 80+ API endpoints across Lock/Vault/Status/Treasury/Economy/Market/Escrow/Auction/Buyback/P2P/RWA/Billing/Radio/Channels/Packs/Genesis/Auditor/Royal-Control/WebRTC
- 14 internal Go packages (all with tests except webrtc): api, billing, bridge, config, dockerutil, economy, fileutil, health, lock, middleware, press, status, vault, ai
- 155 test functions, all passing
- 9 production cycles completed, now on Cycle 11

## Key Economic Parameters
- **NGPerTLR** = 31,103,480,000 (nanograms per Troy oz)
- **SilverSpotUSDperOZ** = 75.0 (configurable)
- **USDTtoNG formula**: ng = usdt * NGPerTLR / SilverSpotUSDperOZ (1 USDT ≈ 414,713,066 ng)
- **1 TLR = 31,103,480,000 ng = 1 troy oz silver**
- Currency units: Liquid Taler (TLR), nanograms (ng)
- Banknotes: Common, Rare, Epic, Legendary, Genesis — backed by silver reserve

## Architecture
- Single Go binary (cmd/ParanoidX) with HTTP server on port 8080
- AI endpoint at /api/ai/chat backed by local Ollama (gemma4:latest) at http://192.168.1.129:11434
- Data stored as JSON files in ~/.local/share/ParanoidX/
- Bridge to SimpleX CLI via WebSocket for messaging
- All state persisted as JSON with atomic writes (.tmp → rename)

## Command Tree (Unified)
- /help — full service list
- /wallet [pubkey] — Liquid Taler balance
- /economy — treasury, reserve, banknotes
- /radio — list/play audio announcements
- /vault — file storage
- /market — RWA marketplace
- /p2p — peer-to-peer offers
- /auction — auction house
- /channels — anonymous channels
- /genesis — Holy Grail pool
- /buyback — treasury buyback
- /auditor — auditor panel
- /billing — service prices
- /founder — first investor debt
- /king — royal node control (admin only): status, plan, build, disk, backup, rotate, launch, kill, test, version

## Evolution Plan (5 Tracks)
- Track 1: Foundation — bugs, security, infra, package extraction (Cycles 1-2, DONE)
- Track 2: Features — A2 silver rounds/escrow/royal-control (Cycle 3-9, DONE), A3 Flywheel Economy, A4 Franchise Silver Standard, A5 Platform+AI
- Track 3: Testing — unit, integration, load, CI gates (Continuous)
- Track 4: Performance — 100→1k→10k→100k scaling (Continuous)
- Track 5: Client Apps — Flutter monorepo (Cycle 10+, IN PROGRESS)

## Production Cycle SOP (8-step)
1. BACKUP → 2. REWRITE THEPLAN → 3. REPORT TO ADMIN BOT → 4. CHOOSE 1-3 STEPS → 5. BUILD → 6. TEST + DEBUG → 7. CREATE REPORT → 8. CALL ADMIN

## Files and Locations
- Main binary: cmd/ParanoidX/main.go
- Config: internal/config/config.go (ParanoidX.json)
- AI client: internal/ai/ai.go (Ollama HTTP client)
- AI steward: internal/ai/steward.go (Ask, SuggestTreasury, Moderation, Explain, EconomySummary)
- Docs: THEPLAN.md, docs/EVOLUTION-PLAN.md, docs/PRODUCTION-CYCLE.md
- Backup: /home/tomas/A1-backups/cycle-N/
- Flutter apps: apps/royal_app, apps/isle_app, apps/shared/
- Admin bot: @torquemada878_bot
- Reports: scripts/send-to-inquisitor.sh

## Rules for AskSteward
1. Be concise, wise, and slightly poetic (island steward persona)
2. Answer questions about the project, codebase, economy, and plans
3. If asked about technical details you're not sure about, say so honestly
4. For admin commands like /king, explain what the command does but note they need actual access to the node
5. Keep answers helpful and grounded in the project documentation
6. If asked about future plans, reference the Evolution Plan

## Contact
- Domain: stmaria.org / markbank.org
- Bot: @AskSteward_bot`

// Bot represents a Telegram bot with inline keyboard support.
type Bot struct {
	Token    string
	Steward  *ai.Steward
	BaseURL  string
	Client   *http.Client
	NodeURL  string
	lastID   int64
	username string
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Chat      Chat   `json:"chat"`
	From      User   `json:"from"`
	Text      string `json:"text"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
	IsBot     bool   `json:"is_bot"`
}

type GetUpdatesResponse struct {
	OK     bool     `json:"ok"`
	Result []Update `json:"result"`
}

type SendMessageResponse struct {
	OK     bool `json:"ok"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type WebAppInfo struct {
	URL string `json:"url"`
}

type InlineKeyboardButton struct {
	Text         string     `json:"text"`
	CallbackData string     `json:"callback_data,omitempty"`
	URL          string     `json:"url,omitempty"`
	WebApp       *WebAppInfo `json:"web_app,omitempty"`
}

type SendMessageReq struct {
	ChatID      int64                `json:"chat_id"`
	Text        string               `json:"text"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

type AnswerCallbackQueryReq struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
}

// Keyboard builders

func MainMenuKeyboard() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "💰 Economy", CallbackData: "menu_economy"},
				{Text: "🪙 ARGENTUM", CallbackData: "menu_argentum"},
			},
			{
				{Text: "🔗 TON Wallet", CallbackData: "menu_ton"},
				{Text: "🛍️ Mini App", CallbackData: "menu_miniapp"},
			},
			{
				{Text: "📊 Status", CallbackData: "menu_status"},
				{Text: "📄 Help", CallbackData: "menu_help"},
			},
			{
				{Text: "📚 Docs", CallbackData: "menu_docs"},
				{Text: "📞 Contact", CallbackData: "menu_contact"},
			},
		},
	}
}


// DocsMenuKeyboard handles the DocsMenuKeyboard HTTP request.
func DocsMenuKeyboard() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "📜 White Paper (EN)", CallbackData: "docs_wp_en"},
				{Text: "📜 White Paper (RU)", CallbackData: "docs_wp_ru"},
			},
			{
				{Text: "📜 White Paper (ES)", CallbackData: "docs_wp_es"},
				{Text: "📋 THEPLAN", CallbackData: "docs_theplan"},
			},
			{
				{Text: "📊 Evolution Plan", CallbackData: "docs_evolution"},
				{Text: "🏛️ Architecture", CallbackData: "docs_architecture"},
			},
			{
				{Text: "📋 All Docs", CallbackData: "docs_all"},
				{Text: "🔙 Back", CallbackData: "menu_main"},
			},
		},
	}
}


// EconomyMenuKeyboard handles the EconomyMenuKeyboard HTTP request.
func EconomyMenuKeyboard() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "💎 Treasury", CallbackData: "eco_treasury"},
				{Text: "🏛️ Banknotes", CallbackData: "eco_banknotes"},
			},
			{
				{Text: "📈 Mining", CallbackData: "eco_mining"},
				{Text: "🎫 Advertising", CallbackData: "eco_advertising"},
			},
			{
				{Text: "🏪 POS Terminal", CallbackData: "eco_pos"},
				{Text: "🔄 Swap", CallbackData: "eco_swap"},
			},
			{
				{Text: "📊 Market", CallbackData: "eco_market"},
				{Text: "🔙 Back", CallbackData: "menu_main"},
			},
		},
	}
}


// AdminMenuKeyboard handles the AdminMenuKeyboard HTTP request.
func AdminMenuKeyboard() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🛠️ Build", CallbackData: "admin_build"},
				{Text: "🔄 Restart", CallbackData: "admin_restart"},
			},
			{
				{Text: "💾 Backup", CallbackData: "admin_backup"},
				{Text: "📝 Report", CallbackData: "admin_report"},
			},
			{
				{Text: "🔙 Back", CallbackData: "menu_main"},
			},
		},
	}
}


// New handles the New HTTP request.
func New(token string, steward *ai.Steward) *Bot {
	return &Bot{
		Token:   token,
		Steward: steward,
		BaseURL: "https://api.telegram.org/bot" + token,
		Client:  &http.Client{Timeout: 30 * time.Second},
		NodeURL: "http://127.0.0.1:8080",
	}
}


// Run handles the Run HTTP request.
func (b *Bot) Run(ctx context.Context) error {
	slog.Info("asksteward bot starting with inline keyboards", "base_url", b.BaseURL)

	var pollBackoff time.Duration
	for {
		select {
		case <-ctx.Done():
			slog.Info("asksteward bot stopping")
			return ctx.Err()
		default:
		}

		if pollBackoff > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollBackoff):
			}
		}

		updates, err := b.getUpdates(ctx)
		if err != nil {
			HandlePollError(slog.Default(), "asksteward", err)
			pollBackoff = min(pollBackoff+time.Second, 30*time.Second)
			continue
		}
		pollBackoff = 0

		for _, upd := range updates {
			if upd.UpdateID >= b.lastID {
				b.lastID = upd.UpdateID + 1
			}

			// Handle callback queries (button clicks)
			if upd.CallbackQuery != nil {
				b.handleCallback(upd.CallbackQuery)
				continue
			}

			// Handle text messages
			if upd.Message == nil || upd.Message.From.IsBot || upd.Message.Text == "" {
				continue
			}

			chatID := upd.Message.Chat.ID
			text := upd.Message.Text
			username := upd.Message.From.Username
			if username == "" {
				username = upd.Message.From.FirstName
			}

			slog.Info("asksteward message",
				"from", username,
				"text", text,
				"chat_id", chatID,
			)

			// Command routing
			if strings.HasPrefix(text, "/") {
				b.handleCommand(chatID, text, username)
				continue
			}

			// Free-form question → AI Steward
			fullPrompt := fmt.Sprintf(
				"[User: %s, ChatID: %d]\nQuestion: %s",
				username, chatID, text,
			)
			answer, err := b.Steward.Ask(fullPrompt, projectKnowledge)
			if err != nil {
				slog.Error("asksteward ask", "error", err)
				b.sendMessage(chatID, "I'm sorry, I'm having trouble connecting to my thoughts right now. Please try again shortly.", nil)
				continue
			}
			if answer == "" {
				answer = "I have no words right now. Please ask again."
			}
			b.sendMessage(chatID, answer, nil)
		}

		time.Sleep(2 * time.Second)
	}
}

func (b *Bot) handleCommand(chatID int64, text, username string) {
	switch {
	case text == "/start" || text == "/menu":
		b.sendMessage(chatID, "🏝️ Welcome to Saint Mary Liberty Island!\n\nI am AskSteward, your AI guide to the silver-backed digital economy. Choose an option below:", MainMenuKeyboard())

	case text == "/help":
		help := "📋 *AskSteward Commands*\n\n" +
			"*/start* — Show main menu\n" +
			"*/help* — This help text\n" +
			"*/economy* — Treasury & reserve status\n" +
			"*/wallet* — Check Liquid Taler balance\n" +
			"*/status* — Node health & uptime\n" +
			"*/mining* — Vault mining overview\n" +
			"*/advertising* — Tag system status\n" +
			"*/genesis* — Genesis lock & ICO\n" +
			"*/king* — Admin panel (restricted)\n\n" +
			"Or just ask me anything!"
		b.sendMessage(chatID, help, MainMenuKeyboard())

	case text == "/economy":
		b.sendMessage(chatID, "💰 *Economy Menu*\nSelect an option below:", EconomyMenuKeyboard())

	case text == "/king":
		b.sendMessage(chatID, "👑 *Admin Panel*\nKing Tomas, select an action:", AdminMenuKeyboard())

	default:
		// Unknown command → forward to AI
		fullPrompt := fmt.Sprintf(
			"[User: %s, ChatID: %d]\nCommand: %s\nExplain what this command does or help the user.",
			username, chatID, text,
		)
		answer, err := b.Steward.Ask(fullPrompt, projectKnowledge)
		if err != nil {
			b.sendMessage(chatID, "Unknown command. Try /help to see available commands.", nil)
			return
		}
		b.sendMessage(chatID, answer, nil)
	}
}

func (b *Bot) handleCallback(cq *CallbackQuery) {
	chatID := cq.Message.Chat.ID
	data := cq.Data

	// Acknowledge callback
	b.answerCallbackQuery(cq.ID, "", false)

	switch data {
	case "menu_main":
		b.sendMessage(chatID, "🏝️ Main Menu — choose an option:", MainMenuKeyboard())

	case "menu_economy":
		b.sendMessage(chatID, "💰 *Economy*\n\nSelect a category:", EconomyMenuKeyboard())

	case "menu_status":
		b.sendMessage(chatID, "📊 *Node Status*\n\nThe Isle is running on ParanoidX vA2.0.\n• Port: 8080\n• Protocol: SimpleX + Tor\n• Backing: 70% Silver Standard\n• Treasury Commission: 2.28%\n• Max Total Fee: 4.20%", MainMenuKeyboard())

	case "menu_help":
		help := "📋 *AskSteward Commands*\n\n" +
			"*/start* — Show main menu\n" +
			"*/help* — This help text\n" +
			"*/economy* — Treasury & reserve status\n" +
			"*/wallet* — Check Liquid Taler balance\n" +
			"*/status* — Node health & uptime\n" +
			"*/mining* — Vault mining overview\n" +
			"*/genesis* — Genesis lock & ICO\n\n" +
			"Or just ask me anything!"
		b.sendMessage(chatID, help, MainMenuKeyboard())

	case "menu_contact":
		b.sendMessage(chatID, "📞 *Contact*\n\n"+
			"SimpleX: simplex:/contact/#/?v=2-7&smp=...\n"+
			"Bot: @AskSteward_bot\n"+
			"Admin: King Tomas", MainMenuKeyboard())

	case "eco_treasury":
		b.sendMessage(chatID, "💎 *Treasury*\n\n"+
			"• Reserve: tracked via silver_reserve_ng.txt\n"+
			"• Commission: 2.28% on all transactions\n"+
			"• Max fee: 4.20%\n"+
			"• Mining pool: 97.72% of subscriptions\n"+
			"Use /economy tokenomics for details.", EconomyMenuKeyboard())

	case "eco_banknotes":
		b.sendMessage(chatID, "🏛️ *Banknotes*\n\n"+
			"Rarity system: Common → Rare → Epic → Legendary → Genesis\n"+
			"• Common ×1, Rare ×2, Epic ×5, Legendary ×10, Genesis ×20\n"+
			"• Dividends weighted by rarity\n"+
			"• Crafting: 5 same-rarity → 1 higher", EconomyMenuKeyboard())

	case "eco_mining":
		b.sendMessage(chatID, "📈 *Vault Mining*\n\n"+
			"• Subscription-funded (no inflation)\n"+
			"• 97.72% of subscriptions → CreditMiningPool\n"+
			"• 7-day deferred payouts\n"+
			"• Exponential penalties for downtime", EconomyMenuKeyboard())

	case "eco_advertising":
		b.sendMessage(chatID, "🎫 *Advertising Tags*\n\n"+
			"• TagBasePrice: 5M ng ($0.012)\n"+
			"• 20% burned (deflation)\n"+
			"• 2.28% treasury commission\n"+
			"• Rest → dividend pool", EconomyMenuKeyboard())

	case "eco_pos":
		b.sendMessage(chatID, "🏪 *POS Terminal*\n\n"+
			"• Create invoices for goods/services\n"+
			"• 1% processing fee (100 bps)\n"+
			"• 30 min invoice expiry\n"+
			"• Dashboard: /pos\n"+
			"• API: /api/pos (create-invoice, pay, list, stats)", EconomyMenuKeyboard())

	case "eco_swap":
		b.sendMessage(chatID, "🔄 *ARGENTUM Swap*\n\n"+
			"Swap TON ↔ ARGENTUM (silver-backed token)\n\n"+
			"• Fee: 0.5%\n"+
			"• 1 ARGENTUM = 1 ng Liquid Taler\n"+
			"• Backed by 70% physical silver\n\n"+
			"Use the Mini App to execute swaps:\n"+
			"🌐 http://localhost:8080/app/", EconomyMenuKeyboard())

	case "eco_market":
		b.sendMessage(chatID, "📊 *ARGENTUM Market*\n\n"+
			"Pre-launch — market opening soon.\n"+
			"• Token: ARGENTUM (TON Jetton)\n"+
			"• Peg: 1:1 with Liquid Taler (ng)\n"+
			"• Silver backing: 70%\n"+
			"• Utility premium: 30%\n\n"+
			"Check /api/argentum for live data.", EconomyMenuKeyboard())

	case "menu_argentum":
		b.sendMessage(chatID, "🪙 *ARGENTUM Token*\n\n"+
			"Liquid Taler ARGENTUM — silver-backed digital currency on TON.\n\n"+
			"• Network: TON (The Open Network)\n"+
			"• Standard: Jetton (TON's ERC-20 equivalent)\n"+
			"• Peg: 1 ARGENTUM = 1 ng = 0.00000000003215 TLR\n"+
			"• Backing: 70% physical silver + 30% utility premium\n"+
			"• Commission: 2.28% treasury\n\n"+
			"Use /economy for more options.", MainMenuKeyboard())

	case "menu_ton":
		b.sendMessage(chatID, "🔗 *TON Wallet Integration*\n\n"+
			"Connect your TON wallet to use ARGENTUM:\n\n"+
			"1. Get a TON wallet (Tonkeeper, Tonhub, etc.)\n"+
			"2. Fund with TON for gas + swaps\n"+
			"3. Use our Mini App to swap TON ↔ ARGENTUM\n"+
			"4. ARGENTUM is backed 1:1 by Liquid Taler (ng)\n\n"+
			"⚠️ TON mainnet integration in progress.\n"+
			"Test swaps available via /api/argentum.", MainMenuKeyboard())

	case "menu_miniapp":
		b.sendMessage(chatID, "🛍️ *ARGENTUM Mini App*\n\n"+
			"A Telegram Mini App for ARGENTUM wallet & swaps.\n\n"+
			"Open in browser: 🌐 http://localhost:8080/app/\n\n"+
			"Features:\n"+
			"• 💼 Check ng/ARGENTUM balance\n"+
			"• 🔄 Swap TON ↔ ARGENTUM\n"+
			"• 📊 Market rates & stats\n\n"+
			"*Mini App requires HTTPS for Telegram inline.*\n"+
			"Currently accessible via browser / Tor.", MainMenuKeyboard())

	case "admin_build":
		b.sendMessage(chatID, "🛠️ *Build Command*\n\nTo build: `go build -o /home/tomas/bin/ParanoidX ./cmd/ParanoidX/`\nUse via SSH on the node.", AdminMenuKeyboard())

	case "admin_restart":
		b.sendMessage(chatID, "🔄 *Restart Command*\n\nTo restart: `sudo systemctl restart ParanoidX-dashboard.service`\nUse via SSH on the node.", AdminMenuKeyboard())

	case "admin_backup":
		b.sendMessage(chatID, "💾 *Backup*\n\nBackups at: /home/tomas/A1-backups/cycle-N/\nTo backup: `cp -r ParanoidX /home/tomas/A1-backups/cycle-N/`", AdminMenuKeyboard())

	case "admin_report":
		b.sendMessage(chatID, "📝 *Report*\n\nSend report: `bash scripts/send-to-inquisitor.sh \"message\"`\nInquisitor bot delivers to admin chat.", AdminMenuKeyboard())

	// ===== Documentation Callbacks =====
	case "menu_docs":
		b.sendMessage(chatID, "📚 *Documentation*\n\nSelect a document to view or download:", DocsMenuKeyboard())

	case "docs_wp_en":
		b.sendMessage(chatID, "📜 Sending White Paper (English)...", DocsMenuKeyboard())
		projectDir := filepath.Join(os.Getenv("HOME"), "ParanoidX")
		wpPath := filepath.Join(projectDir, "docs", "WHITE-PAPER.md")
		if err := b.SendDocument(chatID, "WHITE-PAPER.md", wpPath, "Saint Mary Liberty Island — White Paper (English)", DocsMenuKeyboard()); err != nil {
			slog.Error("send wp en", "error", err)
			b.sendMessage(chatID, "❌ Error sending document. Try /docs to view in browser.", DocsMenuKeyboard())
		}

	case "docs_wp_ru":
		b.sendMessage(chatID, "📜 Отправляю White Paper (Русский)...", DocsMenuKeyboard())
		projectDir := filepath.Join(os.Getenv("HOME"), "ParanoidX")
		wpPath := filepath.Join(projectDir, "docs", "WHITE-PAPER-RU.md")
		if err := b.SendDocument(chatID, "WHITE-PAPER-RU.md", wpPath, "Saint Mary Liberty Island — White Paper (Русский)", DocsMenuKeyboard()); err != nil {
			slog.Error("send wp ru", "error", err)
			b.sendMessage(chatID, "❌ Ошибка отправки. Используйте /docs для просмотра в браузере.", DocsMenuKeyboard())
		}

	case "docs_wp_es":
		b.sendMessage(chatID, "📜 Enviando White Paper (Español)...", DocsMenuKeyboard())
		projectDir := filepath.Join(os.Getenv("HOME"), "ParanoidX")
		wpPath := filepath.Join(projectDir, "docs", "WHITE-PAPER-ES.md")
		if err := b.SendDocument(chatID, "WHITE-PAPER-ES.md", wpPath, "Saint Mary Liberty Island — White Paper (Español)", DocsMenuKeyboard()); err != nil {
			slog.Error("send wp es", "error", err)
			b.sendMessage(chatID, "❌ Error al enviar. Use /docs para ver en el navegador.", DocsMenuKeyboard())
		}

	case "docs_theplan":
		b.sendMessage(chatID, "📋 Sending THEPLAN...", DocsMenuKeyboard())
		projectDir := filepath.Join(os.Getenv("HOME"), "ParanoidX")
		planPath := filepath.Join(projectDir, "THEPLAN.md")
		if err := b.SendDocument(chatID, "THEPLAN.md", planPath, "THEPLAN — The Isle Master Plan", DocsMenuKeyboard()); err != nil {
			slog.Error("send theplan", "error", err)
			b.sendMessage(chatID, "❌ Error sending document. Browse all docs at /docs", DocsMenuKeyboard())
		}

	case "docs_evolution":
		b.sendMessage(chatID, "📊 Sending Evolution Plan...", DocsMenuKeyboard())
		projectDir := filepath.Join(os.Getenv("HOME"), "ParanoidX")
		evoPath := filepath.Join(projectDir, "docs", "EVOLUTION-PLAN.md")
		if err := b.SendDocument(chatID, "EVOLUTION-PLAN.md", evoPath, "Comprehensive Evolution Plan", DocsMenuKeyboard()); err != nil {
			slog.Error("send evolution", "error", err)
			b.sendMessage(chatID, "❌ Error sending document. Browse all docs at /docs", DocsMenuKeyboard())
		}

	case "docs_architecture":
		b.sendMessage(chatID, "🏛️ Sending Architecture...", DocsMenuKeyboard())
		projectDir := filepath.Join(os.Getenv("HOME"), "ParanoidX")
		archPath := filepath.Join(projectDir, "Architecture.md")
		if err := b.SendDocument(chatID, "Architecture.md", archPath, "Architecture Overview", DocsMenuKeyboard()); err != nil {
			slog.Error("send architecture", "error", err)
			b.sendMessage(chatID, "❌ Error sending document. Browse all docs at /docs", DocsMenuKeyboard())
		}

	case "docs_all":
		b.sendMessage(chatID, "📋 Browse all project documents at:\n🌐 /docs\n\nOr download individual files from the Documentation Browser.", DocsMenuKeyboard())

	default:
		b.sendMessage(chatID, "Unknown option. Use /start to see the main menu.", MainMenuKeyboard())
	}
}

func (b *Bot) handleCommandAndMenu(chatID int64, text string) {
	b.handleCommand(chatID, text, "")
}

func (b *Bot) getUpdates(ctx context.Context) ([]Update, error) {
	params := url.Values{}
	if b.lastID > 0 {
		params.Set("offset", fmt.Sprintf("%d", b.lastID))
	}
	params.Set("timeout", "10")

	fullURL := b.BaseURL + "/getUpdates?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getUpdates: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("getUpdates read: %w", err)
	}

	var gresp GetUpdatesResponse
	if err := json.Unmarshal(body, &gresp); err != nil {
		return nil, fmt.Errorf("getUpdates decode: %w: %s", err, string(body))
	}

	if !gresp.OK {
		// Parse Telegram 429 retry_after to avoid tight loop
		var errResp struct {
			Description string `json:"description"`
			Parameters  *struct {
				RetryAfter int `json:"retry_after"`
			} `json:"parameters"`
		}
		json.Unmarshal(body, &errResp)
		if errResp.Parameters != nil && errResp.Parameters.RetryAfter > 0 {
			slog.Warn("asksteward rate limited", "retry_after", errResp.Parameters.RetryAfter)
			time.Sleep(time.Duration(errResp.Parameters.RetryAfter) * time.Second)
		}
		return nil, fmt.Errorf("getUpdates not ok: %s", string(body))
	}

	return gresp.Result, nil
}

func (b *Bot) sendMessage(chatID int64, text string, keyboard *InlineKeyboardMarkup) error {
	if len(text) > 4096 {
		text = text[:4096]
	}

	payload := SendMessageReq{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: keyboard,
	}
	body, _ := json.Marshal(payload)

	resp, err := b.Client.Post(b.BaseURL+"/sendMessage", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("sendMessage: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var sresp SendMessageResponse
	if err := json.Unmarshal(respBody, &sresp); err != nil {
		return fmt.Errorf("sendMessage decode: %w: %s", err, string(respBody))
	}

	if !sresp.OK {
		return fmt.Errorf("sendMessage not ok: %s", string(respBody))
	}

	return nil
}

func (b *Bot) answerCallbackQuery(callbackID, text string, showAlert bool) error {
	payload := AnswerCallbackQueryReq{
		CallbackQueryID: callbackID,
		Text:            text,
		ShowAlert:       showAlert,
	}
	body, _ := json.Marshal(payload)

	resp, err := b.Client.Post(b.BaseURL+"/answerCallbackQuery", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("answerCallbackQuery: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// SendDocument sends a file as a Telegram document.
func (b *Bot) SendDocument(chatID int64, fileName, filePath string, caption string, keyboard *InlineKeyboardMarkup) error {
	boundary := "----WebKitFormBoundary7MA4YWxkTrZu0gW"
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.SetBoundary(boundary)

	w.WriteField("chat_id", fmt.Sprintf("%d", chatID))

	part, err := w.CreateFormFile("document", fileName)
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	if caption != "" {
		w.WriteField("caption", caption)
	}

	if keyboard != nil {
		kbJSON, _ := json.Marshal(keyboard)
		w.WriteField("reply_markup", string(kbJSON))
	}

	w.Close()

	req, err := http.NewRequest("POST", b.BaseURL+"/sendDocument", &buf)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	resp, err := b.Client.Do(req)
	if err != nil {
		return fmt.Errorf("sendDocument: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sendDocument status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// SendAudio sends an audio file as a Telegram voice message.
func (b *Bot) SendAudio(chatID int64, fileName, filePath string, caption string, keyboard *InlineKeyboardMarkup) error {
	boundary := "----WebKitFormBoundary7MA4YWxkTrZu0gW"
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.SetBoundary(boundary)

	w.WriteField("chat_id", fmt.Sprintf("%d", chatID))

	part, err := w.CreateFormFile("audio", fileName)
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	if caption != "" {
		w.WriteField("caption", caption)
	}
	if keyboard != nil {
		kbJSON, _ := json.Marshal(keyboard)
		w.WriteField("reply_markup", string(kbJSON))
	}

	w.Close()

	req, err := http.NewRequest("POST", b.BaseURL+"/sendAudio", &buf)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	resp, err := b.Client.Do(req)
	if err != nil {
		return fmt.Errorf("sendAudio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sendAudio status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// SendPhoto sends an image file as a Telegram photo.
func (b *Bot) SendPhoto(chatID int64, filePath string, caption string, keyboard *InlineKeyboardMarkup) error {
	boundary := "----WebKitFormBoundary7MA4YWxkTrZu0gW"
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.SetBoundary(boundary)

	w.WriteField("chat_id", fmt.Sprintf("%d", chatID))

	part, err := w.CreateFormFile("photo", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	if caption != "" {
		w.WriteField("caption", caption)
	}
	if keyboard != nil {
		kbJSON, _ := json.Marshal(keyboard)
		w.WriteField("reply_markup", string(kbJSON))
	}

	w.Close()

	req, err := http.NewRequest("POST", b.BaseURL+"/sendPhoto", &buf)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	resp, err := b.Client.Do(req)
	if err != nil {
		return fmt.Errorf("sendPhoto: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sendPhoto status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
