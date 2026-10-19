package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"novapost/contract"
	"novapost/emulator/scenarios"
	"novapost/emulator/world"
)

func post(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestGetStatusDocuments(t *testing.T) {
	w := world.New()
	facts, _ := scenarios.Build(2)
	w.Load(facts)
	clk := &ClockHolder{}
	clk.Set(contract.Clock{VirtualBase: scenarios.Start.Add(48 * time.Hour)})
	srv := httptest.NewServer(NewServer(w, clk, func(f func()) { f() }, func(time.Time) {}))
	defer srv.Close()
	body := `{"apiKey":"demo","modelName":"TrackingDocument","calledMethod":"getStatusDocuments","methodProperties":{"Documents":[{"DocumentNumber":"59000000000001","Phone":""},{"DocumentNumber":"59999999999999","Phone":""}]}}`
	var r contract.Response[contract.TrackingStatus]
	json.NewDecoder(post(t, srv.URL+"/v2.0/json/", body).Body).Decode(&r)
	if !r.Success || len(r.Data) != 2 || r.Data[0].StatusCode != "6" || r.Data[1].StatusCode != "3" {
		t.Fatalf("%+v", r)
	}
}

func TestGetStatusDocuments_RejectsOtherMethodsAndTooMany(t *testing.T) {
	srv := httptest.NewServer(NewServer(world.New(), &ClockHolder{}, func(f func()) { f() }, func(time.Time) {}))
	defer srv.Close()
	var r contract.Response[contract.TrackingStatus]
	json.NewDecoder(post(t, srv.URL+"/v2.0/json/", `{"modelName":"Address","calledMethod":"getCities","methodProperties":{}}`).Body).Decode(&r)
	if r.Success || len(r.Errors) == 0 {
		t.Fatalf("%+v", r)
	}
	docs := strings.Repeat(`{"DocumentNumber":"59000000000001"},`, 101)
	body := `{"modelName":"TrackingDocument","calledMethod":"getStatusDocuments","methodProperties":{"Documents":[` + strings.TrimSuffix(docs, ",") + `]}}`
	r = contract.Response[contract.TrackingStatus]{}
	json.NewDecoder(post(t, srv.URL+"/v2.0/json/", body).Body).Decode(&r)
	if r.Success {
		t.Fatal("101 documents accepted")
	}
}

func TestAdmin_ScenarioClockPhysicalOperatorEvent(t *testing.T) {
	w := world.New()
	clk := &ClockHolder{}
	cleared := false
	srv := httptest.NewServer(NewServer(w, clk, func(f func()) { f(); cleared = true }, func(time.Time) {}))
	defer srv.Close()
	if resp := post(t, srv.URL+"/admin/scenarios/2", ``); resp.StatusCode != 200 || !cleared {
		t.Fatalf("scenario: %d cleared=%v", resp.StatusCode, cleared)
	}
	if resp := post(t, srv.URL+"/admin/scenarios/9", ``); resp.StatusCode != 400 {
		t.Fatalf("bad scenario: %d", resp.StatusCode)
	}
	vt := scenarios.Start.Add(48 * time.Hour).Format(time.RFC3339)
	post(t, srv.URL+"/admin/clock", `{"virtualNow":"`+vt+`","speed":0}`)
	resp, _ := http.Get(srv.URL + "/admin/physical?location=branch-kyiv-12")
	var ph struct {
		TTNs []string `json:"ttns"`
	}
	json.NewDecoder(resp.Body).Decode(&ph)
	if !strings.Contains(strings.Join(ph.TTNs, ","), scenarios.FocusTTN) {
		t.Fatalf("physical: %+v", ph)
	}
	ev := `{"EventId":"op1","Kind":"arrived_branch","TTN":"` + scenarios.FocusTTN + `","LocationId":"branch-kyiv-12","OccurredAt":"` + vt + `","Source":"operator"}`
	post(t, srv.URL+"/admin/operator-event", ev)
	if st, _ := w.Track(scenarios.FocusTTN, scenarios.Start.Add(48*time.Hour)); st.StatusCode != "7" {
		t.Fatalf("%+v", st)
	}
}

func TestAdmin_StateReportsLoadedScenario(t *testing.T) {
	srv := httptest.NewServer(NewServer(world.New(), &ClockHolder{}, func(f func()) { f() }, func(time.Time) {}))
	defer srv.Close()
	state := func() int {
		resp, _ := http.Get(srv.URL + "/admin/state")
		var s struct {
			Scenario int `json:"scenario"`
		}
		json.NewDecoder(resp.Body).Decode(&s)
		return s.Scenario
	}
	if state() != 0 {
		t.Fatal("fresh emulator should report no scenario")
	}
	post(t, srv.URL+"/admin/scenarios/3", ``)
	if got := state(); got != 3 {
		t.Fatalf("after load: %d", got)
	}
	post(t, srv.URL+"/admin/reset", ``)
	if got := state(); got != 0 {
		t.Fatalf("after reset: %d", got)
	}
}

func TestAdmin_ClockFlushesDueEventsBeforeReplying(t *testing.T) {
	var flushedAt time.Time
	srv := httptest.NewServer(NewServer(world.New(), &ClockHolder{}, func(f func()) { f() }, func(now time.Time) { flushedAt = now }))
	defer srv.Close()
	vt := scenarios.Start.Add(30 * time.Hour)
	post(t, srv.URL+"/admin/clock", `{"virtualNow":"`+vt.Format(time.RFC3339)+`","speed":0}`)
	if !flushedAt.Equal(vt) {
		t.Fatalf("flushed at %v, want %v", flushedAt, vt)
	}
}
