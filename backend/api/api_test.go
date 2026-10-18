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
	"novapost/backend/store"
	"novapost/contract"
)

var t0 = contract.ScenarioStart

func testDeps(t *testing.T) Deps {
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
	cfg := config.Config{IngestSecret: "ingest", AppSecret: "app", AdminPassword: "a", SeniorPassword: "s", OperatorPassword: "o", LookupLimitPerHour: 30}
	return Deps{Cfg: cfg, Store: st, Hub: NewHub()}
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
