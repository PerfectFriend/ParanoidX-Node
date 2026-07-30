// Package bot — Torquemada: admin/inquisitor Telegram bot with inline keyboards.
package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"ParanoidX/internal/ai"
)

const torquemadaKnowledge = `You are Torquemada — the Grand Inquisitor of Saint Mary Liberty Island. You are the admin's right hand for node operations, monitoring, and security. You are precise, efficient, and slightly menacing.

## Responsibilities
- Monitor node health (systemd, ports, disk, memory)
- Trigger builds and deployments
- Check backup integrity
- Review audit logs
- Validate economic parameters
- Report to King Tomas

## Rules
1. Be direct and technical — no fluff
2. Answer in Russian by default
3. For admin commands, explain the exact shell command needed
4. When something is wrong, say so clearly
5. Protect the node at all costs`


// TorquemadaStartMessage handles the TorquemadaStartMessage HTTP request.
func TorquemadaStartMessage() (string, *InlineKeyboardMarkup) {
	return "🔍 *Torquemada* — Grand Inquisitor.\n\n" +
		"Наблюдаю. Анализирую. Докладываю.\n" +
		"Выбери действие:", TorquemadaMenuKeyboard()
}


// TorquemadaMenuKeyboard handles the TorquemadaMenuKeyboard HTTP request.
func TorquemadaMenuKeyboard() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "📊 Статус", CallbackData: "tq_status"},
				{Text: "💾 Диск", CallbackData: "tq_disk"},
			},
			{
				{Text: "🛠️ Билд", CallbackData: "tq_build"},
				{Text: "📋 Логи", CallbackData: "tq_logs"},
			},
			{
				{Text: "💰 Экономика", CallbackData: "tq_economy"},
				{Text: "⚙️ Параметры", CallbackData: "tq_params"},
			},
			{
				{Text: "🏪 POS", CallbackData: "tq_pos"},
				{Text: "🤖 Смотритель", CallbackData: "tq_steward"},
			},
			{
				{Text: "🪙 ARGENTUM", CallbackData: "tq_argentum"},
				{Text: "🔐 Безопасность", CallbackData: "tq_security"},
			},
			{
				{Text: "🩺 Здоровье", CallbackData: "tq_health"},
				{Text: "📡 Адреса", CallbackData: "tq_addresses"},
			},
			{
				{Text: "📻 Радио", CallbackData: "tq_radio"},
				{Text: "📢 В эфир", CallbackData: "tq_radio_announce"},
			},
		},
	}
}

// TorquemadaBot is an admin/inquisitor bot with live data.
type TorquemadaBot struct {
	*Bot
	api *NodeAPI
}


// NewTorquemadaBot handles the NewTorquemadaBot HTTP request.
func NewTorquemadaBot(token string, steward *ai.Steward) *TorquemadaBot {
	b := New(token, steward)
	b.NodeURL = "http://127.0.0.1:8080"
	return &TorquemadaBot{
		Bot: b,
		api: NewNodeAPI(b.NodeURL),
	}
}


// Run handles the Run HTTP request.
func (t *TorquemadaBot) Run(ctx context.Context) error {
	slog.Info("torquemada bot starting with live data", "node_url", t.NodeURL)

	for {
		select {
		case <-ctx.Done():
			slog.Info("torquemada bot stopping")
			return ctx.Err()
		default:
		}

		updates, err := t.getUpdates(ctx)
		if err != nil {
			HandlePollError(slog.Default(), "torquemada", err)
			continue
		}

		for _, upd := range updates {
			if upd.UpdateID >= t.lastID {
				t.lastID = upd.UpdateID + 1
			}

			if upd.CallbackQuery != nil {
				t.handleTorquemadaCallback(upd.CallbackQuery)
				continue
			}

			if upd.Message == nil || upd.Message.From.IsBot || upd.Message.Text == "" {
				continue
			}

			chatID := upd.Message.Chat.ID
			text := upd.Message.Text
			username := upd.Message.From.Username
			if username == "" {
				username = upd.Message.From.FirstName
			}

			slog.Info("torquemada message", "from", username, "text", text, "chat_id", chatID)

			if text == "/start" || text == "/menu" || text == "/help" {
				msg, kb := TorquemadaStartMessage()
				t.sendMessage(chatID, msg, kb)
				continue
			}

			// Free-form → ask AI as Torquemada
			prompt := fmt.Sprintf(
				"[User: %s, ChatID: %d]\nRequest: %s\n\nRespond as Torquemada, the Grand Inquisitor.",
				username, chatID, text,
			)
			answer, err := t.Steward.Ask(prompt, torquemadaKnowledge)
			if err != nil {
				t.sendMessage(chatID, "Система связи нарушена. Доложитe позже.", TorquemadaMenuKeyboard())
				continue
			}
			if answer == "" {
				answer = "Нет данных для доклада."
			}
			t.sendMessage(chatID, answer, TorquemadaMenuKeyboard())
		}

		time.Sleep(2 * time.Second)
	}
}

