package app

import (
	"context"

	"github.com/vmkteam/cron"
)

const (
	reminderSchedule = "* * * * *"
	dailySchedule    = "0 8 * * *"
)

var finished = true

func (a *App) registerCron(ctx context.Context) {
	m := cron.NewManager()

	m.AddFunc("process-reminders", reminderSchedule, func(ctx context.Context) error {
		if a.rm != nil && finished {
			finished = false
			err := a.rm.ProcessReminders(ctx)
			finished = true
			return err
		}
		return nil
	})

	m.AddFunc("daily-events", dailySchedule, func(ctx context.Context) error {
		if a.bm != nil {
			a.bm.SendDailyEvents(ctx)
		}
		return nil
	})

	go func() {
		if err := m.Run(ctx); err != nil {
			a.Errorf("cron error: %v", err)
		}
	}()
}
