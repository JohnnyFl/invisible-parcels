package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"novapost/backend/auth"
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

func sessionCookie(d Deps, role, branch string) *http.Cookie {
	return &http.Cookie{Name: auth.CookieName, Value: auth.Encode(auth.Session{Role: role, BranchID: branch, ExpiresAt: time.Now().Add(time.Hour)}, d.Cfg.AppSecret)}
}

func adminCookie(d Deps) *http.Cookie { return sessionCookie(d, "admin", "") }

func operatorCookie(d Deps, branch string) *http.Cookie { return sessionCookie(d, "operator", branch) }

func jsonReq(method, path, body string, cookies ...*http.Cookie) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return req
}

func TestSimAPI_RequiresAdmin(t *testing.T) {
	d := testDeps(t)
	e := NewServer(d)
	if rec := do(e, jsonReq("GET", "/api/sim/clock", "")); rec.Code != 401 {
		t.Fatal(rec.Code)
	}
	if rec := do(e, jsonReq("GET", "/api/sim/clock", "", operatorCookie(d, "branch-kyiv-12"))); rec.Code != 403 {
		t.Fatal(rec.Code)
	}
}

func TestSimAPI_ClockAndMode(t *testing.T) {
	d := testDeps(t)
	e := NewServer(d)
	a := adminCookie(d)
	if rec := do(e, jsonReq("POST", "/api/sim/clock", `{"jumpTo":"2026-10-18T10:00:00Z","speed":0}`, a)); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := do(e, jsonReq("POST", "/api/sim/mode", `{"mode":"as_is"}`, a)); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	rec := do(e, jsonReq("GET", "/api/sim/clock", "", a))
	if !strings.Contains(rec.Body.String(), `"virtualNow":"2026-10-18T10:00:00Z"`) || !strings.Contains(rec.Body.String(), `"mode":"as_is"`) {
		t.Fatal(rec.Body.String())
	}
	if rec := do(e, jsonReq("POST", "/api/sim/mode", `{"mode":"bogus"}`, a)); rec.Code != 400 {
		t.Fatal(rec.Code)
	}
}