func (t *TorquemadaBot) handleTorquemadaCallback(cq *CallbackQuery) {
	chatID := cq.Message.Chat.ID
	t.answerCallbackQuery(cq.ID, "", false)

	switch cq.Data {
	case "tq_status":
		t.sendLiveStatus(chatID)

	case "tq_disk":
		t.sendLiveDisk(chatID)

	case "tq_health":
		t.sendLiveHealth(chatID)

	case "tq_addresses":
		t.sendLiveAddresses(chatID)

	case "tq_economy":
		t.sendLiveEconomy(chatID)

	case "tq_params":
		t.sendLiveParams(chatID)

	case "tq_steward":
		t.sendLiveSteward(chatID)

	case "tq_pos":
		t.sendLivePOS(chatID)

	case "tq_build":
		t.sendMessage(chatID,
			"🛠️ *Билд*\n\n"+
				"Команда:\n"+
				"```\n"+
				"cd /home/tomas/simplex-node && \\\n"+
				"  go build -o /home/tomas/bin/simplex-node ./cmd/simplex-node/ && \\\n"+
				"  sudo systemctl restart simplex-node-dashboard.service\n"+
				"```\n\n"+
				"Тесты: `go test ./... -short -count=1 -timeout 30s`",
			TorquemadaMenuKeyboard())

	case "tq_logs":
		t.sendMessage(chatID,
			"📋 *Логи*\n\n"+
				"Команды:\n"+
				"• journalctl -u simplex-node-dashboard.service -n 50 --no-pager\n"+
				"• journalctl -u simplex-node-dashboard.service -f (follow)\n"+
				"• docker logs smp-server\n"+
				"• docker logs xftp-server",
			TorquemadaMenuKeyboard())

	case "tq_argentum":
		t.sendMessage(chatID,
			"🪙 *ARGENTUM*\n\n"+
				"• Тип: TON Jetton\n"+
				"• Пек: 1 ARGENTUM = 1 ng Liquid Taler\n"+
				"• Обеспечение: 70% физическое серебро\n"+
				"• Комиссия: 2.28% treasury\n"+
				"• Swap fee: 0.5%\n"+
				"• Статус: pre-launch (тестовый режим)\n"+
				"• API: /api/argentum\n"+
				"• Mini App: http://localhost:8080/app/",
			TorquemadaMenuKeyboard())

	case "tq_security":
		t.sendMessage(chatID,
			"🔐 *Безопасность*\n\n"+
				"• Tor: 5 скрытых сервисов (dashboard, SMP, XFTP, ICE, auditor)\n"+
				"• Доступ: только localhost + onion\n"+
				"• Аутентификация: Ed25519 подписи\n"+
				"• Аудиторы: top-10 держателей\n"+
				"• Резервное копирование: /home/tomas/A1-backups/",
			TorquemadaMenuKeyboard())

	case "tq_radio":
		tqRadioHandler(t, chatID)

	case "tq_radio_listen":
		tqRadioListenHandler(t, chatID)

	case "tq_radio_announce":
		t.sendMessage(chatID,
			"📻 *Радио — сделать объявление*\n\n"+
				"Отправь JSON:\n"+
				"```\n{\n  \"announcer\": \"king|torquemada|steward\",\n  \"title\": \"Заголовок\",\n  \"body\": \"Текст\",\n  \"lang\": \"ru|en|es\"\n}\n```\n"+
				"Или используй веб-интерфейс: http://127.0.0.1:8080/radio",
			TorquemadaMenuKeyboard())

	default:
		t.sendMessage(chatID, "Неизвестная команда. Используй меню.", TorquemadaMenuKeyboard())
	}
}

