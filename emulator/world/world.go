// Package world is the emulator's in-memory "Nova Poshta": what physically
// happened, what was reported, and what the client app shows.
package world

import (
	"encoding/json"
	"slices"
	"sync"
	"time"

	"novapost/contract"
)

// Fact is something that happens in the world. Hidden = it physically happened
// but was never reported (a missed scan). Something that physically did not
// happen is simply absent.
type Fact struct {
	Event  contract.Event
	Hidden bool
}

type fact struct {
	Fact
	sent bool
}

type World struct {
	mu      sync.Mutex
	facts   []fact
	parcels map[string]contract.CreatedPayload
}

func New() *World { return &World{parcels: map[string]contract.CreatedPayload{}} }

// Load replaces the whole world.
func (w *World) Load(facts []Fact) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.facts = nil
	w.parcels = map[string]contract.CreatedPayload{}
	for _, f := range facts {
		w.add(fact{Fact: f})
	}
}

func (w *World) add(f fact) {
	if f.Event.Kind == contract.KindCreated {
		var p contract.CreatedPayload
		if json.Unmarshal(f.Event.Payload, &p) == nil {
			w.parcels[f.Event.TTN] = p
		}
	}
	w.facts = append(w.facts, f)
}

// Due returns not-yet-sent reported events with OccurredAt <= now, oldest first.
func (w *World) Due(now time.Time) []contract.Event {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []contract.Event
	for i := range w.facts {
		f := &w.facts[i]
		if !f.sent && !f.Hidden && !f.Event.OccurredAt.After(now) {
			f.sent = true
			out = append(out, f.Event)
		}
	}
	slices.SortStableFunc(out, func(a, b contract.Event) int { return a.OccurredAt.Compare(b.OccurredAt) })
	return out
}

// Apply records an operator action. It is already known to the backend, so it is never pushed back.
func (w *World) Apply(e contract.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.add(fact{Fact: Fact{Event: e}, sent: true})
}

// past returns facts that happened by now, oldest first.
func (w *World) past(now time.Time) []fact {
	var out []fact
	for _, f := range w.facts {
		if !f.Event.OccurredAt.After(now) {
			out = append(out, f)
		}
	}
	slices.SortStableFunc(out, func(a, b fact) int { return a.Event.OccurredAt.Compare(b.Event.OccurredAt) })
	return out
}

// Track computes what the client app shows. It follows the trips in the route
// plan, not the parcel's own scans — that is the hypothesis behind the case.
func (w *World) Track(ttn string, now time.Time) (contract.TrackingStatus, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.parcels[ttn]
	if !ok {
		return contract.TrackingStatus{}, false
	}
	trips := map[string]bool{}
	for _, l := range p.RouteLegs {
		trips[l.TripID] = true
	}
	var last contract.RouteLeg
	if n := len(p.RouteLegs); n > 0 {
		last = p.RouteLegs[n-1]
	}

	seen := map[string]bool{}
	created := false
	for _, f := range w.past(now) {
		e := f.Event
		switch {
		case e.TTN == ttn && e.Kind == contract.KindCreated:
			created = true // created counts even if hidden from the backend
		case e.TTN == ttn && !f.Hidden:
			if (e.Kind == contract.KindUnloaded || e.Kind == contract.KindArrivedBranch) && e.LocationID == p.DestinationID {
				seen["arrived"] = true
			}
			seen[e.Kind] = true
		case e.TTN == "" && e.Kind == contract.KindTripArrived && e.TripID == last.TripID && last.TripID != "":
			seen["last_trip_arrived"] = true
		case e.TTN == "" && e.Kind == contract.KindTripDeparted && trips[e.TripID]:
			seen["trip_departed"] = true
		}
	}
	if !created {
		return contract.TrackingStatus{}, false
	}

	st := contract.TrackingStatus{Number: ttn}
	if l, ok := contract.LocationByID(p.DestinationID); ok {
		st.CityRecipient, st.WarehouseRecipient = l.City, l.Name
	}
	switch {
	case seen[contract.KindIssued]:
		st.StatusCode = contract.StatusIssued
	case seen[contract.KindReturnInitiated]:
		st.StatusCode = contract.StatusReturn
	case seen[contract.KindRedirected]:
		st.StatusCode = contract.StatusRedirected
	case seen["arrived"]:
		st.StatusCode = contract.StatusArrived
	case seen["last_trip_arrived"]:
		st.StatusCode = contract.StatusInRecipientCity
		st.ScheduledDeliveryDate = last.PlannedArrival.In(contract.Kyiv).Format(contract.NPDateLayout)
	case seen["trip_departed"]:
		st.StatusCode = contract.StatusInTransit
	case seen[contract.KindAccepted]:
		st.StatusCode = contract.StatusAccepted
	default:
		st.StatusCode = contract.StatusCreated
	}
	st.Status = contract.StatusText(st.StatusCode)
	return st, true
}

var placing = map[string]bool{
	contract.KindAccepted: true, contract.KindArrivedHub: true, contract.KindSorted: true,
	contract.KindUnloaded: true, contract.KindArrivedBranch: true, contract.KindOperatorScan: true,
}

var leaving = map[string]bool{
	contract.KindLoaded: true, contract.KindIssued: true, contract.KindRedirected: true, contract.KindReturnInitiated: true,
}

// Physical lists TTNs physically lying at the location now (reported or not).
func (w *World) Physical(locationID string, now time.Time) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	at := map[string]string{} // ttn -> location, "" when moving
	var order []string
	for _, f := range w.past(now) {
		e := f.Event
		if e.TTN == "" {
			continue
		}
		if _, ok := at[e.TTN]; !ok {
			order = append(order, e.TTN)
		}
		switch {
		case placing[e.Kind]:
			at[e.TTN] = e.LocationID
		case leaving[e.Kind]:
			at[e.TTN] = ""
		}
	}
	out := []string{}
	for _, ttn := range order {
		if at[ttn] == locationID {
			out = append(out, ttn)
		}
	}
	return out
}
