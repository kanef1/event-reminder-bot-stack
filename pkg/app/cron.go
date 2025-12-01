package app

import (
	"context"

	"github.com/vmkteam/cron"
)

func (a *App) registerCron(ctx context.Context) {
	m := cron.NewManager()

	m.AddFunc("daily-events", "0 8 * * *", func(ctx context.Context) error {
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
