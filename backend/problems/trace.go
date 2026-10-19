package problems

import (
	"time"

	"novapost/contract"
)

type LegState string

const (
	LegDone      LegState = "done"
	LegInTransit LegState = "in_transit"
	LegBroken    LegState = "broken"
	LegIncident  LegState = "incident"
	LegPending   LegState = "pending"
)

type TraceLeg struct {
	Seq            int        `json:"seq"`
	From           string     `json:"from"`
	To             string     `json:"to"`
	TripID         string     `json:"tripId"`
	PlannedArrival time.Time  `json:"plannedArrival"`
	Loaded         *time.Time `json:"loaded,omitempty"`
	Departed       *time.Time `json:"departed,omitempty"`
	Arrived        *time.Time `json:"arrived,omitempty"`
	Unloaded       *time.Time `json:"unloaded,omitempty"`
	State          LegState   `json:"state"`
}

type Confirmed struct {
	Location string    `json:"location"`
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
}

type Break struct {
	Seq  int    `json:"seq"`
	Hint string `json:"hint"`
}

// Trace is the planned route next to what was actually scanned.
type Trace struct {
	TTN           string     `json:"ttn"`
	Destination   string     `json:"destination,omitempty"`
	Legs          []TraceLeg `json:"legs"`
	LastConfirmed *Confirmed `json:"lastConfirmed,omitempty"`
	Break         *Break     `json:"break,omitempty"`
	Delivered     bool       `json:"delivered"`
}

var confirmingKinds = map[string]bool{
	contract.KindAccepted: true, contract.KindLoaded: true, contract.KindArrivedHub: true, contract.KindSorted: true,
	contract.KindUnloaded: true, contract.KindArrivedBranch: true, contract.KindOperatorScan: true,
	contract.KindHold: true, contract.KindRedirected: true,
}

// placeName: "Київ, Відділення №12" for branches, "Сортувальний центр Київ" for hubs.
func placeName(id string) string {
	l, ok := contract.LocationByID(id)
	switch {
	case !ok:
		return id
	case l.Kind == "branch":
		return l.City + ", " + l.Name
	default:
		return l.Name
	}
}

// TraceParcel shows, leg by leg, where the parcel's chain of scans broke. Pure, like Classify.
func TraceParcel(in Input) Trace {
	tr := Trace{TTN: in.TTN, Legs: []TraceLeg{}}
	for i := range in.Events {
		e := &in.Events[i]
		if e.TTN != "" && e.LocationID != "" && confirmingKinds[e.Kind] {
			tr.LastConfirmed = &Confirmed{Location: e.LocationID, At: e.OccurredAt, Kind: e.Kind}
		}
	}
	if in.Parcel == nil {
		return tr
	}
	tr.Destination = in.Parcel.DestinationID

	for _, l := range in.Legs {
		tl := TraceLeg{Seq: l.Seq, From: l.FromID, To: l.ToID, TripID: l.TripID, PlannedArrival: l.PlannedArrival}
		incident := false
		for i := range in.Events {
			e := &in.Events[i]
			onTrip := e.TripID == l.TripID && l.TripID != ""
			switch {
			case e.Kind == contract.KindLoaded && e.TTN != "" && onTrip:
				tl.Loaded = &e.OccurredAt
			case e.Kind == contract.KindTripDeparted && onTrip:
				tl.Departed = &e.OccurredAt
			case e.Kind == contract.KindTripArrived && onTrip:
				tl.Arrived = &e.OccurredAt
			case (e.Kind == contract.KindUnloaded || e.Kind == contract.KindArrivedBranch) && e.TTN != "" && e.LocationID == l.ToID:
				tl.Unloaded = &e.OccurredAt
				if l.ToID == in.Parcel.DestinationID {
					tr.Delivered = true
				}
			case e.Kind == contract.KindIncident && (onTrip || (e.LocationID != "" && (e.LocationID == l.FromID || e.LocationID == l.ToID))):
				incident = true
			case e.Kind == contract.KindIssued:
				tr.Delivered = true
			}
		}

		from, to := placeName(l.FromID), placeName(l.ToID)
		hint := ""
		switch {
		case tl.Unloaded != nil:
			tl.State = LegDone
		case incident:
			tl.State = LegIncident
		case tl.Arrived != nil && tl.Loaded == nil:
			tl.State = LegBroken
			hint = "Рейс прибув у " + to + ", але ні завантаження в " + from + ", ні вивантаження в " + to + " не підтверджено. Посилка або лишилась у " + from + ", або лежить у " + to + " без скану."
		case tl.Arrived != nil:
			tl.State = LegBroken
			hint = "Рейс прибув у " + to + ", але вивантаження посилки не підтверджено. Перевірте полиці й зону розвантаження: " + to + "."
		case tl.Departed != nil && tl.Loaded == nil:
			tl.State = LegBroken
			hint = "Рейс виїхав, але завантаження посилки не підтверджено. Можливо, вона лишилась у " + from + "."
		case tl.Departed != nil:
			tl.State = LegInTransit
		default:
			tl.State = LegPending
		}
		if hint != "" && tr.Break == nil {
			tr.Break = &Break{Seq: l.Seq, Hint: hint}
		}
		tr.Legs = append(tr.Legs, tl)
	}
	return tr
}
