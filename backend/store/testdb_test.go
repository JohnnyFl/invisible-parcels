package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"novapost/contract"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://novapost:novapost@localhost:5432/novapost?sslmode=disable"
	}
	s, err := Open(context.Background(), url)
	if err != nil {
		t.Fatalf("open db (run `make db`): %v", err)
	}
	if err := s.Reset(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Pool.Close)
	return s
}

var t0 = contract.ScenarioStart

// scenario2Events: TTN(1) created, its last-leg trip departed and arrived, no unload reported.
func scenario2Events() []contract.Event {
	legs := []contract.RouteLeg{{Seq: 1, FromID: "hub-kyiv", ToID: "branch-kyiv-12", TripID: "trip-1",
		PlannedDeparture: t0.Add(26 * time.Hour), PlannedArrival: t0.Add(28 * time.Hour)}}
	p, _ := json.Marshal(contract.CreatedPayload{RecipientPhone: "+380670000001", OriginID: "hub-kyiv", DestinationID: "branch-kyiv-12", RouteLegs: legs})
	return []contract.Event{
		{EventID: "e1", Kind: contract.KindCreated, TTN: contract.TTN(1), LocationID: "hub-kyiv", OccurredAt: t0, Source: "np", Payload: p},
		{EventID: "e2", Kind: contract.KindTripDeparted, TripID: "trip-1", LocationID: "hub-kyiv", OccurredAt: t0.Add(time.Hour), Source: "np"},
		{EventID: "e3", Kind: contract.KindTripArrived, TripID: "trip-1", LocationID: "branch-kyiv-12", OccurredAt: t0.Add(4 * time.Hour), Source: "np"},
	}
}
