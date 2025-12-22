package botService

import (
	"context"
	"errors"
	"event-reminder-bot/pkg/db"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	botManager "event-reminder-bot/pkg/event-reminder-bot"
	"event-reminder-bot/pkg/reminder"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	startCommand = "/start"
	helpCommand  = "/help"
	addCommand   = "/add"
	listCommand  = "/list"

	eventDetailPrefix = "event_detail_"
	eventEditPrefix   = "event_edit_"
	eventDeletePrefix = "event_delete_"
	eventBackToList   = "back_to_list"

	editDatePrefix        = "edit_date_"
	editDescPrefix        = "edit_desc_"
	editPeriodicityPrefix = "edit_periodicity_"

	postponeHour   = "postpone_hour_"
	postponeDay    = "postpone_day_"
	postponeWeek   = "postpone_week_"
	postponeCustom = "postpone_custom_"
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
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, startCommand, bot.MatchTypeExact, botManager.StartHandler)
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, helpCommand, bot.MatchTypeExact, botManager.HelpHandler)
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, addCommand, bot.MatchTypePrefix, bs.AddHandler)
	bs.b.RegisterHandler(bot.HandlerTypeMessageText, listCommand, bot.MatchTypeExact, bs.bm.ListHandler)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "done_", bot.MatchTypePrefix, bs.handleDoneCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "snooze_", bot.MatchTypePrefix, bs.handleSnoozeCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "period:", bot.MatchTypePrefix, bs.bm.HandlePeriodicityCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "weekday:", bot.MatchTypePrefix, bs.bm.HandleWeekdayCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "weekdays_done:", bot.MatchTypePrefix, bs.bm.HandleWeekdaysDoneCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "page_", bot.MatchTypePrefix, bs.handlePageCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, eventDetailPrefix, bot.MatchTypePrefix, bs.handleEventDetailCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, eventEditPrefix, bot.MatchTypePrefix, bs.handleEventEditCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, eventDeletePrefix, bot.MatchTypePrefix, bs.handleEventDeleteCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, eventBackToList, bot.MatchTypeExact, bs.handleBackToListCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, editDatePrefix, bot.MatchTypePrefix, bs.handleEditDateCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, editDescPrefix, bot.MatchTypePrefix, bs.handleEditDescCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, editPeriodicityPrefix, bot.MatchTypePrefix, bs.handleEditPeriodicityCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, postponeHour, bot.MatchTypePrefix, bs.handlePostponeCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, postponeDay, bot.MatchTypePrefix, bs.handlePostponeCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, postponeWeek, bot.MatchTypePrefix, bs.handlePostponeCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, postponeCustom, bot.MatchTypePrefix, bs.handlePostponeCustomCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "edit_period:", bot.MatchTypePrefix, bs.handleEditPeriodicityValueCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "edit_weekday:", bot.MatchTypePrefix, bs.handleEditWeekdayCallback)
	bs.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "edit_weekdays_done:", bot.MatchTypePrefix, bs.handleEditWeekdaysDoneCallback)
	bs.b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil && update.Message.Text != ""
	}, bs.textHandler)
}

func (bs *BotService) AddHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	args := strings.TrimSpace(strings.TrimPrefix(update.Message.Text, "/add"))
	parts := strings.SplitN(args, " ", 3)
	if len(parts) < 3 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❗ Формат: /add YYYY-MM-DD HH:MM Текст",
		})
		if err != nil {
			return
		}
		return
	}

	_, err := bs.bm.AddEvent(ctx, update.Message.Chat.ID, parts)
	if err != nil {
		var text string
		switch err.Error() {
		case "invalid_format":
			text = "❗ Недопустимый формат даты (используйте YYYY-MM-DD HH:MM)"
		case "past_date":
			text = "❗ Недопустимый формат даты (событие должно быть в будущем)"
		case "text_too_long":
			text = "❗ Текст события должен быть не длиннее 200 символов"
		default:
			text = fmt.Sprintf("Ошибка: %v", err)
		}

		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   text,
		})
		if err != nil {
			return
		}
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
	eventID, ok := bs.snoozeStates[chatID]
	bs.mu.RUnlock()

	if ok {
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
			return
		}

		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   fmt.Sprintf("✅ Событие перенесено на %s", newTime.Format("2006-01-02 15:04")),
		})
		if err != nil {
			return
		}
		return
	}

	bs.bm.Mu.RLock()
	editState, existsEdit := bs.bm.EditStates[chatID]
	bs.bm.Mu.RUnlock()

	if existsEdit {
		bs.bm.Mu.Lock()
		delete(bs.bm.EditStates, chatID)
		bs.bm.Mu.Unlock()

		switch editState.WaitingFor {
		case "custom_date":
			bs.handleCustomDateInput(ctx, b, chatID, text, editState.EventID)
			return
		case "description":
			bs.handleDescriptionInput(ctx, b, chatID, text, editState.EventID)
			return
		}
	}

	botManager.DefaultHandler(ctx, b, update)
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

