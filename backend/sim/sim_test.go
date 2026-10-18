package sim

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
