package problems

import (
	"strings"
	"testing"
	"time"

	"novapost/contract"
)

func traceInput(evs ...contract.Event) Input {
	legs := []contract.RouteLeg{
		{Seq: 1, FromID: "branch-kharkiv-3", ToID: "hub-kharkiv", TripID: "t1", PlannedArrival: t0.Add(4 * time.Hour)},
		{Seq: 2, FromID: "hub-kharkiv", ToID: "hub-kyiv", TripID: "t2", PlannedArrival: t0.Add(16 * time.Hour)},
		{Seq: 3, FromID: "hub-kyiv", ToID: "branch-kyiv-12", TripID: "t3", PlannedArrival: t0.Add(28 * time.Hour)},
	}
	p := &contract.CreatedPayload{OriginID: "branch-kharkiv-3", DestinationID: "branch-kyiv-12", RouteLegs: legs}
	return Input{TTN: contract.TTN(1), Parcel: p, Legs: legs, Events: evs, Now: t0.Add(30 * time.Hour)}
}

func states(tr Trace) string {
	var out []string
	for _, l := range tr.Legs {
		out = append(out, string(l.State))
	}
	return strings.Join(out, ",")
}

// leg emits the full, reported chain of one leg.
func leg(tripID, from, to string, dep, arr time.Duration) []contract.Event {
	return []contract.Event{
		ev(contract.KindLoaded, dep, trip(tripID), loc(from)),
		ev(contract.KindTripDeparted, dep, trip(tripID), loc(from)),
		ev(contract.KindTripArrived, arr, trip(tripID), loc(to)),
		ev(contract.KindUnloaded, arr, trip(tripID), loc(to)),
	}
}

func TestTrace(t *testing.T) {
	accepted := ev(contract.KindAccepted, 0, loc("branch-kharkiv-3"))
	l1 := leg("t1", "branch-kharkiv-3", "hub-kharkiv", 2*time.Hour, 4*time.Hour)
	l2 := leg("t2", "hub-kharkiv", "hub-kyiv", 6*time.Hour, 16*time.Hour)
	l3 := leg("t3", "hub-kyiv", "branch-kyiv-12", 20*time.Hour, 22*time.Hour)
	cat := func(parts ...[]contract.Event) []contract.Event {
		var out []contract.Event
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}

	t.Run("delivered: every leg done, no break", func(t *testing.T) {
		tr := TraceParcel(traceInput(cat([]contract.Event{accepted}, l1, l2, l3)...))
		if states(tr) != "done,done,done" || tr.Break != nil || !tr.Delivered {
			t.Fatalf("%s %+v", states(tr), tr.Break)
		}
	})

	t.Run("in transit: loaded and departed, not arrived yet", func(t *testing.T) {
		tr := TraceParcel(traceInput(cat([]contract.Event{accepted}, l1, l2[:2])...))
		if states(tr) != "done,in_transit,pending" || tr.Break != nil {
			t.Fatalf("%s %+v", states(tr), tr.Break)
		}
	})

	t.Run("trip arrived, unload not confirmed", func(t *testing.T) {
		tr := TraceParcel(traceInput(cat([]contract.Event{accepted}, l1, l2[:3])...))
		if states(tr) != "done,broken,pending" || tr.Break == nil || tr.Break.Seq != 2 {
			t.Fatalf("%s %+v", states(tr), tr.Break)
		}
		if !strings.Contains(tr.Break.Hint, "Сортувальний центр Київ") || !strings.Contains(tr.Break.Hint, "вивантаження") {
			t.Fatal(tr.Break.Hint)
		}
		if tr.LastConfirmed == nil || tr.LastConfirmed.Location != "hub-kharkiv" {
			t.Fatalf("last confirmed %+v", tr.LastConfirmed)
		}
	})

	t.Run("neither load nor unload: parcel is at one end or the other", func(t *testing.T) {
		trip3 := []contract.Event{l3[1], l3[2]} // departed + arrived only
		tr := TraceParcel(traceInput(cat([]contract.Event{accepted}, l1, l2, trip3)...))
		if tr.Break == nil || tr.Break.Seq != 3 {
			t.Fatalf("%+v", tr.Break)
		}
		if !strings.Contains(tr.Break.Hint, "Сортувальний центр Київ") || !strings.Contains(tr.Break.Hint, "Київ, Відділення №12") {
			t.Fatal(tr.Break.Hint)
		}
	})

	t.Run("incident on the trip is not a break", func(t *testing.T) {
		inc := ev(contract.KindIncident, 21*time.Hour, trip("t3"))
		tr := TraceParcel(traceInput(cat([]contract.Event{accepted}, l1, l2, l3[:2], []contract.Event{inc})...))
		if states(tr) != "done,done,incident" || tr.Break != nil {
			t.Fatalf("%s %+v", states(tr), tr.Break)
		}
	})

	t.Run("unknown parcel has no route", func(t *testing.T) {
		tr := TraceParcel(Input{TTN: contract.TTN(9), Events: []contract.Event{ev(contract.KindOperatorScan, time.Hour, loc("branch-kharkiv-3"))}})
		if len(tr.Legs) != 0 || tr.LastConfirmed == nil || tr.LastConfirmed.Location != "branch-kharkiv-3" {
			t.Fatalf("%+v", tr)
		}
	})
}
