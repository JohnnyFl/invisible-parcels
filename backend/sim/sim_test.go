package sim

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"novapost/backend/emuclient"
	"novapost/backend/store"
	"novapost/backend/testdb"
	"novapost/contract"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	url := testdb.URL(t)
	st, err := store.Open(context.Background(), url)
	if err != nil {
		t.Fatalf("open db (run `make db`): %v", err)
	}
	st.Pool.Exec(context.Background(), `DELETE FROM app_state`)
	t.Cleanup(st.Pool.Close)
	return st
}

type clockReq struct {
	VirtualNow time.Time `json:"virtualNow"`
	Speed      float64   `json:"speed"`
}

func fakeEmulator(t *testing.T, got *clockReq) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin/clock" {
			json.NewDecoder(r.Body).Decode(got)
		}
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSim_DefaultsPausedAtScenarioStart(t *testing.T) {
	var got clockReq
	s, err := New(context.Background(), testStore(t), &emuclient.Client{URL: fakeEmulator(t, &got).URL, HTTP: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Now().Equal(contract.ScenarioStart) || s.Mode() != ModeWithSolution || s.Clock().Speed != 0 {
		t.Fatalf("now=%v mode=%v", s.Now(), s.Mode())
	}
}

func TestSim_PersistsClockAndModeAndSyncsEmulator(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	var got clockReq
	emu := &emuclient.Client{URL: fakeEmulator(t, &got).URL, HTTP: http.DefaultClient}
	s, _ := New(ctx, st, emu)
	target := contract.ScenarioStart.Add(5 * time.Hour)
	if err := s.Jump(ctx, target); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(ctx, ModeAsIs); err != nil {
		t.Fatal(err)
	}
	if !got.VirtualNow.Equal(target) {
		t.Fatalf("emulator not synced: %+v", got)
	}
	s2, _ := New(ctx, st, emu)
	if !s2.Now().Equal(target) || s2.Mode() != ModeAsIs {
		t.Fatalf("not persisted: %v %v", s2.Now(), s2.Mode())
	}
	if err := s.SetMode(ctx, "bogus"); err == nil {
		t.Fatal("bogus mode accepted")
	}
}

func TestSim_SpeedRunsClock(t *testing.T) {
	ctx := context.Background()
	var got clockReq
	s, _ := New(ctx, testStore(t), &emuclient.Client{URL: fakeEmulator(t, &got).URL, HTTP: http.DefaultClient})
	s.SetSpeed(ctx, 3600) // 1 real ms = 3.6 virtual s
	time.Sleep(20 * time.Millisecond)
	if d := s.Now().Sub(contract.ScenarioStart); d < time.Minute {
		t.Fatalf("clock did not run: %v", d)
	}
	if got.Speed != 3600 {
		t.Fatalf("emulator speed %v", got.Speed)
	}
}

func TestSim_EmulatorDownReturnsError(t *testing.T) {
	ctx := context.Background()
	s, _ := New(ctx, testStore(t), &emuclient.Client{URL: "http://127.0.0.1:1", HTTP: http.DefaultClient})
	if err := s.SetSpeed(ctx, 60); err == nil {
		t.Fatal("expected error when emulator is down")
	}
}

func TestSim_SyncEmulatorRestoresScenarioClockAndOperatorEvents(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	st.Reset(ctx)
	st.InsertEvents(ctx, []contract.Event{
		{EventID: "np1", Kind: contract.KindCreated, TTN: contract.TTN(1), OccurredAt: contract.ScenarioStart, Source: "np", Payload: []byte(`{"DestinationID":"branch-kyiv-12"}`)},
		{EventID: "op1", Kind: contract.KindArrivedBranch, TTN: contract.TTN(1), LocationID: "branch-kyiv-12", OccurredAt: contract.ScenarioStart.Add(time.Hour), Source: "operator"},
	}, time.Now())
	emuScenario := 0
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/state":
			json.NewEncoder(w).Encode(map[string]int{"scenario": emuScenario})
			return
		case "/admin/operator-event":
			var e contract.Event
			json.NewDecoder(r.Body).Decode(&e)
			calls = append(calls, "op "+e.EventID)
		default:
			calls = append(calls, r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	t.Cleanup(srv.Close)
	s, _ := New(ctx, st, &emuclient.Client{URL: srv.URL, HTTP: http.DefaultClient})
	if err := s.SetScenario(ctx, 2); err != nil {
		t.Fatal(err)
	}

	emuScenario = 2 // in sync: nothing to do
	calls = nil
	s.SyncEmulator(ctx)
	if len(calls) != 0 {
		t.Fatalf("in sync but called %v", calls)
	}

	emuScenario = 0 // emulator restarted and lost its world
	if err := s.SyncEmulator(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"/admin/scenarios/2", "/admin/clock", "op op1"}
	if !slices.Equal(calls, want) {
		t.Fatalf("got %v, want %v", calls, want)
	}
}
