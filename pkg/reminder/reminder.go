package reminder

import (
	"context"
	"slices"
	"time"

	"event-reminder-bot/pkg/db"
	"event-reminder-bot/pkg/model"

	"github.com/vmkteam/embedlog"
)

type ReminderManager struct {
	embedlog.Logger
	bm         BotMessenger
	eventsRepo db.EventsRepo
}

type BotMessenger interface {
	SendReminder(ctx context.Context, chatID int64, text string, eventID int)
	SendReminderPeriodicity(ctx context.Context, id int64, text string)
}

func NewReminderManager(bm BotMessenger, eventsRepo db.EventsRepo, logger embedlog.Logger) *ReminderManager {
	return &ReminderManager{
		bm:         bm,
		eventsRepo: eventsRepo,
		Logger:     logger,
	}
}

func (rm *ReminderManager) ProcessReminders(ctx context.Context) error {
	events, err := rm.eventsRepo.EventsToSend(ctx)
	if err != nil {
		rm.Errorf("Ошибка получения событий для отправки: %v", err)
		return err
	}

	for _, event := range events {
		rm.processEvent(ctx, &event)
	}

	return nil
}

func (rm *ReminderManager) processEvent(ctx context.Context, event *db.Event) {
	if event.Periodicity != nil {
		rm.bm.SendReminderPeriodicity(ctx, event.UserTgID, event.Message)

		reminderEvent := model.NewReminderEvent(event)
		nextTime := rm.CalculateNextTime(reminderEvent)

		if nextTime != nil {
			event.SendAt = *nextTime
			_, err := rm.eventsRepo.UpdateEvent(ctx, event, db.WithColumns("sendAt"))
			if err != nil {
				rm.Errorf("Ошибка обновления времени события %d: %v", event.ID, err)
			}
		} else {
			_, err := rm.eventsRepo.DeleteEvent(ctx, event.ID)
			if err != nil {
				rm.Errorf("Ошибка деактивации события %d: %v", event.ID, err)
			}
		}
	} else {
		rm.bm.SendReminder(ctx, event.UserTgID, event.Message, event.ID)
	}
}

func (rm *ReminderManager) CalculateNextTime(e model.ReminderEvent) *time.Time {
	if e.Periodicity == nil {
		return nil
	}

	currentTime := e.DateTime

	switch *e.Periodicity {
	case "hour":
		t := currentTime.Add(time.Hour)
		return &t
	case "day":
		t := currentTime.Add(24 * time.Hour)
		return &t
	case "week":
		t := currentTime.Add(7 * 24 * time.Hour)
		return &t
	case "weekdays":
		return rm.calculateNextWeekday(currentTime, e.Weekdays)
	default:
		return nil
	}
}

func (rm *ReminderManager) calculateNextWeekday(currentTime time.Time, weekdays []int) *time.Time {
	if len(weekdays) == 0 {
		return nil
	}

	nextTime := currentTime.Add(24 * time.Hour)

	for i := 1; i < 8; i++ {
		weekday := int(nextTime.Weekday())
		if weekday == 0 {
			weekday = 7
		} else {
			weekday = weekday - 1
		}

		if slices.Contains(weekdays, weekday) {
			t := time.Date(
				nextTime.Year(), nextTime.Month(), nextTime.Day(),
				currentTime.Hour(), currentTime.Minute(), 0, 0, currentTime.Location(),
			)
			return &t
		}
		nextTime = nextTime.Add(24 * time.Hour)
	}

	return nil
}