func (bs *BotService) handlePageCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	data := update.CallbackQuery.Data
	chatID := update.CallbackQuery.Message.Message.Chat.ID

	parts := strings.Split(data, "_")
	if len(parts) != 2 {
		return
	}

	page, err := strconv.Atoi(parts[1])
	if err != nil || page < 1 {
		return
	}

	pageSize := 10

	events, total, err := bs.bm.GetUserEventsPaged(ctx, chatID, page, pageSize)
	if err != nil {
		bs.bm.Errorf("Ошибка загрузки событий: %v", err)
		return
	}

	if len(events) == 0 {
		return
	}

	periodicCount, err := bs.bm.EventsRepo.CountUserPeriodicEvents(ctx, chatID)
	if err != nil {
		bs.bm.Errorf("Ошибка подсчета периодических событий: %v", err)
		periodicCount = 0
	}

	start := (page - 1) * pageSize

	var msg strings.Builder
	msg.WriteString("📅 Список событий:\n\n")
	msg.WriteString(fmt.Sprintf("📊 Периодических уведомлений: %d/%d\n\n", periodicCount, botManager.MaxPeriodic))

	for i, e := range events {
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
					days = append(days, botManager.DayName(day))
				}
				msg.WriteString(fmt.Sprintf("🔄 По дням: %s\n", strings.Join(days, ", ")))
			}
		} else {
			msg.WriteString("⏹️ Без повтора\n")
		}
	}

	var buttons [][]models.InlineKeyboardButton

	row := []models.InlineKeyboardButton{}
	for i := range events {
		eventNum := start + i + 1
		row = append(row, models.InlineKeyboardButton{
			Text:         fmt.Sprintf("%d", eventNum),
			CallbackData: fmt.Sprintf("%s%d", botManager.EventDetailPrefix, events[i].ID),
		})

		if len(row) == 5 {
			buttons = append(buttons, row)
			row = []models.InlineKeyboardButton{}
		}
	}
	if len(row) > 0 {
		buttons = append(buttons, row)
	}

	var navRow []models.InlineKeyboardButton
	if page > 1 {
		navRow = append(navRow, models.InlineKeyboardButton{
			Text:         "⬅️ Назад",
			CallbackData: fmt.Sprintf("page_%d", page-1),
		})
	}
	if start+pageSize < total {
		navRow = append(navRow, models.InlineKeyboardButton{
			Text:         "➡️ Далее",
			CallbackData: fmt.Sprintf("page_%d", page+1),
		})
	}
	if len(navRow) > 0 {
		buttons = append(buttons, navRow)
	}

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: update.CallbackQuery.Message.Message.ID,
		Text:      msg.String(),
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: buttons,
		},
	})
}

func (bs *BotService) handleCallback(handler func(context.Context, *bot.Bot, string, int64, int)) func(context.Context, *bot.Bot, *models.Update) {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.CallbackQuery == nil {
			return
		}

		_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
		})
		bs.bm.OnError(err)

		handler(ctx, b, update.CallbackQuery.Data,
			update.CallbackQuery.Message.Message.Chat.ID,
			update.CallbackQuery.Message.Message.ID)
	}
}

func (bs *BotService) handleCallbackWithUserID(handler func(context.Context, *bot.Bot, string, int64, int)) func(context.Context, *bot.Bot, *models.Update) {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.CallbackQuery == nil {
			return
		}

		_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
		})
		bs.bm.OnError(err)

		handler(ctx, b, update.CallbackQuery.Data,
			update.CallbackQuery.From.ID,
			update.CallbackQuery.Message.Message.ID)
	}
}

