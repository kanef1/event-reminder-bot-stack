package event_reminder_bot

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"event-reminder-bot/pkg/db"
	"event-reminder-bot/pkg/model"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func DefaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Нет такой команды, используйте /help чтобы посмотреть доступные команды команд",
	})
	if err != nil {
		return
	}
}

func StartHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text: "Добрый день, данный бот предназначен для простого планирования.\n" +
			"Список умений:\n" +
			"Добавить событие: /add 2025-08-08 21:05 <Текст>\n" +
			"Список событий: /list \n" +
			"Удалить событие: /delete id\n" +
			"Перенести событие: /snooze <id> <YYYY-MM-DD HH:MM>\n" +
			"Список команд: /help",
	})
	if err != nil {
		return
	}
}

func HelpHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text: "Список умений:\n" +
			"Добавить событие: /add 2025-08-08 21:05 <Текст>\n" +
			"Список событий: /list\n" +
			"Удалить событие: /delete id\n" +
			"Перенести событие: /snooze <id> <YYYY-MM-DD HH:MM>\n" +
			"Список команд: /help",
	})
	if err != nil {
		return
	}
}

func DeleteHandler(ctx context.Context, b *bot.Bot, update *models.Update, bm *BotManager) {
	args := strings.TrimSpace(strings.TrimPrefix(update.Message.Text, "/delete"))
	if args == "" {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❗ Укажите ID события, например: /delete 123",
		})
		if err != nil {
			return
		}
		return
	}

	id, err := strconv.Atoi(args)
	if err != nil {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❗ ID должен быть числом",
		})
		if err != nil {
			return
		}
		return
	}

	err = bm.DeleteEventByID(ctx, id)
	if err != nil {
		log.Printf("Ошибка удаления события: %v", err)
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка при удалении события",
		})
		if err != nil {
			return
		}
		return
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "✅ Событие удалено!",
	})
	if err != nil {
		return
	}
}

func ListHandler(ctx context.Context, b *bot.Bot, update *models.Update, bm *BotManager) {
	events, err := bm.GetUserEvents(ctx, update.Message.Chat.ID)
	if err != nil {
		log.Printf("Ошибка загрузки событий: %v", err)
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка при загрузке событий",
		})
		if err != nil {
			return
		}
		return
	}

	if len(events) == 0 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "🔍 Нет событий",
		})
		if err != nil {
			return
		}
		return
	}

	var msg strings.Builder
	msg.WriteString("📅 Список событий (от ближайших):\n\n")
	for i, e := range events {
		msg.WriteString(fmt.Sprintf(
			"%d. %s — %s\n",
			i+1,
			e.Text,
			e.DateTime.Format("2006-01-02 15:04"),
		))
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   msg.String(),
	})
	if err != nil {
		return
	}
}

type BotManager struct {
	b          *bot.Bot
	eventsRepo db.EventsRepo
}

func NewBotManager(b *bot.Bot, eventsRepo db.EventsRepo) *BotManager {
	return &BotManager{b: b, eventsRepo: eventsRepo}
}

func (bm BotManager) SendReminder(ctx context.Context, chatID int64, text string, eventID int) {
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "⏱️ 5 мин", CallbackData: fmt.Sprintf("snooze_%d_5", eventID)},
				{Text: "⏱️ 10 мин", CallbackData: fmt.Sprintf("snooze_%d_10", eventID)},
			},
			{
				{Text: "📅 Выбрать время", CallbackData: fmt.Sprintf("snooze_custom_%d", eventID)},
			},
			{
				{Text: "✅ Выполнено", CallbackData: fmt.Sprintf("done_%d", eventID)},
			},
		},
	}

	_, err := bm.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        "🔔 Напоминание: " + text,
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return
	}
}

func (bm BotManager) AddEvent(ctx context.Context, chatId int64, parts []string) (*model.Event, error) {
	datePart := parts[0]
	timePart := parts[1]
	text := parts[2]

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Println("Ошибка загрузки часового пояса:", err)
		loc = time.Local
	}

	dt, err := time.ParseInLocation("2006-01-02 15:04", datePart+" "+timePart, loc)
	if err != nil {
		return nil, fmt.Errorf("invalid_format")
	}

	if dt.Before(time.Now()) {
		return nil, fmt.Errorf("past_date")
	}

	event := &db.Event{
		UserTgID: chatId,
		Message:  text,
		SendAt:   dt,
		StatusID: db.StatusEnabled,
	}

	addedEvent, err := bm.eventsRepo.AddEvent(ctx, event)
	if err != nil {
		log.Printf("Ошибка сохранения события: %v", err)
		return nil, err
	}

	return &model.Event{
		ID:         addedEvent.ID,
		OriginalID: addedEvent.ID,
		ChatID:     addedEvent.UserTgID,
		Text:       addedEvent.Message,
		DateTime:   addedEvent.SendAt,
	}, nil
}

func (bm BotManager) DeleteEventByID(ctx context.Context, id int) error {
	event, err := bm.eventsRepo.EventByID(ctx, id)
	if err != nil {
		return err
	}

	if event == nil {
		return fmt.Errorf("event not found")
	}

	deleted, err := bm.eventsRepo.DeleteEvent(ctx, id)
	if err != nil {
		return err
	}

	if !deleted {
		return fmt.Errorf("event not found")
	}

	return nil
}

func (bm BotManager) GetUserEvents(ctx context.Context, chatID int64) ([]model.Event, error) {
	statusId := db.StatusEnabled
	search := &db.EventSearch{
		UserTgID: &chatID,
		StatusID: &statusId,
	}
	dbEvents, err := bm.eventsRepo.EventsByFilters(ctx, search, db.PagerNoLimit)
	if err != nil {
		return nil, err
	}

	sort.Slice(dbEvents, func(i, j int) bool {
		return dbEvents[i].SendAt.Before(dbEvents[j].SendAt)
	})

	events := make([]model.Event, len(dbEvents))
	for i, dbEvent := range dbEvents {
		events[i] = model.Event{
			ID:         dbEvent.ID,
			OriginalID: dbEvent.ID,
			ChatID:     dbEvent.UserTgID,
			Text:       dbEvent.Message,
			DateTime:   dbEvent.SendAt,
		}
	}

	return events, nil
}

func (bm BotManager) GetEventByID(ctx context.Context, id int) (*model.Event, error) {
	dbEvent, err := bm.eventsRepo.EventByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if dbEvent == nil {
		return nil, nil
	}

	return &model.Event{
		ID:         dbEvent.ID,
		OriginalID: dbEvent.ID,
		ChatID:     dbEvent.UserTgID,
		Text:       dbEvent.Message,
		DateTime:   dbEvent.SendAt,
	}, nil
}

func (bm BotManager) SnoozeEvent(ctx context.Context, eventID int, userTgID int64, newTime time.Time) error {
	event, err := bm.eventsRepo.EventByID(ctx, eventID)
	if err != nil {
		return err
	}

	if event == nil {
		return fmt.Errorf("event not found")
	}

	if event.UserTgID != userTgID {
		return fmt.Errorf("access denied")
	}

	if event.StatusID != db.StatusEnabled {
		return fmt.Errorf("event not active")
	}

	if newTime.Before(time.Now()) {
		return fmt.Errorf("past_date")
	}

	event.SendAt = newTime
	_, err = bm.eventsRepo.UpdateEvent(ctx, event, db.WithColumns(db.Columns.Event.SendAt))
	if err != nil {
		return err
	}

	return nil
}
