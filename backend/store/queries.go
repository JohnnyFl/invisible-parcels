package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"novapost/contract"
)

// UnissuedTTNs lists parcels that have no "issued" event yet.
func (s *Store) UnissuedTTNs(ctx context.Context) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT ttn FROM parcels p
		WHERE NOT EXISTS (SELECT 1 FROM events e WHERE e.ttn = p.ttn AND e.kind = 'issued') ORDER BY ttn`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// SaveNPStatus upserts the client-side snapshot from getStatusDocuments.
func (s *Store) SaveNPStatus(ctx context.Context, sts []contract.TrackingStatus, fetchedAt time.Time) error {
	batch := &pgx.Batch{}
	for _, st := range sts {
		var sched *time.Time
		if t, err := time.ParseInLocation(contract.NPDateLayout, st.ScheduledDeliveryDate, contract.Kyiv); err == nil {
			sched = &t
		}
		batch.Queue(`INSERT INTO np_status (ttn, status_code, status_text, scheduled_delivery, fetched_at) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (ttn) DO UPDATE SET status_code=$2, status_text=$3, scheduled_delivery=$4, fetched_at=$5`,
			st.Number, st.StatusCode, st.Status, sched, fetchedAt)
	}
	return s.Pool.SendBatch(ctx, batch).Close()
}

func (s *Store) NPStatus(ctx context.Context, ttn string) (code, text string, scheduled *time.Time, ok bool, err error) {
	err = s.Pool.QueryRow(ctx, `SELECT status_code, status_text, scheduled_delivery FROM np_status WHERE ttn = $1`, ttn).
		Scan(&code, &text, &scheduled)
	if err == pgx.ErrNoRows {
		return "", "", nil, false, nil
	}
	return code, text, scheduled, err == nil, err
}
