// Package scenarios builds the demo worlds: 25 normal background parcels plus
// a focus parcel with one fault (spec §12).
package scenarios

import (
	"encoding/json"
	"fmt"
	"time"

	"novapost/contract"
	"novapost/emulator/world"
)

const FocusTTN = "59000000000001"

var Start = contract.ScenarioStart

var hubOf = map[string]string{
	"branch-kyiv-12": "hub-kyiv", "branch-lviv-5": "hub-lviv", "branch-kharkiv-3": "hub-kharkiv",
}

var branches = []string{"branch-kyiv-12", "branch-lviv-5", "branch-kharkiv-3"}

type builder struct {
	id    int
	facts []world.Fact
}

func (b *builder) add(e contract.Event, hidden bool) {
	e.EventID = fmt.Sprintf("s%d-e%d", b.id, len(b.facts))
	e.Source = "np"
	b.facts = append(b.facts, world.Fact{Event: e, Hidden: hidden})
}

func at(h float64) time.Time { return Start.Add(time.Duration(h * float64(time.Hour))) }

// trip is one vehicle run with actual times (plan may differ).
type trip struct {
	id, from, to string
	dep, arr     float64 // actual, hours from Start
}

func (b *builder) runTrip(t trip, arrives bool) {
	b.add(contract.Event{Kind: contract.KindTripDeparted, TripID: t.id, LocationID: t.from, OccurredAt: at(t.dep)}, false)
	if arrives {
		b.add(contract.Event{Kind: contract.KindTripArrived, TripID: t.id, LocationID: t.to, OccurredAt: at(t.arr)}, false)
	}
}

// parcel emits created with a route plan built from trips; planned times are given per leg.
func (b *builder) created(ttn, origin, dest string, legs []contract.RouteLeg, hidden bool) {
	p, _ := json.Marshal(contract.CreatedPayload{
		SenderPhone:    "+38050" + ttn[len(ttn)-7:],
		RecipientPhone: "+38067" + ttn[len(ttn)-7:],
		OriginID:       origin, DestinationID: dest, RouteLegs: legs,
	})
	b.add(contract.Event{Kind: contract.KindCreated, TTN: ttn, LocationID: origin, OccurredAt: at(0), Payload: p}, hidden)
}

func legsFor(trips []trip, planned [][2]float64) []contract.RouteLeg {
	out := make([]contract.RouteLeg, len(trips))
	for i, t := range trips {
		out[i] = contract.RouteLeg{Seq: i + 1, FromID: t.from, ToID: t.to, TripID: t.id,
			PlannedDeparture: at(planned[i][0]), PlannedArrival: at(planned[i][1])}
	}
	return out
}

// moveAll emits the normal scan chain of one parcel along trips (all reported).
func (b *builder) moveAll(ttn string, trips []trip) {
	b.add(contract.Event{Kind: contract.KindAccepted, TTN: ttn, LocationID: trips[0].from, OccurredAt: at(0.5)}, false)
	for _, t := range trips {
		b.moveLeg(ttn, t, false, false)
	}
}

func (b *builder) moveLeg(ttn string, t trip, hideLoad, hideUnload bool) {
	b.add(contract.Event{Kind: contract.KindLoaded, TTN: ttn, TripID: t.id, LocationID: t.from, OccurredAt: at(t.dep - 0.25)}, hideLoad)
	b.add(contract.Event{Kind: contract.KindUnloaded, TTN: ttn, TripID: t.id, LocationID: t.to, OccurredAt: at(t.arr)}, hideUnload)
	if _, isBranch := hubOf[t.to]; isBranch {
		b.add(contract.Event{Kind: contract.KindArrivedBranch, TTN: ttn, LocationID: t.to, OccurredAt: at(t.arr + 0.2)}, hideUnload)
	} else {
		b.add(contract.Event{Kind: contract.KindArrivedHub, TTN: ttn, LocationID: t.to, OccurredAt: at(t.arr + 0.1)}, hideUnload)
		b.add(contract.Event{Kind: contract.KindSorted, TTN: ttn, LocationID: t.to, OccurredAt: at(t.arr + 1)}, hideUnload)
	}
}

func route(id string, origin, dest string, offset float64) []trip {
	return []trip{
		{id + "-1", origin, hubOf[origin], 2 + offset, 4 + offset},
		{id + "-2", hubOf[origin], hubOf[dest], 6 + offset, 16 + offset}, // < StuckAfter (12h), or parcels look stuck mid-way
		{id + "-3", hubOf[dest], dest, 20 + offset, 22 + offset},
	}
}

