package event_reminder_bot

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"event-reminder-bot/pkg/db"
	"event-reminder-bot/pkg/model"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/vmkteam/embedlog"
)

const MaxPeriodic = 100

const (
	Monday    = "1"
	Tuesday   = "2"
	Wednesday = "3"
	Thursday  = "4"
	Friday    = "5"
	Saturday  = "6"
	Sunday    = "7"
)

type WeekDay struct {
	ID   string
	Name string
}

var weekDays = []WeekDay{
	{ID: Monday, Name: "Понедельник"},
	{ID: Tuesday, Name: "Вторник"},
	{ID: Wednesday, Name: "Среда"},
	{ID: Thursday, Name: "Четверг"},
	{ID: Friday, Name: "Пятница"},
	{ID: Saturday, Name: "Суббота"},
	{ID: Sunday, Name: "Воскресенье"},
}

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

func (bm BotManager) DeleteHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	args := strings.TrimSpace(strings.TrimPrefix(update.Message.Text, "/delete"))
	if args == "" {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❗ Укажите ID события, например: /delete 123",
		})
		bm.onError(err)
		return
	}

	index, err := strconv.Atoi(args)
	if err != nil || index < 1 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❗ Номер события должен быть положительным числом",
		})
		bm.onError(err)
		return
	}

	events, err := bm.GetUserEvents(ctx, update.Message.Chat.ID)
	if err != nil {
		bm.Errorf("Ошибка загрузки событий: %v", err)
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка при загрузке событий",
		})
		bm.onError(err)
		return
	}

	if len(events) == 0 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "🔍 У вас нет событий для удаления",
		})
		bm.onError(err)
		return
	}

	if index > len(events) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❗ Нет события с номером %d", index),
		})
		bm.onError(err)
		return
	}

	eventToDelete := events[index-1]

	err = bm.DeleteEventByID(ctx, eventToDelete.ID)
	if err != nil {
		bm.Errorf("Ошибка удаления события: %v", err)
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка при удалении события",
		})
		bm.onError(err)
		return
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "✅ Событие удалено!",
	})
	bm.onError(err)
}

func (bm BotManager) GetUserEventsPaged(ctx context.Context, chatID int64, page int, pageSize int) ([]model.Event, int, error) {
	events, err := bm.GetUserEvents(ctx, chatID)
	if err != nil {
		return nil, 0, err
	}

	total := len(events)
	start := (page - 1) * pageSize
	if start >= total {
		return []model.Event{}, total, nil
	}

	end := start + pageSize
	if end > total {
		end = total
	}

	return events[start:end], total, nil
}