func (bs *BotService) handleCallbackNoData(handler func(context.Context, *bot.Bot, int64, int)) func(context.Context, *bot.Bot, *models.Update) {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.CallbackQuery == nil {
			return
		}

		_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
		})
		bs.bm.OnError(err)

		handler(ctx, b,
			update.CallbackQuery.Message.Message.Chat.ID,
			update.CallbackQuery.Message.Message.ID)
	}
}

func (bs *BotService) handleEventDetailCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	bs.handleCallback(bs.bm.HandleEventDetail)(ctx, b, update)
}

func (bs *BotService) handleEventEditCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	bs.handleCallback(bs.bm.HandleEventEdit)(ctx, b, update)
}

func (bs *BotService) handleEventDeleteCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	bs.handleCallback(bs.bm.HandleEventDelete)(ctx, b, update)
}

func (bs *BotService) handleBackToListCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	bs.handleCallbackNoData(bs.bm.HandleBackToList)(ctx, b, update)
}

func (bs *BotService) handleEditDateCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	bs.handleCallback(bs.bm.HandleEditDate)(ctx, b, update)
}

func (bs *BotService) handleEditDescCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	bs.handleCallbackWithUserID(bs.bm.HandleEditDescription)(ctx, b, update)
}

func (bs *BotService) handleEditPeriodicityCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	bs.handleCallback(bs.bm.HandleEditPeriodicity)(ctx, b, update)
}

func (bs *BotService) handlePostponeCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	bs.handleCallback(bs.bm.HandlePostpone)(ctx, b, update)
}

func (bs *BotService) handlePostponeCustomCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	bs.handleCallbackWithUserID(bs.bm.HandlePostponeCustom)(ctx, b, update)
}

func (bs *BotService) handleEditPeriodicityValueCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	bs.handleCallback(bs.bm.HandleEditPeriodicityCallback)(ctx, b, update)
}

func (bs *BotService) handleEditWeekdayCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	bs.handleCallback(bs.bm.HandleEditWeekday)(ctx, b, update)
}

func (bs *BotService) handleEditWeekdaysDoneCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	bs.handleCallback(bs.bm.HandleEditWeekdaysDone)(ctx, b, update)
}
func (bs *BotService) handleCustomDateInput(ctx context.Context, b *bot.Bot, chatID int64, text string, eventID int) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Println("Ошибка загрузки часового пояса:", err)
		loc = time.Local
	}

	newTime, err := time.ParseInLocation("2006-01-02 15:04", text, loc)
	if err != nil {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❗ Недопустимый формат даты. Используйте: YYYY-MM-DD HH:MM\nНапример: 2025-12-31 23:59",
		})
		if err != nil {
			return
		}
		return
	}

	if newTime.Before(time.Now()) {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❗ Дата не может быть в прошлом",
		})
		if err != nil {
			return
		}
		return
	}

	event, err := bs.bm.EventsRepo.EventByID(ctx, eventID)
	if err != nil || event == nil {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Событие не найдено",
		})
		if err != nil {
			return
		}
		return
	}

	if event.UserTgID != chatID {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ У вас нет доступа к этому событию",
		})
		if err != nil {
			return
		}
		return
	}

	event.SendAt = newTime
	_, err = bs.bm.EventsRepo.UpdateEvent(ctx, event, db.WithColumns(db.Columns.Event.SendAt))
	if err != nil {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Ошибка при обновлении события",
		})
		if err != nil {
			return
		}
		return
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   fmt.Sprintf("✅ Дата изменена на %s", newTime.Format("2006-01-02 15:04")),
	})
	if err != nil {
		return
	}
}

func (bs *BotService) handleDescriptionInput(ctx context.Context, b *bot.Bot, chatID int64, text string, eventID int) {
	if len(text) > 200 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❗ Описание не может быть длиннее 200 символов",
		})
		if err != nil {
			return
		}
		return
	}

	event, err := bs.bm.EventsRepo.EventByID(ctx, eventID)
	if err != nil || event == nil {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Событие не найдено",
		})
		if err != nil {
			return
		}
		return
	}

	if event.UserTgID != chatID {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ У вас нет доступа к этому событию",
		})
		if err != nil {
			return
		}
		return
	}

	event.Message = text
	_, err = bs.bm.EventsRepo.UpdateEvent(ctx, event, db.WithColumns(db.Columns.Event.Message))
	if err != nil {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Ошибка при обновлении события",
		})
		if err != nil {
			return
		}
		return
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "✅ Описание изменено!",
	})
	if err != nil {
		return
	}
}
