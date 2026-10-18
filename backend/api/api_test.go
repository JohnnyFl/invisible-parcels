package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"novapost/backend/config"
	"novapost/backend/emuclient"
	"novapost/backend/sim"
	"novapost/backend/store"
	"novapost/contract"
)

var t0 = contract.ScenarioStart

// fakeEmulator accepts every admin call and serves physical/tracking from the given maps.
func fakeEmulator(t *testing.T, physical map[string][]string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/physical":
			json.NewEncoder(w).Encode(map[string][]string{"ttns": physical[r.URL.Query().Get("location")]})
		default:
			w.WriteHeader(204)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testDeps(t *testing.T) Deps { return testDepsEmu(t, fakeEmulator(t, nil).URL) }

func testDepsEmu(t *testing.T, emuURL string) Deps {
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
	st.Pool.Exec(context.Background(), `DELETE FROM app_state`)
	t.Cleanup(st.Pool.Close)
	cfg := config.Config{IngestSecret: "ingest", AppSecret: "app", AdminPassword: "a", SeniorPassword: "s", OperatorPassword: "o", LookupLimitPerHour: 30}
	emu := &emuclient.Client{URL: emuURL, HTTP: http.DefaultClient}
	s, err := sim.New(context.Background(), st, emu)
	if err != nil {
		t.Fatal(err)
	}
	return Deps{Cfg: cfg, Store: st, Hub: NewHub(), Sim: s, Emu: emu}
}

func scenario2Events() []contract.Event {
	legs := []contract.RouteLeg{{Seq: 1, FromID: "hub-kyiv", ToID: "branch-kyiv-12", TripID: "trip-1",
		PlannedDeparture: t0.Add(26 * time.Hour), PlannedArrival: t0.Add(28 * time.Hour)}}
	p, _ := json.Marshal(contract.CreatedPayload{SenderPhone: "+380500000001", RecipientPhone: "+380670000001", OriginID: "hub-kyiv", DestinationID: "branch-kyiv-12", RouteLegs: legs})
	return []contract.Event{
		{EventID: "e1", Kind: contract.KindCreated, TTN: contract.TTN(1), LocationID: "hub-kyiv", OccurredAt: t0, Source: "np", Payload: p},
		{EventID: "e2", Kind: contract.KindTripDeparted, TripID: "trip-1", LocationID: "hub-kyiv", OccurredAt: t0.Add(time.Hour), Source: "np"},
		{EventID: "e3", Kind: contract.KindTripArrived, TripID: "trip-1", LocationID: "branch-kyiv-12", OccurredAt: t0.Add(4 * time.Hour), Source: "np"},
	}
}

func ingestBody(evs []contract.Event) []byte {
	b, _ := json.Marshal(contract.Envelope[contract.EventBatch]{ModelName: "ScanEvent", CalledMethod: "push", MethodProperties: contract.EventBatch{Events: evs}})
	return b
}

func do(e http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func ingest(t *testing.T, e http.Handler, secret string, evs []contract.Event) {
	t.Helper()
	body := ingestBody(evs)
	req := httptest.NewRequest("POST", "/api/ingest/events", strings.NewReader(string(body)))
	req.Header.Set("X-Signature", contract.Sign(body, secret))
	req.Header.Set("Content-Type", "application/json")
	if rec := do(e, req); rec.Code != 200 {
		t.Fatalf("ingest: %d %s", rec.Code, rec.Body)
	}
}

func TestIngest_RejectsBadSignature(t *testing.T) {
	e := NewServer(testDeps(t))
	req := httptest.NewRequest("POST", "/api/ingest/events", strings.NewReader(string(ingestBody(scenario2Events()))))
	req.Header.Set("X-Signature", "deadbeef")
	if rec := do(e, req); rec.Code != 401 {
		t.Fatal(rec.Code)
	}
}

func TestIngest_StoresAndReportsInserted(t *testing.T) {
	d := testDeps(t)
	e := NewServer(d)
	body := ingestBody(scenario2Events())
	for i, want := range []string{`"inserted":3`, `"inserted":0`} {
		req := httptest.NewRequest("POST", "/api/ingest/events", strings.NewReader(string(body)))
		req.Header.Set("X-Signature", contract.Sign(body, "ingest"))
		rec := do(e, req)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("call %d: %d %s", i, rec.Code, rec.Body)
		}
	}
	if p, _, _ := d.Store.Parcel(context.Background(), contract.TTN(1)); p == nil {
		t.Fatal("parcel not stored")
	}
}

func TestLocations(t *testing.T) {
	rec := do(NewServer(testDeps(t)), httptest.NewRequest("GET", "/api/locations", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"branch-kyiv-12"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func jsonReq(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestSimAPI_ClockAndMode(t *testing.T) {
	d := testDeps(t)
	e := NewServer(d)
	if rec := do(e, jsonReq("POST", "/api/sim/clock", `{"jumpTo":"2026-10-18T10:00:00Z","speed":0}`)); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := do(e, jsonReq("POST", "/api/sim/mode", `{"mode":"as_is"}`)); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	rec := do(e, httptest.NewRequest("GET", "/api/sim/clock", nil))
	if !strings.Contains(rec.Body.String(), `"virtualNow":"2026-10-18T10:00:00Z"`) || !strings.Contains(rec.Body.String(), `"mode":"as_is"`) {
		t.Fatal(rec.Body.String())
	}
	if rec := do(e, jsonReq("POST", "/api/sim/mode", `{"mode":"bogus"}`)); rec.Code != 400 {
		t.Fatal(rec.Code)
	}
}

func TestSimAPI_ScenarioWhenEmulatorDownIs502(t *testing.T) {
	e := NewServer(testDepsEmu(t, "http://127.0.0.1:1"))
	if rec := do(e, jsonReq("POST", "/api/sim/scenarios/2", ``)); rec.Code != 502 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestSimAPI_ScenarioResetsDataAndStartsClock(t *testing.T) {
	d := testDeps(t)
	e := NewServer(d)
	ingest(t, e, "ingest", scenario2Events())
	if rec := do(e, jsonReq("POST", "/api/sim/scenarios/2", ``)); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if p, _, _ := d.Store.Parcel(context.Background(), contract.TTN(1)); p != nil {
		t.Fatal("data not reset")
	}
	if d.Sim.Clock().Speed != 60 || d.Sim.Now().Before(contract.ScenarioStart) {
		t.Fatalf("clock %+v", d.Sim.Clock())
	}
}
