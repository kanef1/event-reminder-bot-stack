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

	id, err := strconv.Atoi(args)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❗ ID должен быть числом",
		})
		return
	}

	err = bm.DeleteEventByID(ctx, id)
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

	var msg strings.Builder
	msg.WriteString("📅 Список событий:\n\n")

	for i, e := range events {
		periodicityText := ""
		if e.Periodicity != nil {
			switch *e.Periodicity {
			case db.PeriodicityHour:
				periodicityText = "🔄 Каждый час"
			case db.PeriodicityDay:
				periodicityText = "🔄 Ежедневно"
			case db.PeriodicityWeek:
				periodicityText = "🔄 Еженедельно"
			case db.PeriodicityWeekdays:
				days := []string{}
				for _, day := range e.Weekdays {
					days = append(days, dayName(day))
				}
				periodicityText = fmt.Sprintf("🔄 По дням: %s", strings.Join(days, ", "))
			}
		}

		msg.WriteString(fmt.Sprintf(
			"%d. %s\n⏰ %s\n",
			i+1,
			e.Text,
			e.DateTime.Format("2006-01-02 15:04"),
		))

		if periodicityText != "" {
			msg.WriteString(fmt.Sprintf("%s\n", periodicityText))
		}

		msg.WriteString(fmt.Sprintf("ID: %d\n\n", e.ID))
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   msg.String(),
	})
}

func dayName(day int) string {
	days := map[int]string{
		0: "Пн", 1: "Вт", 2: "Ср", 3: "Чт", 4: "Пт", 5: "Сб", 6: "Вс",
	}
	return days[day]
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

	log.Printf("🕒 Создано событие: время %v (локальное: %v, UTC: %v)",
		dt, dt.Local(), dt.UTC())

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

	// Запрашиваем периодичность
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

// askForPeriodicity запрашивает тип периодичности
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

// askForWeekdays запрашивает выбор дней недели
func (bm BotManager) askForWeekdays(ctx context.Context, chatID int64, eventID int, selectedDays []int) error {
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{},
	}

	days := []struct {
		name string
		day  int
	}{
		{"Пн", 0}, {"Вт", 1}, {"Ср", 2}, {"Чт", 3}, {"Пт", 4}, {"Сб", 5}, {"Вс", 6},
	}

	var row []models.InlineKeyboardButton
	for i, day := range days {
		icon := "⚪"
		if contains(selectedDays, day.day) {
			icon = "✅"
		}

		btn := models.InlineKeyboardButton{
			Text:         fmt.Sprintf("%s %s", icon, day.name),
			CallbackData: fmt.Sprintf("weekday:%d:%d", eventID, day.day),
		}
		row = append(row, btn)

		// Создаем новую строку каждые 3 дня
		if (i+1)%3 == 0 || i == len(days)-1 {
			keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, row)
			row = []models.InlineKeyboardButton{}
		}
	}

	// Кнопка "Готово"
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{
		{Text: "✅ Готово", CallbackData: fmt.Sprintf("weekdays_done:%d", eventID)},
	})

	_, err := bm.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        "Выберите дни недели:",
		ReplyMarkup: keyboard,
	})
	return err
}

// HandleCallback обрабатывает callback от кнопок
func (bm BotManager) HandleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	data := update.CallbackQuery.Data
	parts := strings.Split(data, ":")

	if len(parts) < 2 {
		return
	}

	chatID := update.CallbackQuery.From.ID

	switch parts[0] {
	case "period":
		if len(parts) < 3 {
			return
		}
		periodType := parts[1]
		eventID, _ := strconv.Atoi(parts[2])

		bm.handlePeriodicitySelection(ctx, b, chatID, eventID, periodType, 0)

	case "weekday":
		if len(parts) < 3 {
			return
		}
		eventID, _ := strconv.Atoi(parts[1])
		day, _ := strconv.Atoi(parts[2])

		bm.handleWeekdaySelection(ctx, b, chatID, eventID, day, 0)

	case "weekdays_done":
		if len(parts) < 2 {
			return
		}
		eventID, _ := strconv.Atoi(parts[1])

		bm.handleWeekdaysDone(ctx, b, chatID, eventID, 0)
	}
}

func (bm BotManager) handlePeriodicitySelection(ctx context.Context, b *bot.Bot, chatID int64, eventID int, periodType string, messageID int) {
	event, err := bm.eventsRepo.EventByID(ctx, eventID)
	if err != nil || event == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Ошибка: событие не найдено",
		})
		return
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
		bm.askForWeekdays(ctx, chatID, eventID, []int{})
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

func (bm BotManager) handleWeekdaySelection(ctx context.Context, b *bot.Bot, chatID int64, eventID int, day int, messageID int) {
	event, err := bm.eventsRepo.EventByID(ctx, eventID)
	if err != nil || event == nil {
		return
	}

	// Переключаем выбранный день
	newWeekdays := toggleDayInSlice(event.Weekdays, day)
	event.Weekdays = newWeekdays

	// Временно сохраняем выбранные дни
	_, err = bm.eventsRepo.UpdateEvent(ctx, event, db.WithColumns("weekdays"))
	if err != nil {
		return
	}

	// Обновляем сообщение с новыми галочками
	bm.askForWeekdays(ctx, chatID, eventID, newWeekdays)

	// Удаляем старое сообщение
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatID,
		MessageID: messageID,
	})
}

func (bm BotManager) handleWeekdaysDone(ctx context.Context, b *bot.Bot, chatID int64, eventID int, messageID int) {
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

	// Устанавливаем периодичность
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

	// Формируем текст с выбранными днями
	daysNames := []string{}
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
		newSlice := []int{}
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

	loc := GetMoscowLocation()

	sort.Slice(dbEvents, func(i, j int) bool {
		return dbEvents[i].SendAt.Before(dbEvents[j].SendAt)
	})

	events := make([]model.Event, len(dbEvents))
	for i, dbEvent := range dbEvents {
		moscowTime := dbEvent.SendAt.In(loc)
		events[i] = model.Event{
			ID:         dbEvent.ID,
			OriginalID: dbEvent.ID,
			ChatID:     dbEvent.UserTgID,
			Text:       dbEvent.Message,
			DateTime:   moscowTime,
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

	loc := GetMoscowLocation()
	moscowTime := dbEvent.SendAt.In(loc)

	return &model.Event{
		ID:         dbEvent.ID,
		OriginalID: dbEvent.ID,
		ChatID:     dbEvent.UserTgID,
		Text:       dbEvent.Message,
		DateTime:   moscowTime, // Московское время
	}, nil
}

func GetMoscowLocation() *time.Location {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return time.UTC // fallback
	}
	return loc
}
