package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"novapost/backend/problems"
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

// ponytail: 3 queries per TTN; fine for a demo with hundreds of parcels, batch when it grows.

// Inputs assembles classification inputs for each TTN.
func (s *Store) Inputs(ctx context.Context, ttns []string, now time.Time) ([]problems.Input, error) {
	out := make([]problems.Input, 0, len(ttns))
	for _, ttn := range ttns {
		p, legs, err := s.Parcel(ctx, ttn)
		if err != nil {
			return nil, err
		}
		evs, err := s.ParcelEvents(ctx, ttn)
		if err != nil {
			return nil, err
		}
		_, _, sched, _, err := s.NPStatus(ctx, ttn)
		if err != nil {
			return nil, err
		}
		out = append(out, problems.Input{TTN: ttn, Parcel: p, Legs: legs, Events: evs, NPScheduled: sched, Now: now})
	}
	return out, nil
}

const unknownScans = `SELECT DISTINCT e.ttn FROM events e
	WHERE e.kind = 'operator_scan' AND e.ttn IS NOT NULL AND NOT EXISTS (SELECT 1 FROM parcels p WHERE p.ttn = e.ttn)`

// BranchTTNs: unissued parcels heading to, routed through or last seen at the
// branch, plus TTNs scanned there that the system does not know.
func (s *Store) BranchTTNs(ctx context.Context, branchID string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT ttn FROM parcels p
		WHERE NOT EXISTS (SELECT 1 FROM events e WHERE e.ttn = p.ttn AND e.kind = 'issued')
		  AND (p.destination_id = $1
		    OR EXISTS (SELECT 1 FROM route_legs l WHERE l.ttn = p.ttn AND l.to_id = $1)
		    OR (SELECT e.location_id FROM events e WHERE e.ttn = p.ttn
		          AND e.kind IN ('unloaded', 'arrived_branch', 'operator_scan')
		          ORDER BY e.occurred_at DESC LIMIT 1) = $1)
		UNION `+unknownScans+` AND e.location_id = $1
		ORDER BY 1`, branchID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (s *Store) AllActiveTTNs(ctx context.Context) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT ttn FROM parcels p
		WHERE NOT EXISTS (SELECT 1 FROM events e WHERE e.ttn = p.ttn AND e.kind = 'issued')
		UNION `+unknownScans+` ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

type IncomingRow struct {
	TTN    string    `json:"ttn"`
	FromID string    `json:"from"`
	TripID string    `json:"tripId"`
	ETA    time.Time `json:"eta"`
}

// Incoming: legs ending at the branch that have not been unloaded there yet.
// Without the solution an operator only sees parcels with a "loaded" scan on that trip.
func (s *Store) Incoming(ctx context.Context, branchID string, withSolution bool) ([]IncomingRow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT l.ttn, l.from_id, l.trip_id, COALESCE(n.scheduled_delivery, l.planned_arrival)
		FROM route_legs l LEFT JOIN np_status n ON n.ttn = l.ttn
		WHERE l.to_id = $1
		  AND NOT EXISTS (SELECT 1 FROM events e WHERE e.ttn = l.ttn AND e.location_id = $1 AND e.kind IN ('unloaded', 'arrived_branch'))
		  AND NOT EXISTS (SELECT 1 FROM events e WHERE e.ttn = l.ttn AND e.kind IN ('issued', 'redirected', 'return_initiated'))
		  AND ($2 OR EXISTS (SELECT 1 FROM events e WHERE e.ttn = l.ttn AND e.kind = 'loaded' AND e.trip_id = l.trip_id))
		ORDER BY 4, 1`, branchID, withSolution)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (IncomingRow, error) {
		var x IncomingRow
		err := r.Scan(&x.TTN, &x.FromID, &x.TripID, &x.ETA)
		return x, err
	})
}

func (s *Store) Audit(ctx context.Context, at time.Time, role, branchID, action, ttn, result string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO audit_log (at, role, branch_id, action, ttn, result) VALUES ($1,$2,$3,$4,$5,$6)`,
		at, role, branchID, action, ttn, result)
	return err
}

// RegisteredAt returns which of ttns the system already knows arrived at the branch.
func (s *Store) RegisteredAt(ctx context.Context, branchID string, ttns []string) (map[string]bool, error) {
	rows, err := s.Pool.Query(ctx, `SELECT DISTINCT ttn FROM events
		WHERE location_id = $1 AND ttn = ANY($2) AND kind IN ('unloaded', 'arrived_branch')`, branchID, ttns)
	if err != nil {
		return nil, err
	}
	known, err := pgx.CollectRows(rows, pgx.RowTo[string])
	out := make(map[string]bool, len(known))
	for _, t := range known {
		out[t] = true
	}
	return out, err
}
