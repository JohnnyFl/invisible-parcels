package scenarios

import (
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"novapost/contract"
	"novapost/emulator/world"
)

var base = Start.Add(30 * time.Hour)

func parcelWorld(t *testing.T, kind Kind) (*world.World, Added) {
	t.Helper()
	facts, added := RandomParcel(kind, contract.TTN(100001), base, 7, rand.New(rand.NewPCG(1, 2)))
	w := world.New()
	w.Load(facts)
	if added.TTN != contract.TTN(100001) || added.Kind != kind || added.LookAt == "" {
		t.Fatalf("%+v", added)
	}
	return w, added
}

func TestRandomParcel_EachKindBehaves(t *testing.T) {
	later := base.Add(48 * time.Hour)
	dest := func(w *world.World, ttn string) string {
		st, _ := w.Track(ttn, later)
		for _, l := range contract.Locations {
			if l.Kind == "branch" && l.Name == st.WarehouseRecipient && l.City == st.CityRecipient {
				return l.ID
			}
		}
		return ""
	}

	t.Run("normal arrives", func(t *testing.T) {
		w, a := parcelWorld(t, KindNormal)
		if st, _ := w.Track(a.TTN, later); st.StatusCode != contract.StatusArrived {
			t.Fatalf("%+v", st)
		}
	})
	t.Run("not loaded stays at the destination city hub", func(t *testing.T) {
		w, a := parcelWorld(t, KindNotLoaded)
		if !slices.Contains(w.Physical(hubOf[dest(w, a.TTN)], later), a.TTN) {
			t.Fatal("not on the hub shelf")
		}
	})
	t.Run("missed scans: on the branch shelf, client sees the forecast", func(t *testing.T) {
		w, a := parcelWorld(t, KindMissedScans)
		if !slices.Contains(w.Physical(a.LookAt, later), a.TTN) {
			t.Fatal("not on the branch shelf")
		}
		if st, _ := w.Track(a.TTN, later); st.StatusCode != contract.StatusInRecipientCity {
			t.Fatalf("%+v", st)
		}
	})
	t.Run("misrouted lands at a branch that is not its destination", func(t *testing.T) {
		w, a := parcelWorld(t, KindMisrouted)
		if a.LookAt == dest(w, a.TTN) || !slices.Contains(w.Physical(a.LookAt, later), a.TTN) {
			t.Fatalf("%+v dest=%s", a, dest(w, a.TTN))
		}
	})
	t.Run("unknown is never reported but lies on a shelf", func(t *testing.T) {
		w, a := parcelWorld(t, KindUnknown)
		for _, e := range w.Due(later) {
			if e.TTN == a.TTN {
				t.Fatalf("reported: %+v", e)
			}
		}
		if !slices.Contains(w.Physical(a.LookAt, later), a.TTN) {
			t.Fatal("not on the shelf")
		}
	})
	t.Run("incident on its last trip", func(t *testing.T) {
		w, _ := parcelWorld(t, KindIncident)
		found := false
		for _, e := range w.Due(later) {
			found = found || e.Kind == contract.KindIncident
		}
		if !found {
			t.Fatal("no incident")
		}
	})
}

func TestRandom_MixesKindsWithUniqueIDsFromBase(t *testing.T) {
	facts, added := Random(rand.New(rand.NewPCG(3, 4)), base, 100001, 1, 200)
	if len(added) != 200 || added[0].TTN != contract.TTN(100001) || added[199].TTN != contract.TTN(100200) {
		t.Fatalf("%d %+v", len(added), added[0])
	}
	seen := map[Kind]bool{}
	for _, a := range added {
		seen[a.Kind] = true
	}
	if len(seen) != len(Kinds) {
		t.Fatalf("kinds seen: %v", seen)
	}
	ids := map[string]bool{}
	for _, f := range facts {
		if ids[f.Event.EventID] {
			t.Fatalf("duplicate %s", f.Event.EventID)
		}
		ids[f.Event.EventID] = true
		if f.Event.OccurredAt.Before(base) {
			t.Fatalf("event before base: %+v", f.Event)
		}
	}
}
