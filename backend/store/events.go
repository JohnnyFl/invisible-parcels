package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"novapost/contract"
)

// InsertEvents appends events idempotently (duplicate event_id is ignored) and
// materializes parcels / route_legs from new "created" events.
func (s *Store) InsertEvents(ctx context.Context, evs []contract.Event, receivedAt time.Time) (int, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	inserted := 0
	for _, e := range evs {
		var payload any
		if len(e.Payload) > 0 {
			payload = string(e.Payload)
		}
		tag, err := tx.Exec(ctx, `INSERT INTO events (event_id, kind, ttn, trip_id, location_id, occurred_at, received_at, source, payload)
			VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,''), $6, $7, $8, $9::jsonb)
			ON CONFLICT (event_id) DO NOTHING`,
			e.EventID, e.Kind, e.TTN, e.TripID, e.LocationID, e.OccurredAt, receivedAt, e.Source, payload)
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		inserted++
		if e.Kind == contract.KindCreated && e.TTN != "" {
			if err := materialize(ctx, tx, e); err != nil {
				return 0, err
			}
		}
	}
	return inserted, tx.Commit(ctx)
}

func materialize(ctx context.Context, tx pgx.Tx, e contract.Event) error {
	var p contract.CreatedPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO parcels (ttn, sender_phone, recipient_phone, origin_id, destination_id, created_at)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (ttn) DO NOTHING`,
		e.TTN, p.SenderPhone, p.RecipientPhone, p.OriginID, p.DestinationID, e.OccurredAt); err != nil {
		return err
	}
	for _, l := range p.RouteLegs {
		if _, err := tx.Exec(ctx, `INSERT INTO route_legs (ttn, seq, from_id, to_id, trip_id, planned_departure, planned_arrival)
			VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (ttn, seq) DO NOTHING`,
			e.TTN, l.Seq, l.FromID, l.ToID, l.TripID, l.PlannedDeparture, l.PlannedArrival); err != nil {
			return err
		}
	}
	return nil
}

const eventCols = `event_id, kind, COALESCE(ttn,''), COALESCE(trip_id,''), COALESCE(location_id,''), occurred_at, source, payload`

func scanEvents(rows pgx.Rows) ([]contract.Event, error) {
	defer rows.Close()
	var out []contract.Event
	for rows.Next() {
		var e contract.Event
		var payload []byte
		if err := rows.Scan(&e.EventID, &e.Kind, &e.TTN, &e.TripID, &e.LocationID, &e.OccurredAt, &e.Source, &payload); err != nil {
			return nil, err
		}
		if payload != nil {
			e.Payload = payload
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ParcelEvents returns the parcel's own events, events of the trips in its
// plan, and incidents on those trips or at places it has been — by occurred_at.
func (s *Store) ParcelEvents(ctx context.Context, ttn string) ([]contract.Event, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+eventCols+` FROM events
		WHERE ttn = $1
		   OR (ttn IS NULL AND trip_id IN (SELECT trip_id FROM route_legs WHERE ttn = $1))
		   OR (kind = 'incident' AND location_id IN (SELECT location_id FROM events WHERE ttn = $1 AND location_id IS NOT NULL))
		ORDER BY occurred_at, received_at, event_id`, ttn)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// Parcel returns the materialized parcel and its route plan; nil if unknown.
func (s *Store) Parcel(ctx context.Context, ttn string) (*contract.CreatedPayload, []contract.RouteLeg, error) {
	var p contract.CreatedPayload
	err := s.Pool.QueryRow(ctx, `SELECT sender_phone, recipient_phone, origin_id, destination_id FROM parcels WHERE ttn = $1`, ttn).
		Scan(&p.SenderPhone, &p.RecipientPhone, &p.OriginID, &p.DestinationID)
	if err == pgx.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT seq, from_id, to_id, trip_id, planned_departure, planned_arrival
		FROM route_legs WHERE ttn = $1 ORDER BY seq`, ttn)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l contract.RouteLeg
		if err := rows.Scan(&l.Seq, &l.FromID, &l.ToID, &l.TripID, &l.PlannedDeparture, &l.PlannedArrival); err != nil {
			return nil, nil, err
		}
		p.RouteLegs = append(p.RouteLegs, l)
	}
	return &p, p.RouteLegs, rows.Err()
}

// OperatorEvents returns every operator action in journal order.
func (s *Store) OperatorEvents(ctx context.Context) ([]contract.Event, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+eventCols+` FROM events WHERE source = 'operator'
		ORDER BY occurred_at, received_at, event_id`)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}
