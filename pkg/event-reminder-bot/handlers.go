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
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Нет такой команды, используйте /help чтобы посмотреть доступные команды команд",
	})
}

func StartHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text: "Добрый день, данный бот предназначен для простого планирования.\n" +
			"Список умений:\n" +
			"Добавить событие: /add 2025-08-08 21:05 <Текст>\n" +
			"Список событий: /list \n" +
			"Удалить событие: /delete id\n" +
			"Список команд: /help",
	})
}

func HelpHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text: "Список умений:\n" +
			"Добавить событие: /add 2025-08-08 21:05 <Текст>\n" +
			"Список событий: /list\n" +
			"Удалить событие: /delete id\n" +
			"Список команд: /help",
	})
}

func DeleteHandler(ctx context.Context, b *bot.Bot, update *models.Update, bm *BotManager) {
	args := strings.TrimSpace(strings.TrimPrefix(update.Message.Text, "/delete"))
	if args == "" {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❗ Укажите ID события, например: /delete 123",
		})
		return
	}

	index, err := strconv.Atoi(args)
	if err != nil || index < 1 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❗ Номер события должен быть положительным числом",
		})
		return
	}

	events, err := bm.GetUserEvents(ctx, update.Message.Chat.ID)
	if err != nil {
		log.Printf("Ошибка загрузки событий: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка при загрузке событий",
		})
		return
	}

	if len(events) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "🔍 У вас нет событий для удаления",
		})
		return
	}

	if index > len(events) {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❗ Нет события с номером %d", index),
		})
		return
	}

	eventToDelete := events[index-1]

	err = bm.DeleteEventByID(ctx, eventToDelete.ID)
	if err != nil {
		log.Printf("Ошибка удаления события: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка при удалении события",
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "✅ Событие удалено!",
	})
}

func ListHandler(ctx context.Context, b *bot.Bot, update *models.Update, bm *BotManager) {
	events, err := bm.GetUserEvents(ctx, update.Message.Chat.ID)
	if err != nil {
		log.Printf("Ошибка загрузки событий: %v", err)
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Ошибка при загрузке событий",
		})
		return
	}

	if len(events) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "🔍 Нет событий",
		})
		return
	}

	// Получаем количество периодических событий для статистики
	periodicCount, _ := bm.eventsRepo.CountUserPeriodicEvents(ctx, update.Message.Chat.ID)

	var msg strings.Builder
	msg.WriteString("📅 Список событий:\n\n")

	// Добавляем информацию о лимите периодических событий
	msg.WriteString(fmt.Sprintf("📊 Периодических уведомлений: %d/%d\n\n", periodicCount, MaxPeriodic))

	for i, e := range events {
		msg.WriteString(fmt.Sprintf("%d. %s", i+1, e.Text))
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
				days := []string{}
				for _, day := range e.Weekdays {
					days = append(days, dayName(day))
				}
				msg.WriteString(fmt.Sprintf("🔄 По дням: %s\n", strings.Join(days, ", ")))
			}
		} else {
			msg.WriteString("⏹️ Без повтора\n")
		}

	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   msg.String(),
	})
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

type BotManager struct {
	b          *bot.Bot
	eventsRepo db.EventsRepo
}

func NewBotManager(b *bot.Bot, eventsRepo db.EventsRepo) *BotManager {
	return &BotManager{b: b, eventsRepo: eventsRepo}
}