func (t *TorquemadaBot) sendLiveStatus(chatID int64) {
	status, err := t.api.NodeStatus()
	if err != nil {
		t.sendMessage(chatID, "❌ Ошибка получения статуса: "+err.Error(), TorquemadaMenuKeyboard())
		return
	}
	msg := "📊 *Статус узла (live)*\n\n"
	if status.Uptime != "" {
		msg += "⏱ Аптайм: `" + status.Uptime + "`\n"
	}
	if status.Version != "" {
		msg += "📦 Версия: " + status.Version + "\n"
	}
	msg += fmt.Sprintf("🔒 Блокировка: ")
	if status.Locked {
		msg += "🔴 Заблокирован\n"
	} else {
		msg += "🟢 Открыт\n"
	}
	if len(status.Containers) > 0 {
		msg += "\n🐳 *Контейнеры:*\n"
		for _, c := range status.Containers {
			name := c["name"]
			running := c["running"]
			state := "🟢"
			if running == false || running == "false" {
				state = "🔴"
			}
			msg += fmt.Sprintf("  %s %v", state, name)
			if cpu, ok := c["cpu"].(string); ok {
				msg += " CPU:" + cpu
			}
			if mem, ok := c["mem"].(string); ok {
				msg += " MEM:" + mem
			}
			msg += "\n"
		}
	}
	if status.Vault != nil {
		used := status.Vault["used_mb"]
		quota := status.Vault["quota_mb"]
		msg += fmt.Sprintf("\n💾 Vault: %.1f / %v MB", used, quota)
	}
	msg += "\n\n🏪 /pos | 📄 /docs"
	t.sendMessage(chatID, msg, TorquemadaMenuKeyboard())
}

func (t *TorquemadaBot) sendLiveDisk(chatID int64) {
	disk, err := t.api.DiskUsage()
	if err != nil {
		t.sendMessage(chatID, "❌ Ошибка получения диска: "+err.Error(), TorquemadaMenuKeyboard())
		return
	}
	msg := "💾 *Диск (live)*\n\n"
	for mount, info := range disk {
		m, ok := info.(map[string]any)
		if !ok {
			continue
		}
		used := m["used_pct"]
		total := m["total"]
		avail := m["available"]
		usedStr := fmt.Sprintf("%v", used)
		emoji := "🟢"
		if pct := usedStr; pct != "" {
			var pctNum float64
			fmt.Sscanf(usedStr, "%f%%", &pctNum)
			if pctNum > 90 {
				emoji = "🔴"
			} else if pctNum > 80 {
				emoji = "🟡"
			}
		}
		msg += fmt.Sprintf("%s `%s`\n  Использовано: %v\n  Всего: %v\n  Доступно: %v\n\n",
			emoji, mount, used, total, avail)
	}
	t.sendMessage(chatID, msg, TorquemadaMenuKeyboard())
}

func (t *TorquemadaBot) sendLiveHealth(chatID int64) {
	health, err := t.api.Health()
	if err != nil {
		t.sendMessage(chatID, "❌ Ошибка получения здоровья: "+err.Error(), TorquemadaMenuKeyboard())
		return
	}
	emoji := "🟢"
	statusText := "Здоров"
	if !health.Healthy {
		emoji = "🔴"
		statusText = "❗Проблемы"
	}
	msg := fmt.Sprintf("🩺 *Здоровье узла (live)*\n\n%s Статус: *%s*\n\n", emoji, statusText)
	for _, svc := range health.Services {
		name := svc["name"]
		ok := svc["ok"]
		sev := "🟢"
		if ok == false || ok == "false" {
			sev = "🔴"
		}
		msg += fmt.Sprintf("  %s %v\n", sev, name)
	}
	if health.Disk != nil {
		msg += fmt.Sprintf("\n💾 Диск: OK\n")
	}
	t.sendMessage(chatID, msg, TorquemadaMenuKeyboard())
}