func (bm BotManager) ListHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	page := 1
	pageSize := 10

	parts := strings.Fields(update.Message.Text)
	if len(parts) > 1 {
		if p, err := strconv.Atoi(parts[1]); err == nil && p > 0 {
			page = p
		}
	}

	events, err := bm.GetUserEvents(ctx, update.Message.Chat.ID)
	if err != nil {
		bm.Errorf("Ошибка загрузки событий: %v", err)
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка при загрузке событий",
		})
		return
	}

	if len(events) == 0 {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "🔍 Нет событий",
		})
		return
	}

	total := len(events)
	start := (page - 1) * pageSize
	if start >= total {
		start = 0
		page = 1
	}

	end := start + pageSize
	if end > total {
		end = total
	}

	pageEvents := events[start:end]

	periodicCount, err := bm.eventsRepo.CountUserPeriodicEvents(ctx, update.Message.Chat.ID)
	if err != nil {
		bm.Errorf("Ошибка подсчета периодических событий: %v", err)
		periodicCount = 0
	}

	var msg strings.Builder
	msg.WriteString("📅 Список событий:\n\n")
	msg.WriteString(fmt.Sprintf("📊 Периодических уведомлений: %d/%d\n\n", periodicCount, MaxPeriodic))

	for i, e := range pageEvents {
		msg.WriteString(fmt.Sprintf("%d. %s — ", start+i+1, e.Text))
		msg.WriteString(fmt.Sprintf("%s\n", e.DateTime.Format("2006-01-02 15:04")))

		if e.Periodicity != nil {
			switch *e.Periodicity {
			case "hour":
				msg.WriteString("🔄 Каждый час\n")
			case "day":
				msg.WriteString("🔄 Ежедневно\n")
			case "week":
				msg.WriteString("🔄 Еженедельно\n")
			case "weekdays":
				var days []string
				for _, day := range e.Weekdays {
					days = append(days, dayName(day))
				}
				msg.WriteString(fmt.Sprintf("🔄 По дням: %s\n", strings.Join(days, ", ")))
			}
		} else {
			msg.WriteString("⏹️ Без повтора\n")
		}
	}

	var buttons [][]models.InlineKeyboardButton

	if page > 1 {
		buttons = append(buttons, []models.InlineKeyboardButton{
			{Text: "⬅️ Назад", CallbackData: fmt.Sprintf("page_%d", page-1)},
		})
	}

	if end < total {
		buttons = append(buttons, []models.InlineKeyboardButton{
			{Text: "➡️ Далее", CallbackData: fmt.Sprintf("page_%d", page+1)},
		})
	}

	if buttons == nil {
		buttons = [][]models.InlineKeyboardButton{}
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   msg.String(),
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: buttons,
		},
	})
	bm.onError(err)
}
func dayName(day int) string {
	switch day {
	case 1:
		return "Пн"
	case 2:
		return "Вт"
	case 3:
		return "Ср"
	case 4:
		return "Чт"
	case 5:
		return "Пт"
	case 6:
		return "Сб"
	case 7:
		return "Вс"
	default:
		return strconv.Itoa(day)
	}
}

func (bm BotManager) SendDailyEvents(ctx context.Context) {
	users, err := bm.eventsRepo.AllUsersWithEventsToday(ctx)
	if err != nil {
		bm.Errorf("Ошибка получения пользователей: %v", err)
		return
	}

	for _, userID := range users {
		events, err := bm.GetUserEvents(ctx, userID)
		if err != nil {
			bm.Errorf("Ошибка загрузки событий пользователя %d: %v", userID, err)
			continue
		}

		var todayEvents []model.Event
		today := time.Now().In(time.FixedZone("MSK", 3*3600)).Format("2006-01-02")

		for _, e := range events {
			if e.DateTime.Format("2006-01-02") == today {
				todayEvents = append(todayEvents, e)
			}
		}

		if len(todayEvents) == 0 {
			continue
		}

		var msg strings.Builder
		msg.WriteString("📅 События на сегодня:\n\n")

		for i, e := range todayEvents {
			msg.WriteString(fmt.Sprintf("%d. %s — %s\n", i+1, e.Text, e.DateTime.Format("15:04")))

			if e.Periodicity != nil {
				switch *e.Periodicity {
				case "hour":
					msg.WriteString("🔄 Каждый час\n")
				case "day":
					msg.WriteString("🔄 Ежедневно\n")
				case "week":
					msg.WriteString("🔄 Еженедельно\n")
				case "weekdays":
					var days []string
					for _, d := range e.Weekdays {
						days = append(days, dayName(d))
					}
					msg.WriteString(fmt.Sprintf("🔄 По дням: %s\n", strings.Join(days, ", ")))
				}
			} else {
				msg.WriteString("⏹️ Без повтора\n")
			}
		}

		_, err = bm.b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: userID,
			Text:   msg.String(),
		})
		bm.onError(err)
	}
}

type BotManager struct {
	embedlog.Logger
	b          *bot.Bot
	eventsRepo db.EventsRepo
}

func NewBotManager(b *bot.Bot, eventsRepo db.EventsRepo, logger embedlog.Logger) *BotManager {
	return &BotManager{
		b:          b,
		eventsRepo: eventsRepo,
		Logger:     logger,
	}
}