func (bm BotManager) SendReminder(ctx context.Context, chatID int64, text string) {
	bm.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "🔔 Напоминание: " + text,
	})
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
		UserTgID:    chatId,
		Message:     text,
		SendAt:      dt,
		StatusID:    db.StatusEnabled,
		Weekdays:    []int{},
		Periodicity: nil,
	}

	addedEvent, err := bm.eventsRepo.AddEvent(ctx, event)
	if err != nil {
		log.Printf("Ошибка сохранения события: %v", err)
		return nil, err
	}

	err = bm.askForPeriodicity(ctx, chatId, addedEvent.ID)
	if err != nil {
		log.Printf("Ошибка запроса периодичности: %v", err)
	}

	return &model.Event{
		ID:          addedEvent.ID,
		OriginalID:  addedEvent.ID,
		ChatID:      addedEvent.UserTgID,
		Text:        addedEvent.Message,
		DateTime:    addedEvent.SendAt.In(loc),
		Weekdays:    addedEvent.Weekdays,
		Periodicity: addedEvent.Periodicity,
	}, nil
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
		dayInt, _ := strconv.Atoi(weekDay.ID)

		if contains(selectedDays, dayInt) {
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

func (bm BotManager) HandleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}

	data := update.CallbackQuery.Data
	chatID := update.CallbackQuery.From.ID
	messageID := update.CallbackQuery.Message.Message.ID

	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})

	if strings.HasPrefix(data, "period:") {
		bm.handlePeriodicityCallback(ctx, b, data, chatID, messageID)
	} else if strings.HasPrefix(data, "weekday:") {
		bm.handleWeekdayCallback(ctx, b, data, chatID, messageID)
	} else if strings.HasPrefix(data, "weekdays_done:") {
		bm.handleWeekdaysDoneCallback(ctx, b, data, chatID, messageID)
	}
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
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Ошибка: событие не найдено",
		})
		return
	}

	if periodType != "none" && periodType != "weekdays" {
		count, err := bm.eventsRepo.CountUserPeriodicEvents(ctx, chatID)
		if err != nil {
			log.Printf("Ошибка подсчёта событий: %v", err)
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "❌ Не удалось проверить количество событий",
			})
			return
		}

		if count >= MaxPeriodic {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "⚠️ Превышен лимит: максимум 100 периодических напоминаний на одного пользователя.",
			})
			b.DeleteMessage(ctx, &bot.DeleteMessageParams{
				ChatID:    chatID,
				MessageID: messageID,
			})
			return
		}
	}

	switch periodType {
	case "none":
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "✅ Событие добавлено без повтора!",
		})
		b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    chatID,
			MessageID: messageID,
		})
		return

	case "weekdays":
		bm.askForWeekdays(ctx, chatID, eventID, event.Weekdays)
		b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    chatID,
			MessageID: messageID,
		})
		return

	default:
		event.Periodicity = &periodType
		_, err = bm.eventsRepo.UpdateEvent(ctx, event, db.WithColumns("periodicity"))
		if err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "❌ Ошибка обновления события",
			})
			return
		}
	}

	periodicityText := getPeriodicityText(periodType)
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   fmt.Sprintf("✅ Событие добавлено! %s", periodicityText),
	})
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatID,
		MessageID: messageID,
	})
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
		return
	}

	keyboard := bm.makeWeekdaysKeyboard(eventID, newWeekdays)
	b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:      chatID,
		MessageID:   messageID,
		ReplyMarkup: keyboard,
	})
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
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Ошибка: событие не найдено",
		})
		return
	}

	if len(event.Weekdays) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Выберите хотя бы один день недели",
		})
		return
	}

	periodicity := db.PeriodicityWeekdays
	event.Periodicity = &periodicity
	_, err = bm.eventsRepo.UpdateEvent(ctx, event, db.WithColumns("periodicity", "weekdays"))
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Ошибка обновления события",
		})
		return
	}

	var daysNames []string
	for _, day := range event.Weekdays {
		daysNames = append(daysNames, dayName(day))
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   fmt.Sprintf("✅ Событие добавлено! 🔄 По дням: %s", strings.Join(daysNames, ", ")),
	})
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatID,
		MessageID: messageID,
	})
}

func contains(slice []int, item int) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func toggleDayInSlice(slice []int, day int) []int {
	if contains(slice, day) {
		var newSlice []int
		for _, d := range slice {
			if d != day {
				newSlice = append(newSlice, d)
			}
		}
		return newSlice
	} else {
		return append(slice, day)
	}
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
	search := &db.EventSearch{UserTgID: &chatID}
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
			ID:          dbEvent.ID,
			OriginalID:  dbEvent.ID,
			ChatID:      dbEvent.UserTgID,
			Text:        dbEvent.Message,
			DateTime:    dbEvent.SendAt,
			Weekdays:    dbEvent.Weekdays,
			Periodicity: dbEvent.Periodicity,
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
		ID:          dbEvent.ID,
		OriginalID:  dbEvent.ID,
		ChatID:      dbEvent.UserTgID,
		Text:        dbEvent.Message,
		DateTime:    dbEvent.SendAt,
		Weekdays:    dbEvent.Weekdays,
		Periodicity: dbEvent.Periodicity,
	}, nil
}