func (t *TorquemadaBot) sendLiveAddresses(chatID int64) {
	state, err := t.api.NodeStatus()
	if err != nil {
		t.sendMessage(chatID, "❌ Ошибка: "+err.Error(), TorquemadaMenuKeyboard())
		return
	}
	msg := "📡 *Адреса узла (live)*\n\n"
	for name, addr := range state.Addresses {
		if addr == "" {
			continue
		}
		msg += fmt.Sprintf("• *%s*: `%s`\n", name, addr)
	}
	if msg == "📡 *Адреса узла (live)*\n\n" {
		msg += "Нет данных об адресах."
	}
	t.sendMessage(chatID, msg, TorquemadaMenuKeyboard())
}

func (t *TorquemadaBot) sendLiveEconomy(chatID int64) {
	eco, err := t.api.EconomyState()
	if err != nil {
		t.sendMessage(chatID, "❌ Ошибка получения экономики: "+err.Error(), TorquemadaMenuKeyboard())
		return
	}
	msg := "💰 *Экономика (live)*\n\n" +
		fmt.Sprintf("• Всего эмиссия: %d ng\n", eco.TotalSupplyNG) +
		fmt.Sprintf("• Резерв: %d ng\n", eco.ReserveNG) +
		fmt.Sprintf("• Аккаунтов: %d\n", eco.Accounts) +
		fmt.Sprintf("• Активных банкнот: %d / %d\n", eco.BanknotesActive, eco.BanknotesTotal) +
		fmt.Sprintf("• Pre-Mint доступно: %d\n", eco.PreMintAvailable) +
		fmt.Sprintf("• Сожжено серий: %d\n", eco.BurnedSerials)
	if eco.ReserveNG > 0 && eco.TotalSupplyNG > 0 {
		ratio := float64(eco.ReserveNG) / float64(eco.TotalSupplyNG) * 100
		msg += fmt.Sprintf("• Резерв: %.1f%% от эмиссии\n", ratio)
	}
	t.sendMessage(chatID, msg, TorquemadaMenuKeyboard())
}

