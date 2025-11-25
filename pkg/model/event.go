package model

import (
	"time"

	"event-reminder-bot/pkg/db"
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

type ReminderEvent struct {
	ID          int
	OriginalID  int
	ChatID      int64
	Text        string
	DateTime    time.Time
	Weekdays    []int
	Periodicity *string
}

func NewEvent(dbEvent *db.Event) *Event {
	if dbEvent == nil {
		return nil
	}
	return &Event{
		ID:          dbEvent.ID,
		OriginalID:  dbEvent.ID,
		ChatID:      dbEvent.UserTgID,
		Text:        dbEvent.Message,
		DateTime:    dbEvent.SendAt,
		Weekdays:    dbEvent.Weekdays,
		Periodicity: dbEvent.Periodicity,
	}
}

func NewEvents(dbEvents []db.Event) []Event {
	events := make([]Event, len(dbEvents))
	for i, dbEvent := range dbEvents {
		events[i] = Event{
			ID:          dbEvent.ID,
			OriginalID:  dbEvent.ID,
			ChatID:      dbEvent.UserTgID,
			Text:        dbEvent.Message,
			DateTime:    dbEvent.SendAt,
			Weekdays:    dbEvent.Weekdays,
			Periodicity: dbEvent.Periodicity,
		}
	}
	return events
}

func NewReminderEvent(dbEvent *db.Event) ReminderEvent {
	return ReminderEvent{
		ID:          dbEvent.ID,
		OriginalID:  dbEvent.ID,
		ChatID:      dbEvent.UserTgID,
		Text:        dbEvent.Message,
		DateTime:    dbEvent.SendAt,
		Weekdays:    dbEvent.Weekdays,
		Periodicity: dbEvent.Periodicity,
	}
}

func ToDB(event *Event) ReminderEvent {
	return ReminderEvent{
		ID:          event.ID,
		OriginalID:  event.OriginalID,
		ChatID:      event.ChatID,
		Text:        event.Text,
		DateTime:    event.DateTime,
		Weekdays:    event.Weekdays,
		Periodicity: event.Periodicity,
	}
}