func onTime(trips []trip) [][2]float64 {
	out := make([][2]float64, len(trips))
	for i, t := range trips {
		out[i] = [2]float64{t.dep, t.arr}
	}
	return out
}

// background: 25 parcels TTN(100..124) between the three cities, grouped per city pair, all on time.
func (b *builder) background() {
	for pair := 0; pair < 3; pair++ {
		origin, dest := branches[pair], branches[(pair+1)%3]
		trips := route(fmt.Sprintf("trip-bg%d", pair), origin, dest, float64(pair))
		for _, t := range trips {
			b.runTrip(t, true)
		}
		for i := pair; i < 25; i += 3 {
			ttn := contract.TTN(100 + i)
			b.created(ttn, origin, dest, legsFor(trips, onTime(trips)), false)
			b.moveAll(ttn, trips)
		}
	}
}

// Focus group: TTN(1..4) from Kharkiv №3 to Kyiv №12 on trip-f1..f3.
// The last leg is planned for tomorrow 15:00 Kyiv time, but actually arrives at 22h (early).
var focusTrips = []trip{
	{"trip-f1", "branch-kharkiv-3", "hub-kharkiv", 2, 4},
	{"trip-f2", "hub-kharkiv", "hub-kyiv", 6, 16},
	{"trip-f3", "hub-kyiv", "branch-kyiv-12", 20, 22},
}

var focusPlan = [][2]float64{{2, 4}, {6, 16}, {26, 28}}

// Build returns the facts of scenario id (1..6).
func Build(id int) ([]world.Fact, error) {
	if id < 1 || id > 6 {
		return nil, fmt.Errorf("unknown scenario %d", id)
	}
	b := &builder{id: id}
	b.background()

	legs := legsFor(focusTrips, focusPlan)
	b.runTrip(focusTrips[0], true)
	b.runTrip(focusTrips[1], true)
	b.runTrip(focusTrips[2], id != 5)

	first := 1
	if id == 4 {
		first = 2 // TTN(1) is the unknown parcel, not part of the group
	}
	for n := first; n <= 4; n++ {
		ttn := contract.TTN(n)
		b.created(ttn, "branch-kharkiv-3", "branch-kyiv-12", legs, false)
		b.add(contract.Event{Kind: contract.KindAccepted, TTN: ttn, LocationID: "branch-kharkiv-3", OccurredAt: at(0.5)}, false)
		b.moveLeg(ttn, focusTrips[0], false, false)
		if id == 6 {
			// the Kyiv terminal was hit before the trip arrived: parcels are processed by hand,
			// hub scans are lost for 1–3, and 1–2 also lose both scans on the last leg
			b.moveLeg(ttn, focusTrips[1], false, n <= 3)
			b.moveLeg(ttn, focusTrips[2], n <= 3, n <= 2)
			continue
		}
		b.moveLeg(ttn, focusTrips[1], false, false)
		if ttn != FocusTTN || id == 5 {
			if id == 5 {
				b.add(contract.Event{Kind: contract.KindLoaded, TTN: ttn, TripID: "trip-f3", LocationID: "hub-kyiv", OccurredAt: at(19.75)}, false)
			} else {
				b.moveLeg(ttn, focusTrips[2], false, false)
			}
			continue
		}
		switch id {
		case 1: // not loaded at all: stays at hub-kyiv, no events on the last leg
		case 2: // loaded and unloaded, neither scan reported (the case from the document)
			b.moveLeg(ttn, focusTrips[2], true, true)
		case 3: // loaded onto the wrong trip, unloaded at Lviv №5
			wrong := trip{"trip-wrong", "hub-kyiv", "branch-lviv-5", 20, 30}
			b.runTrip(wrong, true)
			b.moveLeg(ttn, wrong, false, false)
		}
	}

	switch id {
	case 4: // a parcel nobody knows about lies at Kharkiv №3
		b.created(FocusTTN, "branch-lviv-5", "branch-kyiv-12", nil, true)
		b.add(contract.Event{Kind: contract.KindUnloaded, TTN: FocusTTN, LocationID: "branch-kharkiv-3", OccurredAt: at(22)}, true)
	case 5:
		cause, _ := json.Marshal(contract.IncidentPayload{Cause: "Обстріл"})
		b.add(contract.Event{Kind: contract.KindIncident, TripID: "trip-f3", OccurredAt: at(21), Payload: cause}, false)
	case 6:
		cause, _ := json.Marshal(contract.IncidentPayload{Cause: "Удар по терміналу, обробка вручну"})
		b.add(contract.Event{Kind: contract.KindIncident, LocationID: "hub-kyiv", OccurredAt: at(15), Payload: cause}, false)
	}
	return b.facts, nil
}