func TestSimAPI_ScenarioWhenEmulatorDownIs502(t *testing.T) {
	d := testDepsEmu(t, "http://127.0.0.1:1")
	e := NewServer(d)
	if rec := do(e, jsonReq("POST", "/api/sim/scenarios/2", ``, adminCookie(d))); rec.Code != 502 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestSimAPI_ScenarioResetsDataAndStartsClock(t *testing.T) {
	d := testDeps(t)
	e := NewServer(d)
	ingest(t, e, "ingest", scenario2Events())
	if rec := do(e, jsonReq("POST", "/api/sim/scenarios/2", ``, adminCookie(d))); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if p, _, _ := d.Store.Parcel(context.Background(), contract.TTN(1)); p != nil {
		t.Fatal("data not reset")
	}
	if d.Sim.Clock().Speed != 60 || d.Sim.Now().Before(contract.ScenarioStart) {
		t.Fatalf("clock %+v", d.Sim.Clock())
	}
}

type problemRow struct {
	TTN      string `json:"ttn"`
	Level    string `json:"level"`
	Problems []struct {
		Kind string `json:"kind"`
	} `json:"problems"`
	Destination string `json:"destination"`
	Location    string `json:"location"`
}

func getJSON[T any](t *testing.T, e http.Handler, path string, c *http.Cookie) (T, int) {
	t.Helper()
	var out T
	rec := do(e, jsonReq("GET", path, "", c))
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out, rec.Code
}

func getProblems(t *testing.T, e http.Handler, c *http.Cookie) []problemRow {
	t.Helper()
	rows, code := getJSON[[]problemRow](t, e, "/api/operator/problems", c)
	if code != 200 {
		t.Fatalf("problems: %d", code)
	}
	return rows
}

type scanResult struct {
	Outcome      string `json:"outcome"`
	NextLocation string `json:"nextLocation"`
}

func scan(t *testing.T, e http.Handler, c *http.Cookie, ttn string) (scanResult, *httptest.ResponseRecorder) {
	t.Helper()
	rec := do(e, jsonReq("POST", "/api/operator/scan", `{"ttn":"`+ttn+`"}`, c))
	var r scanResult
	json.Unmarshal(rec.Body.Bytes(), &r)
	return r, rec
}

func act(t *testing.T, e http.Handler, c *http.Cookie, ttn, action, body string) *httptest.ResponseRecorder {
	t.Helper()
	return do(e, jsonReq("POST", "/api/operator/parcels/"+ttn+"/"+action, body, c))
}

func TestCase_HiddenUnload_BecomesOrangeThenRed(t *testing.T) {
	d := testDeps(t)
	ctx := context.Background()
	e := NewServer(d)
	op := operatorCookie(d, "branch-kyiv-12")
	ingest(t, e, "ingest", scenario2Events()) // trip arrived at t0+4h, no unload reported
	d.Sim.Jump(ctx, t0.Add(5*time.Hour))
	rows := getProblems(t, e, op)
	if len(rows) != 1 || rows[0].Level != "orange" || rows[0].Problems[0].Kind != "broken_chain" || rows[0].TTN != contract.TTN(1) {
		t.Fatalf("%+v", rows)
	}
	d.Sim.Jump(ctx, t0.Add(10*time.Hour+time.Minute))
	if rows := getProblems(t, e, op); len(rows) != 1 || rows[0].Level != "red" {
		t.Fatalf("%+v", rows)
	}
	if out, rec := scan(t, e, op, contract.TTN(1)); out.Outcome != "accept" || strings.Contains(rec.Body.String(), "+380") {
		t.Fatalf("%+v %s", out, rec.Body)
	}
	if rec := act(t, e, op, contract.TTN(1), "accept", ""); rec.Code != 200 {
		t.Fatalf("accept: %d %s", rec.Code, rec.Body)
	}
	if rows := getProblems(t, e, op); len(rows) != 0 {
		t.Fatalf("not cleared: %+v", rows)
	}
	// another branch does not see it
	if rows := getProblems(t, e, operatorCookie(d, "branch-lviv-5")); len(rows) != 0 {
		t.Fatalf("leaked to lviv: %+v", rows)
	}
}

func TestOperatorProblems_HiddenInAsIsMode(t *testing.T) {
	d := testDeps(t)
	e := NewServer(d)
	ingest(t, e, "ingest", scenario2Events())
	d.Sim.Jump(context.Background(), t0.Add(5*time.Hour))
	d.Sim.SetMode(context.Background(), sim.ModeAsIs)
	if rows := getProblems(t, e, operatorCookie(d, "branch-kyiv-12")); len(rows) != 0 {
		t.Fatalf("%+v", rows)
	}
}

func TestIncoming_AsIsOnlyWithLoadedScan(t *testing.T) {
	d := testDeps(t)
	ctx := context.Background()
	e := NewServer(d)
	op := operatorCookie(d, "branch-kyiv-12")
	ingest(t, e, "ingest", scenario2Events())
	type row struct {
		TTN    string `json:"ttn"`
		From   string `json:"from"`
		TripID string `json:"tripId"`
	}
	if rows, _ := getJSON[[]row](t, e, "/api/operator/incoming", op); len(rows) != 1 || rows[0].From != "hub-kyiv" || rows[0].TripID != "trip-1" {
		t.Fatalf("with solution: %+v", rows)
	}
	d.Sim.SetMode(ctx, sim.ModeAsIs)
	if rows, _ := getJSON[[]row](t, e, "/api/operator/incoming", op); len(rows) != 0 {
		t.Fatalf("as is without loaded: %+v", rows)
	}
	ingest(t, e, "ingest", []contract.Event{{EventID: "l1", Kind: contract.KindLoaded, TTN: contract.TTN(1), TripID: "trip-1", LocationID: "hub-kyiv", OccurredAt: t0.Add(time.Hour), Source: "np"}})
	if rows, _ := getJSON[[]row](t, e, "/api/operator/incoming", op); len(rows) != 1 {
		t.Fatalf("as is with loaded: %+v", rows)
	}
}

func TestScan_UnknownGoesOnHoldAndMisroutedIsRedirected(t *testing.T) {
	d := testDeps(t)
	e := NewServer(d)
	ingest(t, e, "ingest", scenario2Events())
	lviv := operatorCookie(d, "branch-lviv-5")
	if out, _ := scan(t, e, lviv, contract.TTN(1)); out.Outcome != "redirect" || out.NextLocation != "hub-lviv" {
		t.Fatalf("%+v", out)
	}
	if out, _ := scan(t, e, lviv, contract.TTN(77)); out.Outcome != "hold" {
		t.Fatalf("%+v", out)
	}
	rows := getProblems(t, e, lviv)
	kinds := map[string]string{}
	for _, r := range rows {
		kinds[r.TTN] = r.Problems[0].Kind
	}
	if kinds[contract.TTN(77)] != "unknown" || kinds[contract.TTN(1)] != "misrouted" {
		t.Fatalf("%+v", rows)
	}
	if rec := act(t, e, lviv, contract.TTN(1), "redirect", ""); rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	for _, r := range getProblems(t, e, lviv) {
		if r.TTN == contract.TTN(1) {
			t.Fatalf("still misrouted after redirect: %+v", r)
		}
	}
	d.Sim.SetMode(context.Background(), sim.ModeAsIs)
	if out, _ := scan(t, e, lviv, contract.TTN(78)); out.Outcome != "not_found" {
		t.Fatalf("as is: %+v", out)
	}
}

func TestScan_RateLimited(t *testing.T) {
	d := testDeps(t)
	e := NewServer(d)
	op := operatorCookie(d, "branch-kharkiv-3")
	for i := 0; i < 30; i++ {
		if _, rec := scan(t, e, op, contract.TTN(500+i)); rec.Code != 200 {
			t.Fatalf("scan %d: %d", i, rec.Code)
		}
	}
	if _, rec := scan(t, e, op, contract.TTN(600)); rec.Code != 429 {
		t.Fatalf("31st: %d", rec.Code)
	}
	var n int
	d.Store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE result='rate_limited' AND branch_id='branch-kharkiv-3'`).Scan(&n)
	if n != 1 {
		t.Fatalf("audit rows %d", n)
	}
	if _, rec := scan(t, e, operatorCookie(d, "branch-kyiv-12"), contract.TTN(601)); rec.Code != 200 {
		t.Fatalf("other branch limited: %d", rec.Code)
	}
}

func TestScan_InvalidTTN(t *testing.T) {
	d := testDeps(t)
	e := NewServer(d)
	op := operatorCookie(d, "branch-kharkiv-3")
	for _, bad := range []string{"123", "abc", "5900000000001x", ""} {
		if _, rec := scan(t, e, op, bad); rec.Code != 400 {
			t.Fatalf("%q: %d", bad, rec.Code)
		}
	}
	for i := 0; i < 30; i++ {
		if _, rec := scan(t, e, op, contract.TTN(500+i)); rec.Code != 200 {
			t.Fatalf("limit consumed by invalid scans at %d: %d", i, rec.Code)
		}
	}
}

func TestOperatorEndpoints_RequireOperator(t *testing.T) {
	d := testDeps(t)
	e := NewServer(d)
	if rec := do(e, jsonReq("GET", "/api/operator/problems", "")); rec.Code != 401 {
		t.Fatal(rec.Code)
	}
	if rec := do(e, jsonReq("GET", "/api/operator/problems", "", sessionCookie(d, "senior", ""))); rec.Code != 403 {
		t.Fatal(rec.Code)
	}
	if rec := do(e, jsonReq("GET", "/api/senior/problems", "", operatorCookie(d, "branch-kyiv-12"))); rec.Code != 403 {
		t.Fatal(rec.Code)
	}
}

func TestTrack_PhoneGatesDetails(t *testing.T) {
	d := testDeps(t)
	ctx := context.Background()
	e := NewServer(d)
	ingest(t, e, "ingest", scenario2Events())
	d.Store.SaveNPStatus(ctx, []contract.TrackingStatus{{Number: contract.TTN(1), StatusCode: "6", Status: "У місті одержувача", ScheduledDeliveryDate: "18-10-2026 15:00:00"}}, time.Now())
	rec := do(e, jsonReq("GET", "/api/track/"+contract.TTN(1)+"?phone=0991112233", ""))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "details") || !strings.Contains(rec.Body.String(), `"isForecast":true`) {
		t.Fatalf("wrong phone: %d %s", rec.Code, rec.Body)
	}
	rec = do(e, jsonReq("GET", "/api/track/"+contract.TTN(1)+"?phone=0670000001", ""))
	if !strings.Contains(rec.Body.String(), `"details"`) || !strings.Contains(rec.Body.String(), `"eta":"2026-10-18T12:00:00Z"`) {
		t.Fatalf("right phone: %s", rec.Body)
	}
	if rec := do(e, jsonReq("GET", "/api/track/"+contract.TTN(9), "")); rec.Code != 404 {
		t.Fatal(rec.Code)
	}
	if rec := do(e, jsonReq("GET", "/api/track/abc", "")); rec.Code != 400 {
		t.Fatal(rec.Code)
	}
}

func TestReturn_RequiresReasonWithSolutionAndFlagsRisky(t *testing.T) {
	d := testDeps(t)
	e := NewServer(d)
	op := operatorCookie(d, "branch-kyiv-12")
	ingest(t, e, "ingest", scenario2Events())
	d.Sim.Jump(context.Background(), t0.Add(5*time.Hour)) // broken chain (orange)
	if rec := act(t, e, op, contract.TTN(1), "return", `{}`); rec.Code != 400 {
		t.Fatalf("no reason: %d", rec.Code)
	}
	if rec := act(t, e, op, contract.TTN(1), "return", `{"reason":"other"}`); rec.Code != 400 {
		t.Fatalf("other without comment: %d", rec.Code)
	}
	if rec := act(t, e, op, contract.TTN(1), "return", `{"reason":"damaged"}`); rec.Code != 200 {
		t.Fatalf("damaged: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, e, op, contract.TTN(1), "explode", `{}`); rec.Code != 400 {
		t.Fatalf("bad action: %d", rec.Code)
	}
	rows, code := getJSON[[]problemRow](t, e, "/api/senior/problems", sessionCookie(d, "senior", ""))
	found := false
	for _, r := range rows {
		for _, p := range r.Problems {
			found = found || (r.TTN == contract.TTN(1) && p.Kind == "risky_return")
		}
	}
	if code != 200 || !found {
		t.Fatalf("%d %+v", code, rows)
	}
}

func TestSplit_ShowsClientAndOperatorViews(t *testing.T) {
	d := testDeps(t)
	ctx := context.Background()
	e := NewServer(d)
	ingest(t, e, "ingest", scenario2Events())
	d.Store.SaveNPStatus(ctx, []contract.TrackingStatus{{Number: contract.TTN(1), StatusCode: "6", Status: "x", ScheduledDeliveryDate: "18-10-2026 15:00:00"}}, time.Now())
	d.Sim.Jump(ctx, t0.Add(5*time.Hour))
	d.Sim.SetMode(ctx, sim.ModeAsIs)
	type split struct {
		Client struct {
			StatusCode string `json:"statusCode"`
		} `json:"client"`
		Operator struct {
			VisibleInIncoming bool   `json:"visibleInIncoming"`
			Level             string `json:"level"`
		} `json:"operator"`
	}
	s, code := getJSON[split](t, e, "/api/sim/split/"+contract.TTN(1), adminCookie(d))
	if code != 200 || s.Client.StatusCode != "6" || s.Operator.VisibleInIncoming || s.Operator.Level != "green" {
		t.Fatalf("as is: %d %+v", code, s)
	}
	d.Sim.SetMode(ctx, sim.ModeWithSolution)
	s, _ = getJSON[split](t, e, "/api/sim/split/"+contract.TTN(1), adminCookie(d))
	if !s.Operator.VisibleInIncoming || s.Operator.Level != "orange" {
		t.Fatalf("with solution: %+v", s)
	}
}

func TestPhysical_ProxiesEmulator(t *testing.T) {
	d := testDepsEmu(t, fakeEmulator(t, map[string][]string{"branch-kyiv-12": {contract.TTN(1)}}).URL)
	e := NewServer(d)
	rows, code := getJSON[[]string](t, e, "/api/operator/physical", operatorCookie(d, "branch-kyiv-12"))
	if code != 200 || len(rows) != 1 {
		t.Fatalf("%d %+v", code, rows)
	}
	d2 := testDepsEmu(t, "http://127.0.0.1:1")
	if rec := do(NewServer(d2), jsonReq("GET", "/api/operator/physical", "", operatorCookie(d2, "branch-kyiv-12"))); rec.Code != 502 {
		t.Fatalf("emulator down: %d", rec.Code)
	}
}

func readDataLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			return line
		}
	}
}

func TestStream_DeliversInvalidate(t *testing.T) {
	d := testDeps(t)
	srv := httptest.NewServer(NewServer(d))
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/api/stream", nil)
	req.AddCookie(adminCookie(d))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type %q", ct)
	}
	go func() { time.Sleep(50 * time.Millisecond); d.Hub.Publish("problems") }()
	line := readDataLine(t, bufio.NewReader(resp.Body))
	if !strings.Contains(line, `"type":"invalidate"`) || !strings.Contains(line, `"keys":["problems"]`) {
		t.Fatal(line)
	}
}

func TestStream_RequiresSession(t *testing.T) {
	if rec := do(NewServer(testDeps(t)), jsonReq("GET", "/api/stream", "")); rec.Code != 401 {
		t.Fatal(rec.Code)
	}
}

func TestHub_SlowSubscriberDoesNotBlock(t *testing.T) {
	h := NewHub()
	_, unsub := h.Subscribe()
	defer unsub()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			h.Publish("x")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a full subscriber")
	}
}

func TestClock_ReadableByAnySession(t *testing.T) {
	d := testDeps(t)
	e := NewServer(d)
	if rec := do(e, jsonReq("GET", "/api/clock", "")); rec.Code != 401 {
		t.Fatal(rec.Code)
	}
	rec := do(e, jsonReq("GET", "/api/clock", "", operatorCookie(d, "branch-kyiv-12")))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"mode":"with_solution"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
