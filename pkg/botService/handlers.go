package botService

import (
	"context"
	"fmt"
	"strings"

	botManager "event-reminder-bot/pkg/event-reminder-bot"
	"event-reminder-bot/pkg/reminder"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	startCommand  = "/start"
	helpCommand   = "/help"
	addCommand    = "/add"
	listCommand   = "/list"
	deleteCommand = "/delete"
)

type BotService struct {
	b  *bot.Bot
	bm *botManager.BotManager
	rm *reminder.ReminderManager
}

func NewBotService(b *bot.Bot, bm *botManager.BotManager, rm *reminder.ReminderManager) *BotService {
	return &BotService{b: b, bm: bm, rm: rm}
}

func (bs *BotService) RegisterHandlers() {
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, startCommand, bot.MatchTypeExact, botManager.StartHandler)
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, helpCommand, bot.MatchTypeExact, botManager.HelpHandler)
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, addCommand, bot.MatchTypePrefix, bs.AddHandler)
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, listCommand, bot.MatchTypeExact, bs.listHandler)
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, deleteCommand, bot.MatchTypePrefix, bs.deleteHandler)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "period:", bot.MatchTypePrefix, bs.callbackHandler)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "weekday:", bot.MatchTypePrefix, bs.callbackHandler)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "weekdays_done:", bot.MatchTypePrefix, bs.callbackHandler)
}

func (bs *BotService) callbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	bs.bm.HandleCallback(ctx, b, update)
}

func (bs *BotService) deleteHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	botManager.DeleteHandler(ctx, b, update, bs.bm)
}

func (bs *BotService) listHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	botManager.ListHandler(ctx, b, update, bs.bm)
}

func (bs BotService) AddHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	args := strings.TrimSpace(strings.TrimPrefix(update.Message.Text, "/add"))
	parts := strings.SplitN(args, " ", 3)
	if len(parts) < 3 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❗ Формат: /add 2025-08-06 15:00 Текст",
		})
		return
	}

	event, err := bs.bm.AddEvent(ctx, update.Message.Chat.ID, parts)
	if err != nil {
		var text string
		switch err.Error() {
		case "invalid_format":
			text = "❗ Недопустимый формат даты (используйте YYYY-MM-DD HH:MM)"
		case "past_date":
			text = "❗ Недопустимый формат даты (событие должно быть в будущем)"
		default:
			text = fmt.Sprintf("Ошибка: %v", err)
		}

		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   text,
		})
		return
	}

	reminderEvent := reminder.Event{
		ID:         event.ID,
		OriginalID: event.OriginalID,
		ChatID:     event.ChatID,
		Text:       event.Text,
		DateTime:   event.DateTime,
	}

	bs.rm.ScheduleReminder(ctx, reminderEvent)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "✅ Событие добавлено!",
	})
}
