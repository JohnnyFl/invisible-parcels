package world

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"novapost/contract"
)

var t0 = time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)

func hiddenUnloadFacts() []Fact {
	ttn := contract.TTN(1)
	legs := []contract.RouteLeg{{Seq: 1, FromID: "hub-kyiv", ToID: "branch-kyiv-12", TripID: "trip-1",
		PlannedDeparture: t0.Add(24 * time.Hour), PlannedArrival: t0.Add(28 * time.Hour)}} // план: завтра 15:00 за Києвом
	created, _ := json.Marshal(contract.CreatedPayload{OriginID: "hub-kyiv", DestinationID: "branch-kyiv-12", RouteLegs: legs})
	return []Fact{
		{Event: contract.Event{EventID: "e1", Kind: contract.KindCreated, TTN: ttn, OccurredAt: t0, Payload: created}},
		{Event: contract.Event{EventID: "e2", Kind: contract.KindTripDeparted, TripID: "trip-1", OccurredAt: t0.Add(1 * time.Hour)}},
		{Event: contract.Event{EventID: "e3", Kind: contract.KindTripArrived, TripID: "trip-1", LocationID: "branch-kyiv-12", OccurredAt: t0.Add(4 * time.Hour)}},
		{Event: contract.Event{EventID: "e4", Kind: contract.KindUnloaded, TTN: ttn, TripID: "trip-1", LocationID: "branch-kyiv-12", OccurredAt: t0.Add(4 * time.Hour)}, Hidden: true},
	}
}

func TestHiddenUnload_ClientSeesForecast_ParcelPhysicallyAtBranch(t *testing.T) {
	ttn := contract.TTN(1)
	w := New()
	w.Load(hiddenUnloadFacts())
	now := t0.Add(6 * time.Hour)
	st, ok := w.Track(ttn, now)
	if !ok || st.StatusCode != contract.StatusInRecipientCity || st.ScheduledDeliveryDate != "18-10-2026 15:00:00" {
		t.Fatalf("%+v", st)
	}
	if st.CityRecipient != "Київ" || st.WarehouseRecipient != "Відділення №12" {
		t.Fatalf("%+v", st)
	}
	if got := w.Physical("branch-kyiv-12", now); !slices.Contains(got, ttn) {
		t.Fatalf("physical: %v", got)
	}
	for _, e := range w.Due(now) {
		if e.EventID == "e4" {
			t.Fatal("hidden event pushed")
		}
	}

	w.Apply(contract.Event{EventID: "op1", Kind: contract.KindArrivedBranch, TTN: ttn, LocationID: "branch-kyiv-12", OccurredAt: now, Source: "operator"})
	if st, _ := w.Track(ttn, now); st.StatusCode != contract.StatusArrived {
		t.Fatalf("after accept: %+v", st)
	}
	for _, e := range w.Due(now.Add(time.Hour)) {
		if e.EventID == "op1" {
			t.Fatal("operator event re-pushed")
		}
	}
}

func TestDue_ReturnsEachEventOnce(t *testing.T) {
	w := New()
	w.Load(hiddenUnloadFacts())
	now := t0.Add(6 * time.Hour)
	if got := w.Due(now); len(got) != 3 || got[0].EventID != "e1" {
		t.Fatalf("first Due: %+v", got)
	}
	if got := w.Due(now); len(got) != 0 {
		t.Fatalf("second Due: %+v", got)
	}
}

func TestDue_OnlyPastEvents(t *testing.T) {
	w := New()
	w.Load(hiddenUnloadFacts())
	if got := w.Due(t0.Add(2 * time.Hour)); len(got) != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestTrack_UnknownOrNotYetCreated(t *testing.T) {
	w := New()
	w.Load(hiddenUnloadFacts())
	if _, ok := w.Track(contract.TTN(999), t0.Add(time.Hour)); ok {
		t.Fatal("unknown ttn tracked")
	}
	if _, ok := w.Track(contract.TTN(1), t0.Add(-time.Hour)); ok {
		t.Fatal("tracked before created")
	}
}

func TestTrack_InTransitAfterDeparture(t *testing.T) {
	w := New()
	w.Load(hiddenUnloadFacts())
	if st, _ := w.Track(contract.TTN(1), t0.Add(2*time.Hour)); st.StatusCode != contract.StatusInTransit {
		t.Fatalf("%+v", st)
	}
}

func TestPhysical_NoDuplicates(t *testing.T) {
	w := New()
	w.Load(hiddenUnloadFacts())
	got := w.Physical("branch-kyiv-12", t0.Add(6*time.Hour))
	if len(got) != 1 {
		t.Fatalf("%v", got)
	}
}