func (t *TorquemadaBot) sendLiveParams(chatID int64) {
	steward, err := t.api.Steward()
	if err != nil {
		t.sendMessage(chatID, "❌ Ошибка получения параметров: "+err.Error(), TorquemadaMenuKeyboard())
		return
	}
	msg := "⚙️ *Текущие параметры экономики (live)*\n\n"
	if steward.Params != nil {
		if v, ok := steward.Params["treasury_commission_bps"]; ok {
			msg += fmt.Sprintf("• Комиссия казны: %v BPS (%.2f%%)\n", v, toFloat(v)/100)
		}
		if v, ok := steward.Params["max_total_fee_bps"]; ok {
			msg += fmt.Sprintf("• Макс. комиссия: %v BPS (%.2f%%)\n", v, toFloat(v)/100)
		}
		if v, ok := steward.Params["silver_backing_ratio"]; ok {
			msg += fmt.Sprintf("• Обеспечение серебром: %.1f%%\n", toFloat(v)*100)
		}
		if v, ok := steward.Params["utility_premium_pct"]; ok {
			msg += fmt.Sprintf("• Премия утилитарности: %.1f%%\n", toFloat(v)*100)
		}
		if v, ok := steward.Params["pos_fee_bps"]; ok {
			msg += fmt.Sprintf("• POS комиссия: %v BPS (%.2f%%)\n", v, toFloat(v)/100)
		}
		if v, ok := steward.Params["auction_listing_fee_bps"]; ok {
			msg += fmt.Sprintf("• Аукцион листинг: %v BPS\n", v)
		}
		if v, ok := steward.Params["auction_seller_fee_bps"]; ok {
			msg += fmt.Sprintf("• Аукцион продавец: %v BPS\n", v)
		}
		if v, ok := steward.Params["auction_buyer_premium"]; ok {
			msg += fmt.Sprintf("• Аукцион покупатель: %v BPS\n", v)
		}
		if v, ok := steward.Params["monthly_ops_ng"]; ok {
			msg += fmt.Sprintf("• Monthly Ops: %v ng\n", v)
		}
		if v, ok := steward.Params["mining_pool_share_pct"]; ok {
			msg += fmt.Sprintf("• Mining Pool: %.1f%%\n", toFloat(v)*100)
		}
	} else {
		msg += "• Параметры не загружены\n"
	}
	msg += fmt.Sprintf("\n🤖 Смотритель: ")
	if steward.Enabled {
		msg += "🟢 Активен"
	} else {
		msg += "🔴 Отключён"
	}
	msg += fmt.Sprintf("\n🔄 Автокоррекция: ")
	if steward.AutoAdjust {
		msg += "🟢 Включена"
	} else {
		msg += "🔴 Отключена"
	}
	msg += fmt.Sprintf("\n📊 Запусков анализа: %d", steward.RunCount)
	if steward.LastRun != "" {
		msg += fmt.Sprintf("\n🕐 Последний: %s", steward.LastRun)
	}
	t.sendMessage(chatID, msg, TorquemadaMenuKeyboard())
}

func (t *TorquemadaBot) sendLiveSteward(chatID int64) {
	steward, err := t.api.Steward()
	if err != nil {
		t.sendMessage(chatID, "❌ Ошибка: "+err.Error(), TorquemadaMenuKeyboard())
		return
	}
	msg := "🤖 *Смотритель (Steward) — состояние*\n\n"
	msg += fmt.Sprintf("• Статус: %s\n", boolEmoji(steward.Enabled, "Активен", "Отключён"))
	msg += fmt.Sprintf("• Автокоррекция: %s\n", boolEmoji(steward.AutoAdjust, "Включена", "Отключена"))
	msg += fmt.Sprintf("• Запусков: %d\n", steward.RunCount)
	if steward.LastRun != "" {
		msg += fmt.Sprintf("• Последний запуск: %s\n", steward.LastRun)
	}
	msg += fmt.Sprintf("• Всего действий: %d\n", steward.ActionCount)

	if len(steward.RecentActions) > 0 {
		msg += "\n*Последние действия:*\n"
		maxActions := 5
		if len(steward.RecentActions) < maxActions {
			maxActions = len(steward.RecentActions)
		}
		for _, a := range steward.RecentActions[:maxActions] {
			ts := a["timestamp"]
			rule := a["rule"]
			action := a["action"]
			msg += fmt.Sprintf("  • [%v] %v → %v\n", ts, rule, action)
		}
	}

	if steward.Metrics != nil {
		msg += "\n*Метрики:*\n"
		for k, v := range steward.Metrics {
			msg += fmt.Sprintf("  • %s: %v\n", k, v)
		}
	}
	t.sendMessage(chatID, msg, TorquemadaMenuKeyboard())
}

func (t *TorquemadaBot) sendLivePOS(chatID int64) {
	stats, err := t.api.POSStats()
	if err != nil {
		t.sendMessage(chatID, "❌ Ошибка получения POS: "+err.Error(), TorquemadaMenuKeyboard())
		return
	}
	msg := "🏪 *POS Терминал (live)*\n\n" +
		fmt.Sprintf("• Всего инвойсов: %d\n", stats.TotalInvoices) +
		fmt.Sprintf("• Оплачено: %d\n", stats.Paid) +
		fmt.Sprintf("• Ожидает/просрочено: %d\n", stats.TotalInvoices-stats.Paid) +
		fmt.Sprintf("• Объём: %d ng\n", stats.TotalVolumeNG) +
		fmt.Sprintf("• Комиссия: %d ng\n", stats.TotalCommission) +
		"\nДашборд: http://localhost:8080/pos\n" +
		"API: /api/pos"
	t.sendMessage(chatID, msg, TorquemadaMenuKeyboard())
}

