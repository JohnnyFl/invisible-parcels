package poller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"novapost/backend/emuclient"
	"novapost/backend/store"
	"novapost/contract"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://novapost:novapost@localhost:5432/novapost?sslmode=disable"
	}
	st, err := store.Open(context.Background(), url)
	if err != nil {
		t.Fatalf("open db (run `make db`): %v", err)
	}
	st.Reset(context.Background())
	t.Cleanup(st.Pool.Close)
	return st
}

func insertParcel(t *testing.T, st *store.Store, ttn string) {
	p, _ := json.Marshal(contract.CreatedPayload{OriginID: "hub-kyiv", DestinationID: "branch-kyiv-12"})
	if _, err := st.InsertEvents(context.Background(), []contract.Event{{EventID: "c-" + ttn, Kind: contract.KindCreated, TTN: ttn, OccurredAt: contract.ScenarioStart, Source: "np", Payload: p}}, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met")
}

func TestPoller_SavesStatusAndKeepsSnapshotWhenEmulatorDown(t *testing.T) {
	st := testStore(t)
	insertParcel(t, st, contract.TTN(1))
	var up atomic.Bool
	up.Store(true)
	emu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up.Load() {
			w.WriteHeader(500)
			return
		}
		json.NewEncoder(w).Encode(contract.Response[contract.TrackingStatus]{Success: true, Data: []contract.TrackingStatus{
			{Number: contract.TTN(1), StatusCode: "6", Status: "x", ScheduledDeliveryDate: "18-10-2026 15:00:00"}}})
	}))
	defer emu.Close()
	var changes atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { Run(ctx, st, &emuclient.Client{URL: emu.URL, HTTP: http.DefaultClient}, 10*time.Millisecond, func() { changes.Add(1) }); close(done) }()
	waitFor(t, func() bool { c, _, _, ok, _ := st.NPStatus(context.Background(), contract.TTN(1)); return ok && c == "6" })
	_, _, sched, _, _ := st.NPStatus(context.Background(), contract.TTN(1))
	if want := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC); sched == nil || !sched.Equal(want) {
		t.Fatalf("scheduled %v, want %v (15:00 Kyiv)", sched, want)
	}
	if changes.Load() == 0 {
		t.Fatal("onChange not called")
	}
	up.Store(false)
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	if c, _, _, ok, _ := st.NPStatus(context.Background(), contract.TTN(1)); !ok || c != "6" {
		t.Fatal("snapshot lost")
	}
}

func TestUnissuedTTNs_SkipsIssued(t *testing.T) {
	st := testStore(t)
	insertParcel(t, st, contract.TTN(1))
	insertParcel(t, st, contract.TTN(2))
	st.InsertEvents(context.Background(), []contract.Event{{EventID: "i2", Kind: contract.KindIssued, TTN: contract.TTN(2), OccurredAt: contract.ScenarioStart, Source: "np"}}, time.Now())
	got, err := st.UnissuedTTNs(context.Background())
	if err != nil || len(got) != 1 || got[0] != contract.TTN(1) {
		t.Fatalf("%v %v", got, err)
	}
}
