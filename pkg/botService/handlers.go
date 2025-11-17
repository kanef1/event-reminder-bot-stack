package botService

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	botManager "event-reminder-bot/pkg/event-reminder-bot"
	"event-reminder-bot/pkg/model"
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
	snoozeCommand = "/snooze"
)

type BotService struct {
	b            *bot.Bot
	bm           *botManager.BotManager
	rm           *reminder.ReminderManager
	snoozeStates map[int64]int
	mu           sync.RWMutex
}

func NewBotService(b *bot.Bot, bm *botManager.BotManager, rm *reminder.ReminderManager) *BotService {
	return &BotService{
		b:            b,
		bm:           bm,
		rm:           rm,
		snoozeStates: make(map[int64]int),
		mu:           sync.RWMutex{},
	}
}

func (bs *BotService) RegisterHandlers() {
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, botManager.StartHandler)
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, botManager.HelpHandler)
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, "/add", bot.MatchTypePrefix, bs.AddHandler)
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, "/list", bot.MatchTypeExact, bs.listHandler)
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, "/delete", bot.MatchTypePrefix, bs.deleteHandler)
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
		if err != nil {
			return
		}
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

		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   text,
		})
		return
	}

	reminderEvent := model.NewReminderEventFromModel(event)

	bs.rm.ScheduleReminder(ctx, reminderEvent)

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "✅ Событие добавлено!",
	})
	if err != nil {
		return
	}
	if err != nil {
		return
	}
}

func (bs *BotService) snoozeHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	args := strings.TrimSpace(strings.TrimPrefix(update.Message.Text, "/snooze"))

	parts := strings.Fields(args)
	if len(parts) < 3 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❗ Формат: /snooze <номер> <YYYY-MM-DD HH:MM>\nНапример: /snooze 1 2025-11-10 22:35\n\nНомер можно посмотреть командой /list",
		})
		if err != nil {
			return
		}
		return
	}

	orderNumber, err := strconv.Atoi(parts[0])
	if err != nil {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❗ Номер должен быть числом",
		})
		if err != nil {
			return
		}
		return
	}

	events, err := bs.bm.GetUserEvents(ctx, update.Message.Chat.ID)
	if err != nil {
		log.Printf("Ошибка загрузки событий: %v", err)
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка при загрузке событий",
		})
		if err != nil {
			return
		}
		return
	}

	if orderNumber < 1 || orderNumber > len(events) {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❗ Некорректный номер. У вас %d событий. Используйте /list для просмотра", len(events)),
		})
		if err != nil {
			return
		}
		return
	}

	eventID := events[orderNumber-1].ID

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Println("Ошибка загрузки часового пояса:", err)
		loc = time.Local
	}

	dateTimeStr := parts[1] + " " + parts[2]
	newTime, err := time.ParseInLocation("2006-01-02 15:04", dateTimeStr, loc)
	if err != nil {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❗ Недопустимый формат даты. Используйте: YYYY-MM-DD HH:MM",
		})
		if err != nil {
			return
		}
		return
	}

	err = bs.bm.SnoozeEvent(ctx, eventID, update.Message.Chat.ID, newTime)
	if err != nil {
		responseText := processError(err)
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   responseText})
		return

	}

	bs.rm.CancelReminder(eventID)

	event, err := bs.bm.GetEventByID(ctx, eventID)
	if err == nil && event != nil {
		reminderEvent := reminder.Event{
			ID:         event.ID,
			OriginalID: event.OriginalID,
			ChatID:     event.ChatID,
			Text:       event.Text,
			DateTime:   event.DateTime,
		}
		bs.rm.ScheduleReminder(ctx, reminderEvent)
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   fmt.Sprintf("✅ Событие №%d перенесено на %s", orderNumber, newTime.Format("2006-01-02 15:04")),
	})
	if err != nil {
		return
	}
}

func processError(err error) string {
	var text string
	switch {
	case errors.Is(err, botManager.ErrNotFound):
		text = "❌ Событие не найдено"
	case errors.Is(err, botManager.ErrAccessDenied):
		text = "❌ У вас нет доступа к этому событию"
	case errors.Is(err, botManager.ErrInactive):
		text = "❌ Событие неактивно"
	case errors.Is(err, botManager.ErrPastDate):
		text = "❗ Нельзя перенести событие в прошлое"
	default:
		text = fmt.Sprintf("❌ Ошибка: %v", err)
	}
	return text
}

