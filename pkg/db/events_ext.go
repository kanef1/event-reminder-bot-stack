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

	search.With("periodicity IS NOT NULL")

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
