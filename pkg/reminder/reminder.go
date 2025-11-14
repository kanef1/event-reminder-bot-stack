package reminder

import (
	"context"
	"slices"
	"sync"
	"time"

	"event-reminder-bot/pkg/db"
	"event-reminder-bot/pkg/model"

	"github.com/vmkteam/embedlog"
)

type ReminderManager struct {
	embedlog.Logger
	bm         BotMessenger
	eventsRepo db.EventsRepo
	cancels    map[int]context.CancelFunc
	mu         sync.RWMutex
}

type BotMessenger interface {
	SendReminder(ctx context.Context, chatID int64, text string)
}

func NewReminderManager(bm BotMessenger, eventsRepo db.EventsRepo, logger embedlog.Logger) *ReminderManager {
	return &ReminderManager{
		bm:         bm,
		eventsRepo: eventsRepo,
		cancels:    make(map[int]context.CancelFunc),
		Logger:     logger,
	}
}

func (rm *ReminderManager) CalculateNextTime(e model.ReminderEvent) *time.Time {
	if e.Periodicity == nil {
		return nil
	}

	currentTime := e.DateTime

	switch *e.Periodicity {
	case "hour":
		t := currentTime.Add(time.Minute)
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

func (rm *ReminderManager) ScheduleReminder(parentCtx context.Context, e model.ReminderEvent) context.CancelFunc {
	now := time.Now()
	duration := e.DateTime.Sub(now)

	if duration <= 0 {
		if e.Periodicity != nil {
			nextTime := rm.CalculateNextTime(e)
			if nextTime != nil {
				rm.updateEventTime(parentCtx, e.ID, *nextTime)
				newEvent := e
				newEvent.DateTime = *nextTime
				return rm.ScheduleReminder(parentCtx, newEvent)
			}
		}
		return nil
	}

	childCtx, cancel := context.WithCancel(parentCtx)

	rm.mu.Lock()
	rm.cancels[e.ID] = cancel
	rm.mu.Unlock()

	go func(pCtx context.Context, cCtx context.Context, ev model.ReminderEvent, d time.Duration) {
		defer func() {
			rm.mu.Lock()
			delete(rm.cancels, ev.ID)
			rm.mu.Unlock()
			cancel()
		}()

		select {
		case <-time.After(d):
			dbEvent, err := rm.eventsRepo.EventByID(pCtx, ev.ID)
			if err != nil {
				return
			}

			if dbEvent != nil && dbEvent.StatusID == db.StatusEnabled {
				rm.bm.SendReminder(pCtx, ev.ChatID, ev.Text)

				if dbEvent.Periodicity != nil {
					reminderEvent := model.NewReminderEvent(dbEvent)
					nextTime := rm.CalculateNextTime(reminderEvent)
					if nextTime != nil {
						rm.updateEventTime(pCtx, ev.ID, *nextTime)
						newEvent := reminderEvent
						newEvent.DateTime = *nextTime
						rm.ScheduleReminder(pCtx, newEvent)
					} else {
						_, err := rm.eventsRepo.DeleteEvent(pCtx, ev.ID)
						if err != nil {
							rm.Errorf("Ошибка деактивации события %d: %v", ev.ID, err)
						}
					}
				} else {
					_, err := rm.eventsRepo.DeleteEvent(pCtx, ev.ID)
					if err != nil {
						rm.Errorf("Ошибка деактивации события %d: %v", ev.ID, err)
					}
				}
			}
		case <-cCtx.Done():
			return
		}
	}(parentCtx, childCtx, e, duration)

	return cancel
}

func (rm *ReminderManager) updateEventTime(ctx context.Context, eventID int, newTime time.Time) {
	dbEvent := &db.Event{
		ID:     eventID,
		SendAt: newTime,
	}
	_, err := rm.eventsRepo.UpdateEvent(ctx, dbEvent, db.WithColumns("sendAt"))
	if err != nil {
		rm.Errorf("Ошибка обновления времени события %d: %v", eventID, err)
	}
}

func NewEvent(e *model.Event) model.ReminderEvent {
	return model.ReminderEvent{
		ID:          e.ID,
		OriginalID:  e.OriginalID,
		ChatID:      e.ChatID,
		Text:        e.Text,
		DateTime:    e.DateTime,
		Weekdays:    e.Weekdays,
		Periodicity: e.Periodicity,
	}
}
