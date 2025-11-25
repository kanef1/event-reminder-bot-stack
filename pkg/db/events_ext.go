package db

import (
	"context"
	"fmt"
)

func (er EventsRepo) CountUserPeriodicEvents(ctx context.Context, userTgID int64) (int, error) {
	StatusEnabled := StatusEnabled
	search := &EventSearch{
		UserTgID: &userTgID,
		StatusID: &StatusEnabled,
	}

	search.WithPeriodicityNotNull()

	count, err := er.CountEvents(ctx, search)
	if err != nil {
		return 0, fmt.Errorf("ошибка подсчета периодических событий: %w", err)
	}

	return count, nil
}

func (er EventsRepo) CleanupPastEvents(ctx context.Context) error {
	_, err := er.db.ExecContext(ctx,
		`UPDATE events SET "statusId" = ? WHERE "sendAt" < NOW() AND "statusId" = ? AND periodicity IS NULL`,
		StatusDeleted, StatusEnabled)
	return err
}

func (es *EventSearch) WithPeriodicityNotNull() *EventSearch {
	es.With("periodicity IS NOT NULL")
	return es
}

func (r *EventsRepo) AllUsersWithEventsToday(ctx context.Context) ([]int64, error) {
	var users []int64

	query := `
        SELECT DISTINCT "userTgId"
        FROM "events"
        WHERE "statusId" = 1
          AND DATE("sendAt" AT TIME ZONE 'Europe/Moscow') = DATE(NOW() AT TIME ZONE 'Europe/Moscow')
    `

	_, err := r.db.QueryContext(ctx, &users, query)
	if err != nil {
		return nil, err
	}

	return users, nil
}