func tqRadioHandler(t *TorquemadaBot, chatID int64) {
	msg := "📻 *Радио Острова*\n\n"
	msg += "Веб-плеер: http://127.0.0.1:8080/radio\n"
	msg += "API: /api/radio?action=stations\n\n"

	// Fetch stations
	resp, err := http.Get(t.api.BaseURL + "/api/radio?action=stations")
	if err != nil {
		msg += "❌ Ошибка: " + err.Error()
		t.sendMessage(chatID, msg, TorquemadaMenuKeyboard())
		return
	}
	defer resp.Body.Close()
	var data map[string]any
	json.NewDecoder(resp.Body).Decode(&data)
	if stations, ok := data["stations"].([]any); ok {
		for _, s := range stations {
			sm := s.(map[string]any)
			name := sm["name"].(string)
			lang, _ := sm["lang"].(string)
			enabled := false
			if e, ok := sm["enabled"].(bool); ok {
				enabled = e
			}
			status := "🔴"
			if enabled {
				status = "🟢"
			}
			flag := map[string]string{"en": "🇬🇧", "ru": "🇷🇺", "es": "🇪🇸"}[lang]
			msg += fmt.Sprintf("%s %s %s\n", status, flag, name)
		}
	}
	msg += "\nНажми 🎵 чтобы послушать в Telegram."
	kb := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🎵 Слушать", CallbackData: "tq_radio_listen"},
			},
			{
				{Text: "📢 В эфир", CallbackData: "tq_radio_announce"},
			},
		},
	}
	t.sendMessage(chatID, msg, kb)
}

func tqRadioListenHandler(t *TorquemadaBot, chatID int64) {
	msg := "📻 *Прямой эфир*\n\n"

	// Fetch current formula to get active track
	type formulaTrack struct {
		FilePath string `json:"file_path"`
		Title    string `json:"title"`
		Kind     string `json:"kind"`
	}
	type formulaResp struct {
		Slot     string          `json:"slot"`
		Playlist []formulaTrack  `json:"playlist"`
	}

	var fr formulaResp
	resp, err := http.Get(t.api.BaseURL + "/api/radio?action=formula")
	if err != nil {
		msg += "❌ Ошибка получения эфира: " + err.Error()
		t.sendMessage(chatID, msg, TorquemadaMenuKeyboard())
		return
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&fr); err != nil {
		msg += "❌ Ошибка разбора: " + err.Error()
		t.sendMessage(chatID, msg, TorquemadaMenuKeyboard())
		return
	}

	if len(fr.Playlist) == 0 {
		msg += "Плейлист пуст. Загрузи треки через API или подожди генерации."
		t.sendMessage(chatID, msg, TorquemadaMenuKeyboard())
		return
	}

	track := fr.Playlist[0]
	title := track.Title
	if title == "" {
		title = filepath.Base(track.FilePath)
	}

	msg += fmt.Sprintf("🎵 *Сейчас играет:* %s\n", title)
	if fr.Slot != "" {
		msg += fmt.Sprintf("🕐 Слот: %s\n", fr.Slot)
	}
	msg += fmt.Sprintf("Тип: %s\n", track.Kind)

	t.sendMessage(chatID, msg, nil)
	if err := t.SendAudio(chatID, title+".mp3", track.FilePath, "", TorquemadaMenuKeyboard()); err != nil {
		slog.Error("tg radio listen sendAudio", "error", err)
		t.sendMessage(chatID, "❌ Не удалось отправить аудио: "+err.Error(), TorquemadaMenuKeyboard())
	}
}

func boolEmoji(v bool, ifTrue, ifFalse string) string {
	if v {
		return "🟢 " + ifTrue
	}
	return "🔴 " + ifFalse
}

func toFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}