func (bm *BotManager) onError(err error) {
	if err == nil {
		return
	}
	bm.Errorf("%v", err)
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

func (bm BotManager) SendReminderPeriodicity(ctx context.Context, chatID int64, text string) {
	_, err := bm.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "🔔 Напоминание: " + text,
	})
	bm.onError(err)
}

func (bm BotManager) AddEvent(ctx context.Context, chatId int64, parts []string) (*model.Event, error) {
	datePart := parts[0]
	timePart := parts[1]
	text := parts[2]

	if len(text) > 200 {
		return nil, fmt.Errorf("text_too_long")
	}

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		bm.Errorf("Ошибка загрузки часового пояса: %v", err)
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
		UserTgID:    chatId,
		Message:     text,
		SendAt:      dt,
		StatusID:    db.StatusEnabled,
		Weekdays:    []int{},
		Periodicity: nil,
	}

	addedEvent, err := bm.eventsRepo.AddEvent(ctx, event)
	if err != nil {
		bm.Errorf("Ошибка сохранения события: %v", err)
		return nil, err
	}

	err = bm.askForPeriodicity(ctx, chatId, addedEvent.ID)
	if err != nil {
		bm.Errorf("Ошибка запроса периодичности: %v", err)
	}

	return model.NewEvent(addedEvent), nil
}

func (bm BotManager) askForPeriodicity(ctx context.Context, chatID int64, eventID int) error {
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "🕐 Каждый час", CallbackData: fmt.Sprintf("period:hour:%d", eventID)},
			},
			{
				{Text: "📅 Каждый день", CallbackData: fmt.Sprintf("period:day:%d", eventID)},
			},
			{
				{Text: "🗓️ Каждую неделю", CallbackData: fmt.Sprintf("period:week:%d", eventID)},
			},
			{
				{Text: "🔢 Выбранные дни недели", CallbackData: fmt.Sprintf("period:weekdays:%d", eventID)},
			},
			{
				{Text: "❌ Без повтора", CallbackData: fmt.Sprintf("period:none:%d", eventID)},
			},
		},
	}

	_, err := bm.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        "📅 Выберите периодичность уведомления:",
		ReplyMarkup: keyboard,
	})
	return err
}

func (bm BotManager) askForWeekdays(ctx context.Context, chatID int64, eventID int, selectedDays []int) error {
	keyboard := bm.makeWeekdaysKeyboard(eventID, selectedDays)
	_, err := bm.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        "Выберите дни недели:",
		ReplyMarkup: keyboard,
	})
	return err
}

func (bm BotManager) makeWeekdaysKeyboard(eventID int, selectedDays []int) *models.InlineKeyboardMarkup {
	res := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}}

	for _, weekDay := range weekDays {
		buttonText := weekDay.Name
		dayInt, err := strconv.Atoi(weekDay.ID)
		if err != nil {
			bm.Errorf("Ошибка конвертации дня недели: %v", err)
			continue
		}

		if slices.Contains(selectedDays, dayInt) {
			buttonText = "✅ " + weekDay.Name
		} else {
			buttonText = "❌ " + weekDay.Name
		}

		res.InlineKeyboard = append(res.InlineKeyboard, []models.InlineKeyboardButton{{
			Text:         buttonText,
			CallbackData: fmt.Sprintf("weekday:%s:%d", weekDay.ID, eventID),
		}})
	}

	res.InlineKeyboard = append(res.InlineKeyboard, []models.InlineKeyboardButton{
		{
			Text:         "✅ Готово",
			CallbackData: fmt.Sprintf("weekdays_done:%d", eventID),
		},
	})

	return res
}

func (bm BotManager) HandlePeriodicityCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	data := update.CallbackQuery.Data
	chatID := update.CallbackQuery.From.ID
	messageID := update.CallbackQuery.Message.Message.ID

	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
	bm.onError(err)

	bm.handlePeriodicityCallback(ctx, b, data, chatID, messageID)
}

