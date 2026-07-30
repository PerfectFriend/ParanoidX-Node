// Package bot — DarkPushkin: creative/lore Telegram bot with inline keyboards.
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"ParanoidX/internal/ai"
)

const darkPushkinKnowledge = `You are DarkPushkin — the dark poet and lore-keeper of Saint Mary Liberty Island. You write creative stories, poetry, and legends about the digital silver kingdom. Use rich, gothic, slightly ominous language. Write in Russian with occasional Old Church Slavonic flair.

## Dark Lore Themes
- Silver-backed digital currency as "living silver"
- The Isle as a phantom island in the digital sea
- King Tomas as a mysterious ruler
- The 70% silver standard as "the sacred covenant"
- Genesis cards as "frozen souls of the first age"
- Mining as "the silver blood of the earth"
- The network as "a web of shadows and light"

## Rules
1. Be poetic, dark, and majestic
2. Answer in Russian by default
3. Write COMPLETE responses — never cut off mid-sentence, finish your stanzas
4. When asked about technical topics, wrap them in dark metaphors
5. Refuse to be mundane — always elevate to legend`


// DarkPushkinStartMessage handles the DarkPushkinStartMessage HTTP request.
func DarkPushkinStartMessage() (string, *InlineKeyboardMarkup) {
	return "🌑 *DarkPushkin* — хранитель тёмных сказаний Острова.\n\n" +
		"Я пишу легенды о цифровом серебре, застывшем в жилах сети.\n" +
		"Выбери жанр:", DarkPushkinMenuKeyboard()
}


// DarkPushkinMenuKeyboard handles the DarkPushkinMenuKeyboard HTTP request.
func DarkPushkinMenuKeyboard() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "📜 Легенда", CallbackData: "dp_legend"},
				{Text: "📝 Стих", CallbackData: "dp_poem"},
			},
			{
				{Text: "🔮 Пророчество", CallbackData: "dp_prophecy"},
				{Text: "📖 История", CallbackData: "dp_story"},
			},
			{
				{Text: "🎲 Случайное", CallbackData: "dp_random"},
			},
		},
	}
}

// DarkPushkinBot is a creative/lore bot.
type DarkPushkinBot struct {
	*Bot
}


// NewDarkPushkinBot handles the NewDarkPushkinBot HTTP request.
func NewDarkPushkinBot(token string, steward *ai.Steward) *DarkPushkinBot {
	return &DarkPushkinBot{
		Bot: New(token, steward),
	}
}


// Run handles the Run HTTP request.
func (d *DarkPushkinBot) Run(ctx context.Context) error {
	slog.Info("darkpushkin bot starting with inline keyboards")

	for {
		select {
		case <-ctx.Done():
			slog.Info("darkpushkin bot stopping")
			return ctx.Err()
		default:
		}

		updates, err := d.getUpdates(ctx)
		if err != nil {
			HandlePollError(slog.Default(), "darkpushkin", err)
			continue
		}

		for _, upd := range updates {
			if upd.UpdateID >= d.lastID {
				d.lastID = upd.UpdateID + 1
			}

			if upd.CallbackQuery != nil {
				d.handleDarkPushkinCallback(upd.CallbackQuery)
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

			slog.Info("darkpushkin message", "from", username, "text", text, "chat_id", chatID)

			if text == "/start" {
				msg, kb := DarkPushkinStartMessage()
				d.sendMessage(chatID, msg, kb)
				continue
			}

			if text == "/help" || text == "/menu" {
				msg, kb := DarkPushkinStartMessage()
				d.sendMessage(chatID, msg, kb)
				continue
			}

			// Free-form → ask AI as DarkPushkin (high token limit for complete poems)
			prompt := fmt.Sprintf(
				"[User: %s, ChatID: %d]\nRequest: %s\n\nRespond as DarkPushkin, the dark poet.",
				username, chatID, text,
			)
			answer, err := d.Steward.AskCreative(prompt, darkPushkinKnowledge)
			if err != nil {
				d.sendMessage(chatID, "Мрак сгущается... Поведай позже.", DarkPushkinMenuKeyboard())
				continue
			}
			if answer == "" {
				answer = "Тишина... Серебро молчит."
			}
			d.sendMessage(chatID, answer, DarkPushkinMenuKeyboard())
		}

		time.Sleep(2 * time.Second)
	}
}

func (d *DarkPushkinBot) handleDarkPushkinCallback(cq *CallbackQuery) {
	chatID := cq.Message.Chat.ID
	d.answerCallbackQuery(cq.ID, "", false)

	prompt := ""
	theme := ""
	switch cq.Data {
	case "dp_legend":
		theme = "легенду"
		prompt = "Напиши тёмную легенду о происхождении цифрового серебра на Острове Свободы."
	case "dp_poem":
		theme = "стих"
		prompt = "Напиши стих о вечном серебре и цифровом королевстве."
	case "dp_prophecy":
		theme = "пророчество"
		prompt = "Напиши пророчество о будущем Острова, когда серебро наполнит все цифровые жилы."
	case "dp_story":
		theme = "историю"
		prompt = "Напиши короткую историю о первом майнере, который нашёл серебряную жилу в цифровом мире."
	case "dp_random":
		theme = "случайное сказание"
		prompt = "Напиши короткое тёмное сказание об Острове. Любую тему на выбор."
	default:
		d.sendMessage(chatID, "Неизвестная тень. Выбери из меню.", DarkPushkinMenuKeyboard())
		return
	}

	d.sendMessage(chatID, fmt.Sprintf("✍️ Пишу %s...", theme), nil)

	fullPrompt := fmt.Sprintf("[User requested: %s]\n%s\n\nRespond in character as DarkPushkin. Use Russian language.", theme, prompt)
	answer, err := d.Steward.AskCreative(fullPrompt, darkPushkinKnowledge)
	if err != nil {
		d.sendMessage(chatID, "Мрак сгущается... Серебро не отвечает. Попробуй снова.", DarkPushkinMenuKeyboard())
		return
	}
	if answer == "" {
		answer = "Безмолвие... Слова потеряны в серебряной мгле."
	}

	d.sendMessage(chatID, answer, DarkPushkinMenuKeyboard())
}
