package store

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"novapost/contract"
)

func ids(evs []contract.Event) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.EventID
	}
	return out
}

func TestInsertEvents_IdempotentAndOrderIndependent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	evs := scenario2Events()
	n, err := s.InsertEvents(ctx, evs, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	again, _ := s.InsertEvents(ctx, evs, time.Now())
	if n != len(evs) || again != 0 {
		t.Fatalf("n=%d again=%d", n, again)
	}
	got1, _ := s.ParcelEvents(ctx, contract.TTN(1))

	s.Reset(ctx)
	rev := slices.Clone(evs)
	slices.Reverse(rev)
	s.InsertEvents(ctx, rev, time.Now())
	got2, _ := s.ParcelEvents(ctx, contract.TTN(1))
	if len(got1) != 3 || !reflect.DeepEqual(ids(got1), ids(got2)) {
		t.Fatalf("order-dependent state: %v vs %v", ids(got1), ids(got2))
	}
	p, legs, _ := s.Parcel(ctx, contract.TTN(1))
	if p == nil || len(legs) != 1 || p.DestinationID != "branch-kyiv-12" || legs[0].TripID != "trip-1" {
		t.Fatalf("parcel not materialized: %+v %+v", p, legs)
	}
	if !legs[0].PlannedArrival.Equal(t0.Add(28 * time.Hour)) {
		t.Fatalf("planned arrival %v", legs[0].PlannedArrival)
	}
}

func TestParcelEvents_IncludesIncidentOnItsTrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	evs := append(scenario2Events(),
		contract.Event{EventID: "i1", Kind: contract.KindIncident, TripID: "trip-1", OccurredAt: t0.Add(2 * time.Hour), Source: "np"},
		contract.Event{EventID: "other", Kind: contract.KindTripArrived, TripID: "trip-x", OccurredAt: t0, Source: "np"})
	s.InsertEvents(ctx, evs, time.Now())
	got, _ := s.ParcelEvents(ctx, contract.TTN(1))
	if !slices.Contains(ids(got), "i1") || slices.Contains(ids(got), "other") {
		t.Fatalf("%v", ids(got))
	}
}

func TestParcel_Unknown(t *testing.T) {
	s := testStore(t)
	if p, _, err := s.Parcel(context.Background(), contract.TTN(9)); p != nil || err != nil {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestParcelEvents_SameVirtualTimeKeepsArrivalOrder(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	at := t0.Add(time.Hour) // paused clock: identical virtual time
	base := time.Now()
	for i, e := range []contract.Event{
		{EventID: "zz-scan", Kind: contract.KindOperatorScan},
		{EventID: "mm-hold", Kind: contract.KindHold},
		{EventID: "aa-return", Kind: contract.KindReturnInitiated},
	} {
		e.TTN, e.OccurredAt, e.Source = contract.TTN(9), at, "operator"
		s.InsertEvents(ctx, []contract.Event{e}, base.Add(time.Duration(i)*time.Millisecond))
	}
	got, _ := s.ParcelEvents(ctx, contract.TTN(9))
	if !reflect.DeepEqual(ids(got), []string{"zz-scan", "mm-hold", "aa-return"}) {
		t.Fatalf("%v", ids(got))
	}
}
