package scenarios

import (
	"slices"
	"testing"
	"time"

	"novapost/contract"
	"novapost/emulator/world"
)

func load(t *testing.T, id int) *world.World {
	t.Helper()
	facts, err := Build(id)
	if err != nil {
		t.Fatal(err)
	}
	w := world.New()
	w.Load(facts)
	return w
}

func TestBuild_AllScenariosHaveFocusParcelAndBackground(t *testing.T) {
	for id := 1; id <= 5; id++ {
		facts, err := Build(id)
		if err != nil {
			t.Fatal(err)
		}
		created, ids := 0, map[string]bool{}
		for _, f := range facts {
			if f.Event.Kind == contract.KindCreated {
				created++
			}
			if ids[f.Event.EventID] {
				t.Fatalf("scenario %d: duplicate event id %s", id, f.Event.EventID)
			}
			ids[f.Event.EventID] = true
		}
		if created < 26 {
			t.Fatalf("scenario %d: %d parcels", id, created)
		}
	}
	if _, err := Build(6); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuild_Deterministic(t *testing.T) {
	a, _ := Build(2)
	b, _ := Build(2)
	if len(a) != len(b) || a[10].Event.EventID != b[10].Event.EventID {
		t.Fatal("not deterministic")
	}
}

func TestScenario2_FocusInRecipientCityButPhysicallyAtBranch(t *testing.T) {
	w := load(t, 2)
	now := Start.Add(48 * time.Hour)
	st, _ := w.Track(FocusTTN, now)
	if st.StatusCode != contract.StatusInRecipientCity {
		t.Fatal(st)
	}
	if !slices.Contains(w.Physical("branch-kyiv-12", now), FocusTTN) {
		t.Fatal("not on shelf")
	}
	for _, e := range w.Due(now) {
		if e.TTN == FocusTTN && (e.Kind == contract.KindLoaded || e.Kind == contract.KindUnloaded) && e.TripID == "trip-f3" {
			t.Fatalf("scan on last leg was reported: %+v", e)
		}
	}
}

func TestScenario1_FocusLeftAtHub(t *testing.T) {
	w := load(t, 1)
	if !slices.Contains(w.Physical("hub-kyiv", Start.Add(48*time.Hour)), FocusTTN) {
		t.Fatal("expected at hub")
	}
}

func TestScenario3_FocusAtWrongBranch(t *testing.T) {
	w := load(t, 3)
	if !slices.Contains(w.Physical("branch-lviv-5", Start.Add(48*time.Hour)), FocusTTN) {
		t.Fatal("expected in Lviv")
	}
}

func TestScenario4_UnknownParcelAtKharkivNotReported(t *testing.T) {
	w := load(t, 4)
	now := Start.Add(48 * time.Hour)
	if !slices.Contains(w.Physical("branch-kharkiv-3", now), FocusTTN) {
		t.Fatal("expected in Kharkiv")
	}
	for _, e := range w.Due(now) {
		if e.TTN == FocusTTN {
			t.Fatalf("unknown parcel event reported: %+v", e)
		}
	}
}

func TestScenario5_IncidentOnLastTrip(t *testing.T) {
	w := load(t, 5)
	found := false
	for _, e := range w.Due(Start.Add(48 * time.Hour)) {
		if e.Kind == contract.KindIncident && e.TripID == "trip-f3" {
			found = true
		}
		if e.Kind == contract.KindTripArrived && e.TripID == "trip-f3" {
			t.Fatal("trip with incident arrived")
		}
	}
	if !found {
		t.Fatal("no incident")
	}
}

func TestBackgroundParcelsArrive(t *testing.T) {
	w := load(t, 2)
	if st, _ := w.Track(contract.TTN(100), Start.Add(48*time.Hour)); st.StatusCode != contract.StatusArrived {
		t.Fatalf("%+v", st)
	}
}
