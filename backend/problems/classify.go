// Package problems classifies parcels into severity levels. Pure: no DB, no clock.
package problems

import (
	"encoding/json"
	"time"

	"novapost/contract"
)

type Severity int

const (
	Green Severity = iota
	Grey
	Yellow
	Orange
	Red
)

var severityNames = [...]string{"green", "grey", "yellow", "orange", "red"}

func (s Severity) String() string { return severityNames[s] }

func (s Severity) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

type Kind string

const (
	Unknown     Kind = "unknown"
	Misrouted   Kind = "misrouted"
	RiskyReturn Kind = "risky_return"
	BrokenChain Kind = "broken_chain"
	Overdue     Kind = "overdue"
	Stuck       Kind = "stuck"
	Incident    Kind = "incident"
)

type Problem struct {
	Kind     Kind      `json:"kind"`
	Severity Severity  `json:"severity"`
	Since    time.Time `json:"since"`
}

// Input is everything known about one TTN. Parcel == nil means the TTN is unknown
// to the system. Events are sorted by OccurredAt.
type Input struct {
	TTN         string
	Parcel      *contract.CreatedPayload
	Legs        []contract.RouteLeg
	Events      []contract.Event
	NPScheduled *time.Time
	Now         time.Time
}

func Level(ps []Problem) Severity {
	l := Green
	for _, p := range ps {
		l = max(l, p.Severity)
	}
	return l
}

func Classify(in Input) []Problem {
	var out []Problem
	add := func(k Kind, s Severity, since time.Time) {
		age := in.Now.Sub(since)
		switch {
		case s == Orange && age > EscalateOrange:
			s = Red
		case s == Yellow && age > EscalateYellow:
			s = Orange
		}
		out = append(out, Problem{Kind: k, Severity: s, Since: since})
	}

	if in.Parcel == nil {
		for _, e := range in.Events {
			if e.Kind == contract.KindOperatorScan {
				add(Unknown, Red, e.OccurredAt)
				break
			}
		}
		addRiskyReturns(in, add)
		return out
	}

	var arrived, issued bool
	var redirectedAt, lastAt time.Time
	var incidentAt *time.Time
	tripArrived := map[string]time.Time{}
	unloadedAt := map[string]bool{}
	var lastPlace *contract.Event
	for i, e := range in.Events {
		if contract.IsManagement(e.Kind) {
			continue // a decision about the parcel is not a sign of life
		}
		lastAt = e.OccurredAt
		switch e.Kind {
		case contract.KindIssued:
			issued = true
		case contract.KindUnloaded, contract.KindArrivedBranch:
			unloadedAt[e.LocationID] = true
			if e.LocationID == in.Parcel.DestinationID {
				arrived = true
			}
		case contract.KindTripArrived:
			tripArrived[e.TripID] = e.OccurredAt
		case contract.KindRedirected:
			redirectedAt = e.OccurredAt
		case contract.KindIncident:
			if incidentAt == nil {
				incidentAt = &in.Events[i].OccurredAt
			}
		}
		if e.LocationID != "" && (e.Kind == contract.KindUnloaded || e.Kind == contract.KindArrivedBranch || e.Kind == contract.KindOperatorScan) {
			lastPlace = &in.Events[i]
		}
	}
	if issued || arrived {
		return nil
	}

	onRoute := map[string]bool{in.Parcel.OriginID: true, in.Parcel.DestinationID: true}
	for _, l := range in.Legs {
		onRoute[l.FromID], onRoute[l.ToID] = true, true
	}
	if lastPlace != nil && !onRoute[lastPlace.LocationID] && lastPlace.OccurredAt.After(redirectedAt) {
		if l, ok := contract.LocationByID(lastPlace.LocationID); ok && l.Kind == "branch" {
			add(Misrouted, Red, lastPlace.OccurredAt)
		}
	}

	addRiskyReturns(in, add)

	if incidentAt != nil {
		add(Incident, Grey, *incidentAt)
	}

	redirected := !redirectedAt.IsZero()
	if !redirected {
		for _, l := range in.Legs {
			if at, ok := tripArrived[l.TripID]; ok && l.TripID != "" && !unloadedAt[l.ToID] {
				add(BrokenChain, Orange, at)
				break
			}
		}
	}

	if incidentAt != nil {
		return out // known cause: no overdue / stuck
	}
	if eta, ok := etaOf(in); ok && !redirected {
		if since := eta.Add(OverdueAfter); in.Now.After(since) {
			add(Overdue, Orange, since)
		}
	}
	if since := lastAt.Add(StuckAfter); !lastAt.IsZero() && in.Now.After(since) {
		add(Stuck, Yellow, since)
	}
	return out
}

func etaOf(in Input) (time.Time, bool) {
	if in.NPScheduled != nil {
		return *in.NPScheduled, true
	}
	if n := len(in.Legs); n > 0 {
		return in.Legs[n-1].PlannedArrival, true
	}
	return time.Time{}, false
}

// addRiskyReturns flags each return started while the parcel had an orange/red
// problem or was on hold — judged by the events before that return.
func addRiskyReturns(in Input, add func(Kind, Severity, time.Time)) {
	for i, e := range in.Events {
		if e.Kind != contract.KindReturnInitiated {
			continue
		}
		before := in
		before.Events = in.Events[:i]
		before.Now = e.OccurredAt
		held := false
		for _, b := range before.Events {
			held = held || b.Kind == contract.KindHold
		}
		after := in.Events[i+1:]
		if (held || Level(Classify(before)) >= Orange) && !scannedElsewhereAfter(after, e.LocationID) && !confirmed(after) {
			add(RiskyReturn, Red, e.OccurredAt)
			return
		}
	}
}

var placingKinds = map[string]bool{
	contract.KindOperatorScan: true, contract.KindUnloaded: true, contract.KindArrivedBranch: true,
	contract.KindArrivedHub: true, contract.KindSorted: true, contract.KindAccepted: true,
}

// scannedElsewhereAfter: the returned parcel turned up at another location, so it is found.
func scannedElsewhereAfter(evs []contract.Event, returnedAt string) bool {
	for _, e := range evs {
		if placingKinds[e.Kind] && e.LocationID != "" && e.LocationID != returnedAt {
			return true
		}
	}
	return false
}

// confirmed: the senior shift checked the return and confirmed it is justified.
func confirmed(evs []contract.Event) bool {
	for _, e := range evs {
		if e.Kind == contract.KindReturnConfirmed {
			return true
		}
	}
	return false
}