func (bm BotManager) HandleWeekdayCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	data := update.CallbackQuery.Data
	chatID := update.CallbackQuery.From.ID
	messageID := update.CallbackQuery.Message.Message.ID

	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
	bm.onError(err)

	bm.handleWeekdayCallback(ctx, b, data, chatID, messageID)
}

func (bm BotManager) HandleWeekdaysDoneCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	data := update.CallbackQuery.Data
	chatID := update.CallbackQuery.From.ID
	messageID := update.CallbackQuery.Message.Message.ID

	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
	bm.onError(err)

	bm.handleWeekdaysDoneCallback(ctx, b, data, chatID, messageID)
}

func (bm BotManager) handlePeriodicityCallback(ctx context.Context, b *bot.Bot, data string, chatID int64, messageID int) {
	parts := strings.Split(data, ":")
	if len(parts) < 3 {
		return
	}

	periodType := parts[1]
	eventID, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}

	event, err := bm.eventsRepo.EventByID(ctx, eventID)
	if err != nil || event == nil {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Ошибка: событие не найдено",
		})
		bm.onError(err)
		return
	}

	if periodType != "none" && periodType != "weekdays" {
		count, err := bm.eventsRepo.CountUserPeriodicEvents(ctx, chatID)
		if err != nil {
			bm.Errorf("Ошибка подсчёта событий: %v", err)
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "❌ Не удалось проверить количество событий",
			})
			bm.onError(err)
			return
		}

		if count >= MaxPeriodic {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "⚠️ Превышен лимит: максимум 100 периодических напоминаний на одного пользователя.",
			})
			bm.onError(err)
			_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
				ChatID:    chatID,
				MessageID: messageID,
			})
			bm.onError(err)
			return
		}
	}

	switch periodType {
	case "none":
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "✅ Событие добавлено без повтора!",
		})
		bm.onError(err)
		_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    chatID,
			MessageID: messageID,
		})
		bm.onError(err)
		return

	case "weekdays":
		err := bm.askForWeekdays(ctx, chatID, eventID, event.Weekdays)
		bm.onError(err)
		_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    chatID,
			MessageID: messageID,
		})
		bm.onError(err)
		return

	default:
		event.Periodicity = &periodType
		_, err = bm.eventsRepo.UpdateEvent(ctx, event, db.WithColumns("periodicity"))
		if err != nil {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "❌ Ошибка обновления события",
			})
			bm.onError(err)
			return
		}
	}

	periodicityText := getPeriodicityText(periodType)
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   fmt.Sprintf("✅ Событие добавлено! %s", periodicityText),
	})
	bm.onError(err)
	_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatID,
		MessageID: messageID,
	})
	bm.onError(err)
}

func (bm BotManager) handleWeekdayCallback(ctx context.Context, b *bot.Bot, data string, chatID int64, messageID int) {
	parts := strings.Split(data, ":")
	if len(parts) < 3 {
		return
	}

	dayStr := parts[1]
	eventID, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}

	event, err := bm.eventsRepo.EventByID(ctx, eventID)
	if err != nil || event == nil {
		return
	}

	day, err := strconv.Atoi(dayStr)
	if err != nil {
		return
	}

	newWeekdays := toggleDayInSlice(event.Weekdays, day)
	event.Weekdays = newWeekdays

	_, err = bm.eventsRepo.UpdateEvent(ctx, event, db.WithColumns("weekdays"))
	if err != nil {
		bm.Errorf("Ошибка обновления дней недели для события %d: %v", eventID, err)
		return
	}

	keyboard := bm.makeWeekdaysKeyboard(eventID, newWeekdays)
	_, err = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:      chatID,
		MessageID:   messageID,
		ReplyMarkup: keyboard,
	})
	bm.onError(err)
}

