package scenarios

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"

	"novapost/contract"
	"novapost/emulator/world"
)

// Kind is what happens to a random parcel on its way.
type Kind string

const (
	KindNormal      Kind = "normal"       // arrives, every scan reported
	KindNotLoaded   Kind = "not_loaded"   // left at the destination city hub, the last trip went without it
	KindMissedScans Kind = "missed_scans" // on the branch shelf, load and unload scans of the last leg lost
	KindMisrouted   Kind = "misrouted"    // unloaded at a branch that is not on its route
	KindUnknown     Kind = "unknown"      // nothing ever reported, lies on a branch shelf
	KindIncident    Kind = "incident"     // incident on its last trip
)

var Kinds = []Kind{KindNormal, KindNotLoaded, KindMissedScans, KindMisrouted, KindUnknown, KindIncident}

// Added tells the presenter what was created and whose operator screen shows it.
type Added struct {
	TTN    string `json:"ttn"`
	Kind   Kind   `json:"kind"`
	LookAt string `json:"lookAt"`
}

// RandomParcel builds one parcel of the given kind that starts at base. batch makes
// its event and trip IDs unique.
func RandomParcel(kind Kind, ttn string, base time.Time, batch int, r *rand.Rand) ([]world.Fact, Added) {
	b := &builder{prefix: fmt.Sprintf("r%d", batch)}
	h := base.Sub(Start).Hours()
	perm := r.Perm(len(branches))
	origin, dest, other := branches[perm[0]], branches[perm[1]], branches[perm[2]]

	if kind == KindUnknown {
		b.created(ttn, origin, dest, nil, true, h)
		b.add(contract.Event{Kind: contract.KindUnloaded, TTN: ttn, LocationID: other, OccurredAt: at(h + 2)}, true)
		return b.facts, Added{TTN: ttn, Kind: kind, LookAt: other}
	}

	trips := route(fmt.Sprintf("trip-r%d", batch), origin, dest, h-1) // first trip leaves an hour after base
	b.created(ttn, origin, dest, legsFor(trips, onTime(trips)), false, h)
	b.add(contract.Event{Kind: contract.KindAccepted, TTN: ttn, LocationID: origin, OccurredAt: at(h + 0.5)}, false)
	for _, t := range trips[:2] {
		b.runTrip(t, true)
		b.moveLeg(ttn, t, false, false)
	}
	last := trips[2]
	lookAt := dest
	switch kind {
	case KindNormal:
		b.runTrip(last, true)
		b.moveLeg(ttn, last, false, false)
	case KindNotLoaded:
		b.runTrip(last, true)
	case KindMissedScans:
		b.runTrip(last, true)
		b.moveLeg(ttn, last, true, true)
	case KindMisrouted:
		b.runTrip(last, true)
		wrong := trip{last.id + "-wrong", last.from, other, last.dep, last.arr}
		b.runTrip(wrong, true)
		b.moveLeg(ttn, wrong, false, false)
		lookAt = other
	case KindIncident:
		b.runTrip(last, false)
		b.add(contract.Event{Kind: contract.KindLoaded, TTN: ttn, TripID: last.id, LocationID: last.from, OccurredAt: at(last.dep - 0.25)}, false)
		cause, _ := json.Marshal(contract.IncidentPayload{Cause: "Обстріл"})
		b.add(contract.Event{Kind: contract.KindIncident, TripID: last.id, OccurredAt: at(last.dep + 1), Payload: cause}, false)
	}
	return b.facts, Added{TTN: ttn, Kind: kind, LookAt: lookAt}
}

// Random builds count parcels with TTNs from firstTTN, mostly normal ones,
// the rest spread over the fault kinds.
func Random(r *rand.Rand, base time.Time, firstTTN, batch, count int) ([]world.Fact, []Added) {
	weighted := append([]Kind{KindNormal, KindNormal, KindNormal}, Kinds...) // normal ≈ 4 in 9
	var facts []world.Fact
	var added []Added
	for i := range count {
		kind := weighted[r.IntN(len(weighted))]
		f, a := RandomParcel(kind, contract.TTN(firstTTN+i), base, batch*1000+i, r)
		facts = append(facts, f...)
		added = append(added, a)
	}
	return facts, added
}
