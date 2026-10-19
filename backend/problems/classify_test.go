package problems

import (
	"slices"
	"testing"
	"time"

	"novapost/contract"
)

var t0 = time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)

func trip(id string) func(*contract.Event) { return func(e *contract.Event) { e.TripID = id } }
func loc(id string) func(*contract.Event)  { return func(e *contract.Event) { e.LocationID = id } }

func ev(kind string, at time.Duration, mod ...func(*contract.Event)) contract.Event {
	e := contract.Event{Kind: kind, OccurredAt: t0.Add(at)}
	if kind != contract.KindTripArrived && kind != contract.KindTripDeparted && kind != contract.KindIncident {
		e.TTN = contract.TTN(1)
	}
	for _, m := range mod {
		m(&e)
	}
	return e
}

func kinds(ps []Problem) []Kind {
	out := make([]Kind, len(ps))
	for i, p := range ps {
		out[i] = p.Kind
	}
	slices.Sort(out)
	return out
}

func TestClassify(t *testing.T) {
	leg := contract.RouteLeg{Seq: 1, FromID: "hub-kyiv", ToID: "branch-kyiv-12", TripID: "trip-1", PlannedArrival: t0.Add(31 * time.Hour)}
	p := &contract.CreatedPayload{OriginID: "hub-kyiv", DestinationID: "branch-kyiv-12", RouteLegs: []contract.RouteLeg{leg}}
	created := ev(contract.KindCreated, 0)
	tripArrived := ev(contract.KindTripArrived, 4*time.Hour, trip("trip-1"), loc("branch-kyiv-12"))
	far := t0.Add(100 * time.Hour)
	in := func(now time.Duration, evs ...contract.Event) Input {
		return Input{TTN: contract.TTN(1), Parcel: p, Legs: p.RouteLegs, Events: evs, Now: t0.Add(now)}
	}

	cases := []struct {
		name  string
		in    Input
		want  Severity
		kinds []Kind
	}{
		{"normal in transit", in(2*time.Hour, created), Green, nil},
		{"broken chain fresh", in(5*time.Hour, created, tripArrived), Orange, []Kind{BrokenChain}},
		{"broken chain escalated after 6h", in(10*time.Hour+time.Minute, created, tripArrived), Red, []Kind{BrokenChain}},
		{"overdue", in(33*time.Hour+time.Minute, created), Orange, []Kind{Overdue, Stuck}},
		{"stuck", in(12*time.Hour+time.Minute, created), Yellow, []Kind{Stuck}},
		{"stuck escalated to orange after 24h", func() Input { i := in(36*time.Hour+time.Minute, created); i.NPScheduled = &far; return i }(), Orange, []Kind{Stuck}},
		{"incident suppresses stuck", in(20*time.Hour, created, ev(contract.KindIncident, time.Hour, trip("trip-1"))), Grey, []Kind{Incident}},
		{"unknown scan", Input{TTN: contract.TTN(9), Events: []contract.Event{ev(contract.KindOperatorScan, time.Hour, loc("branch-kharkiv-3"))}, Now: t0.Add(2 * time.Hour)}, Red, []Kind{Unknown}},
		{"unknown on hold stays red", Input{TTN: contract.TTN(9), Events: []contract.Event{ev(contract.KindOperatorScan, time.Hour, loc("branch-kharkiv-3")), ev(contract.KindHold, 2*time.Hour, loc("branch-kharkiv-3"))}, Now: t0.Add(3 * time.Hour)}, Red, []Kind{Unknown}},
		{"misrouted", in(5*time.Hour, created, ev(contract.KindUnloaded, 4*time.Hour, loc("branch-lviv-5"))), Red, []Kind{Misrouted}},
		{"misrouted cleared by redirect", in(6*time.Hour, created, ev(contract.KindUnloaded, 4*time.Hour, loc("branch-lviv-5")), ev(contract.KindRedirected, 5*time.Hour, loc("branch-lviv-5"))), Green, nil},
		{"risky return after broken chain", in(6*time.Hour, created, tripArrived, ev(contract.KindReturnInitiated, 5*time.Hour)), Red, []Kind{BrokenChain, RiskyReturn}},
		{"risky return cleared once scanned elsewhere", in(8*time.Hour, created, tripArrived, ev(contract.KindReturnInitiated, 5*time.Hour, loc("branch-kyiv-12")), ev(contract.KindArrivedHub, 7*time.Hour, loc("hub-kyiv"))), Orange, []Kind{BrokenChain}},
		{"unknown parcel return cleared once scanned elsewhere", Input{TTN: contract.TTN(9), Now: t0.Add(5 * time.Hour), Events: []contract.Event{
			ev(contract.KindOperatorScan, time.Hour, loc("branch-kharkiv-3")), ev(contract.KindHold, 2*time.Hour, loc("branch-kharkiv-3")),
			ev(contract.KindReturnInitiated, 3*time.Hour, loc("branch-kharkiv-3")), ev(contract.KindOperatorScan, 4*time.Hour, loc("hub-kharkiv"))}}, Red, []Kind{Unknown}},
		{"plain return is not risky", in(3*time.Hour, created, ev(contract.KindReturnInitiated, 2*time.Hour)), Green, nil},
		{"arrived clears everything", in(40*time.Hour, created, tripArrived, ev(contract.KindUnloaded, 4*time.Hour, loc("branch-kyiv-12"), trip("trip-1"))), Green, nil},
		{"issued clears everything", in(40*time.Hour, created, ev(contract.KindIssued, 3*time.Hour)), Green, nil},
		{"np scheduled date drives overdue", func() Input {
			s := t0.Add(5 * time.Hour)
			i := in(7*time.Hour+time.Minute, created, ev(contract.KindAccepted, 6*time.Hour))
			i.NPScheduled = &s
			return i
		}(), Orange, []Kind{Overdue}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.in)
			if Level(got) != c.want || !slices.Equal(kinds(got), c.kinds) {
				t.Fatalf("level %v kinds %v, want %v %v; problems %+v", Level(got), kinds(got), c.want, c.kinds, got)
			}
		})
	}
}

func TestEscalationKeepsSince(t *testing.T) {
	leg := contract.RouteLeg{Seq: 1, FromID: "hub-kyiv", ToID: "branch-kyiv-12", TripID: "trip-1", PlannedArrival: t0.Add(31 * time.Hour)}
	p := &contract.CreatedPayload{DestinationID: "branch-kyiv-12", RouteLegs: []contract.RouteLeg{leg}}
	got := Classify(Input{Parcel: p, Legs: p.RouteLegs, Now: t0.Add(11 * time.Hour),
		Events: []contract.Event{ev(contract.KindCreated, 0), ev(contract.KindTripArrived, 4*time.Hour, trip("trip-1"))}})
	if len(got) != 1 || !got[0].Since.Equal(t0.Add(4*time.Hour)) {
		t.Fatalf("%+v", got)
	}
}

func TestSeverityJSON(t *testing.T) {
	if Red.String() != "red" || Grey.String() != "grey" || Green.String() != "green" {
		t.Fatal(Red.String())
	}
	if b, _ := Orange.MarshalJSON(); string(b) != `"orange"` {
		t.Fatal(string(b))
	}
}