func (bm BotManager) handleWeekdaysDoneCallback(ctx context.Context, b *bot.Bot, data string, chatID int64, messageID int) {
	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return
	}

	eventID, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}

	event, err := bm.eventsRepo.EventByID(ctx, eventID)
	if err != nil || event == nil {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Ошибка: событие не найдено",
		})
		bm.onError(err)
		return
	}

	if len(event.Weekdays) == 0 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Выберите хотя бы один день недели",
		})
		bm.onError(err)
		return
	}

	periodicity := db.PeriodicityWeekdays
	event.Periodicity = &periodicity
	_, err = bm.eventsRepo.UpdateEvent(ctx, event, db.WithColumns("periodicity", "weekdays"))
	if err != nil {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Ошибка обновления события",
		})
		bm.onError(err)
		return
	}

	var daysNames []string
	for _, day := range event.Weekdays {
		daysNames = append(daysNames, dayName(day))
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   fmt.Sprintf("✅ Событие добавлено! 🔄 По дням: %s", strings.Join(daysNames, ", ")),
	})
	bm.onError(err)
	_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatID,
		MessageID: messageID,
	})
	bm.onError(err)
}

func toggleDayInSlice(slice []int, day int) []int {
	if slices.Contains(slice, day) {
		var newSlice []int
		for _, d := range slice {
			if d != day {
				newSlice = append(newSlice, d)
			}
		}
		return newSlice
	}
	return append(slice, day)
}

func getPeriodicityText(periodType string) string {
	switch periodType {
	case db.PeriodicityHour:
		return "🔄 Каждый час"
	case db.PeriodicityDay:
		return "🔄 Ежедневно"
	case db.PeriodicityWeek:
		return "🔄 Еженедельно"
	default:
		return ""
	}
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

	return model.NewEvents(dbEvents), nil
}

func (bm BotManager) GetEventByID(ctx context.Context, id int) (*model.Event, error) {
	dbEvent, err := bm.eventsRepo.EventByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if dbEvent == nil {
		return nil, nil
	}

	return model.NewEvent(dbEvent), nil
}

func (bm BotManager) RestoreReminders(ctx context.Context, rm ReminderScheduler) error {
	statusId := db.StatusEnabled
	events, err := bm.eventsRepo.EventsByFilters(ctx, &db.EventSearch{StatusID: &statusId}, db.PagerNoLimit)
	if err != nil {
		bm.Errorf("Ошибка восстановления напоминаний: %v", err)
		return err
	}

	for _, e := range events {
		reminderEvent := model.NewReminderEvent(&e)

		if e.Periodicity != nil && e.SendAt.Before(time.Now()) {
			nextTime := rm.CalculateNextTime(reminderEvent)
			if nextTime != nil {
				_, err := bm.eventsRepo.UpdateEvent(ctx, &db.Event{
					ID:     e.ID,
					SendAt: *nextTime,
				}, db.WithColumns("sendAt"))
				if err != nil {
					return err
				}
				reminderEvent.DateTime = *nextTime
			}
		}

		rm.ScheduleReminder(ctx, reminderEvent)
		bm.Printf("Восстановлено напоминание: ID=%d", e.ID)
	}

	return nil
}

type ReminderScheduler interface {
	ScheduleReminder(ctx context.Context, e model.ReminderEvent) context.CancelFunc
	CalculateNextTime(e model.ReminderEvent) *time.Time
}

var (
	ErrNotFound     = errors.New("event not found")
	ErrAccessDenied = errors.New("access denied")
	ErrInactive     = errors.New("event not active")
	ErrPastDate     = errors.New("past_date")
)

func (bm BotManager) SnoozeEvent(ctx context.Context, eventID int, userTgID int64, newTime time.Time) error {
	event, err := bm.eventsRepo.EventByID(ctx, eventID)
	if err != nil {
		return err
	}

	if event == nil {
		return ErrNotFound
	}
	if event.UserTgID != userTgID {
		return ErrAccessDenied
	}

	if event.StatusID != db.StatusEnabled {
		return ErrInactive
	}

	if newTime.Before(time.Now()) {
		return ErrPastDate
	}

	event.SendAt = newTime
	_, err = bm.eventsRepo.UpdateEvent(ctx, event, db.WithColumns(db.Columns.Event.SendAt))
	if err != nil {
		return err
	}

	return nil
}
