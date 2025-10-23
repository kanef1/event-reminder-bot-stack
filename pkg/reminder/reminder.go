package reminder

import (
	"context"
	"log"
	"sync"
	"time"

	"event-reminder-bot/pkg/db"
	botManager "event-reminder-bot/pkg/event-reminder-bot"
)

type Event struct {
	ID          int
	OriginalID  int
	ChatID      int64
	Text        string
	DateTime    time.Time
	Weekdays    []int
	Periodicity *string
}

type ReminderManager struct {
	bm         *botManager.BotManager
	eventsRepo db.EventsRepo
	cancels    map[int]context.CancelFunc
	mu         sync.RWMutex
}

func NewReminderManager(bm *botManager.BotManager, eventsRepo db.EventsRepo) *ReminderManager {
	return &ReminderManager{
		bm:         bm,
		eventsRepo: eventsRepo,
		cancels:    make(map[int]context.CancelFunc),
	}
}

func (rm *ReminderManager) calculateNextTime(e Event) time.Time {
	if e.Periodicity == nil {
		return time.Time{}
	}

	currentTime := e.DateTime

	switch *e.Periodicity {
	case "hour":
		return currentTime.Add(time.Minute)
	case "day":
		return currentTime.Add(24 * time.Hour)
	case "week":
		return currentTime.Add(7 * 24 * time.Hour)
	case "weekdays":
		return rm.calculateNextWeekday(currentTime, e.Weekdays)
	default:
		return time.Time{}
	}
}

func (rm *ReminderManager) calculateNextWeekday(currentTime time.Time, weekdays []int) time.Time {
	if len(weekdays) == 0 {
		return time.Time{}
	}

	nextTime := currentTime.Add(24 * time.Hour)

	for i := 1; i < 8; i++ {
		weekday := int(nextTime.Weekday())
		if weekday == 0 {
			weekday = 7
		} else {
			weekday = weekday - 1
		}

		if contains(weekdays, weekday) {
			return time.Date(
				nextTime.Year(), nextTime.Month(), nextTime.Day(),
				currentTime.Hour(), currentTime.Minute(), 0, 0, currentTime.Location(),
			)
		}
		nextTime = nextTime.Add(24 * time.Hour)
	}

	return time.Time{}
}

func (rm *ReminderManager) ScheduleReminder(parentCtx context.Context, e Event) context.CancelFunc {
	now := time.Now()

	duration := e.DateTime.Sub(now)

	if duration <= 0 {
		if e.Periodicity != nil {
			nextTime := rm.calculateNextTime(e)
			if !nextTime.IsZero() {
				rm.updateEventTime(parentCtx, e.ID, nextTime)
				newEvent := e
				newEvent.DateTime = nextTime
				return rm.ScheduleReminder(parentCtx, newEvent)
			}
		}
		return nil
	}

	childCtx, cancel := context.WithCancel(parentCtx)

	rm.mu.Lock()
	rm.cancels[e.ID] = cancel
	rm.mu.Unlock()

	go func(pCtx context.Context, cCtx context.Context, ev Event, d time.Duration) {
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
					reminderEvent := convertDBToReminderEvent(*dbEvent)
					nextTime := rm.calculateNextTime(reminderEvent)
					if !nextTime.IsZero() {
						rm.updateEventTime(pCtx, ev.ID, nextTime)
						newEvent := reminderEvent
						newEvent.DateTime = nextTime
						rm.ScheduleReminder(pCtx, newEvent)
					} else {
						rm.deactivateEvent(pCtx, ev.ID)
					}
				} else {
					rm.deactivateEvent(pCtx, ev.ID)
				}
			}
		case <-cCtx.Done():
			return
		}
	}(parentCtx, childCtx, e, duration)

	return cancel
}

func convertDBToReminderEvent(dbEvent db.Event) Event {
	return Event{
		ID:          dbEvent.ID,
		OriginalID:  dbEvent.ID,
		ChatID:      dbEvent.UserTgID,
		Text:        dbEvent.Message,
		DateTime:    dbEvent.SendAt,
		Weekdays:    dbEvent.Weekdays,
		Periodicity: dbEvent.Periodicity,
	}
}
func (rm *ReminderManager) deactivateEvent(ctx context.Context, eventID int) {
	dbEvent := &db.Event{
		ID:       eventID,
		StatusID: db.StatusDeleted,
	}
	_, err := rm.eventsRepo.UpdateEvent(ctx, dbEvent, db.WithColumns("statusId"))
	if err != nil {
		log.Printf("Ошибка деактивации события %d: %v", eventID, err)
	}
}

func (rm *ReminderManager) updateEventTime(ctx context.Context, eventID int, newTime time.Time) {
	dbEvent := &db.Event{
		ID:     eventID,
		SendAt: newTime,
	}
	_, err := rm.eventsRepo.UpdateEvent(ctx, dbEvent, db.WithColumns("sendAt"))
	if err != nil {
		log.Printf("Ошибка обновления времени события %d: %v", eventID, err)
	}
}

func contains(slice []int, item int) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func (rm *ReminderManager) CalculateNextTime(e Event) time.Time {
	return rm.calculateNextTime(e)
}