func (bs *BotService) textHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	text := strings.TrimSpace(update.Message.Text)

	bs.mu.RLock()
	eventID, exists := bs.snoozeStates[chatID]
	bs.mu.RUnlock()

	if !exists {
		botManager.DefaultHandler(ctx, b, update)
		return
	}

	bs.mu.Lock()
	delete(bs.snoozeStates, chatID)
	bs.mu.Unlock()

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Println("Ошибка загрузки часового пояса:", err)
		loc = time.Local
	}

	newTime, err := time.ParseInLocation("2006-01-02 15:04", text, loc)
	if err != nil {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❗ Недопустимый формат даты. Используйте: YYYY-MM-DD HH:MM\nНапример: 2025-11-10 22:35",
		})
		if err != nil {
			return
		}
		return
	}

	err = bs.bm.SnoozeEvent(ctx, eventID, chatID, newTime)
	if err != nil {
		responseText := processError(err)
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   responseText,
		})
		if err != nil {
			return
		}
	}

	bs.rm.CancelReminder(eventID)

	event, err := bs.bm.GetEventByID(ctx, eventID)
	if err == nil && event != nil {
		reminderEvent := reminder.Event{
			ID:         event.ID,
			OriginalID: event.OriginalID,
			ChatID:     event.ChatID,
			Text:       event.Text,
			DateTime:   event.DateTime,
		}
		bs.rm.ScheduleReminder(ctx, reminderEvent)
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   fmt.Sprintf("✅ Событие перенесено на %s", newTime.Format("2006-01-02 15:04")),
	})
	if err != nil {
		return
	}
}

func (bs *BotService) handleDoneCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	data := update.CallbackQuery.Data
	chatID := update.CallbackQuery.Message.Message.Chat.ID

	parts := strings.Split(data, "_")
	if len(parts) == 2 {
		eventID, err := strconv.Atoi(parts[1])
		if err != nil {
			_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: update.CallbackQuery.ID,
				Text:            "❌ Ошибка обработки",
			})
			if err != nil {
				return
			}
			return
		}

		err = bs.bm.DeleteEventByID(ctx, eventID)
		if err != nil {
			_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: update.CallbackQuery.ID,
				Text:            "❌ Ошибка при удалении события",
				ShowAlert:       true,
			})
			if err != nil {
				return
			}
			return
		}

		bs.rm.CancelReminder(eventID)

		_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "✅ Событие выполнено",
		})
		if err != nil {
			return
		}

		_, err = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
			ChatID:      chatID,
			MessageID:   update.CallbackQuery.Message.Message.ID,
			ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}},
		})
		if err != nil {
			return
		}

		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: update.CallbackQuery.Message.Message.ID,
			Text:      update.CallbackQuery.Message.Message.Text + "\n\n✅ Выполнено",
		})
		if err != nil {
			return
		}
	}
}

func (bs *BotService) handleSnoozeCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	data := update.CallbackQuery.Data
	chatID := update.CallbackQuery.Message.Message.Chat.ID

	parts := strings.Split(data, "_")

	if len(parts) == 3 && parts[1] != "custom" {
		eventID, err := strconv.Atoi(parts[1])
		if err != nil {
			_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: update.CallbackQuery.ID,
				Text:            "❌ Ошибка обработки",
			})
			if err != nil {
				return
			}
			return
		}

		minutes, err := strconv.Atoi(parts[2])
		if err != nil {
			_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: update.CallbackQuery.ID,
				Text:            "❌ Ошибка обработки",
			})
			if err != nil {
				return
			}
			return
		}

		newTime := time.Now().Add(time.Duration(minutes) * time.Minute)

		err = bs.bm.SnoozeEvent(ctx, eventID, chatID, newTime)
		if err != nil {
			response := processError(err)
			_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: update.CallbackQuery.ID,
				Text:            response,
				ShowAlert:       true,
			})
			if err != nil {
				return
			}
			return
		}

		bs.rm.CancelReminder(eventID)

		event, err := bs.bm.GetEventByID(ctx, eventID)
		if err == nil && event != nil {
			reminderEvent := reminder.Event{
				ID:         event.ID,
				OriginalID: event.OriginalID,
				ChatID:     event.ChatID,
				Text:       event.Text,
				DateTime:   event.DateTime,
			}
			bs.rm.ScheduleReminder(ctx, reminderEvent)
		}

		_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            fmt.Sprintf("✅ Отложено на %d мин", minutes),
		})
		if err != nil {
			return
		}

		_, err = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
			ChatID:      chatID,
			MessageID:   update.CallbackQuery.Message.Message.ID,
			ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}},
		})
		if err != nil {
			return
		}

		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: update.CallbackQuery.Message.Message.ID,
			Text:      update.CallbackQuery.Message.Message.Text + fmt.Sprintf("\n\n⏱️ Отложено на %d мин", minutes),
		})
		if err != nil {
			return
		}

	} else if len(parts) == 3 && parts[1] == "custom" {
		eventID, err := strconv.Atoi(parts[2])
		if err != nil {
			_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: update.CallbackQuery.ID,
				Text:            "❌ Ошибка обработки",
			})
			if err != nil {
				return
			}
			return
		}

		bs.mu.Lock()
		bs.snoozeStates[chatID] = eventID
		bs.mu.Unlock()

		_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
		})
		if err != nil {
			return
		}

		_, err = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
			ChatID:      chatID,
			MessageID:   update.CallbackQuery.Message.Message.ID,
			ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}},
		})
		if err != nil {
			return
		}

		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: update.CallbackQuery.Message.Message.ID,
			Text:      update.CallbackQuery.Message.Message.Text + "\n\n⏳ Ожидание ввода времени...",
		})
		if err != nil {
			return
		}

		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "📅 Введите новую дату и время в формате:\nYYYY-MM-DD HH:MM\n\nНапример: 2025-12-31 23:59",
		})
		if err != nil {
			return
		}
	}
}
