package push

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"novapost/contract"
	"novapost/emulator/world"
)

func TestPusherSignsBody(t *testing.T) {
	var got contract.Envelope[contract.EventBatch]
	ok := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ok = r.URL.Path == "/api/ingest/events" && contract.Verify(body, "s", r.Header.Get("X-Signature"))
		json.Unmarshal(body, &got)
	}))
	defer srv.Close()
	p := &Pusher{URL: srv.URL, Secret: "s", Client: srv.Client()}
	if err := p.Send(context.Background(), []contract.Event{{EventID: "e1", Kind: contract.KindCreated}}); err != nil {
		t.Fatal(err)
	}
	if !ok || got.ModelName != "ScanEvent" || got.CalledMethod != "push" || got.MethodProperties.Events[0].EventID != "e1" {
		t.Fatalf("ok=%v %+v", ok, got)
	}
}

func TestPusherErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer srv.Close()
	p := &Pusher{URL: srv.URL, Secret: "s", Client: srv.Client()}
	if err := p.Send(context.Background(), []contract.Event{{EventID: "e1"}}); err == nil {
		t.Fatal("expected error")
	}
}

func TestReset_WaitsForInFlightSend(t *testing.T) {
	started, finished := make(chan struct{}), make(chan struct{})
	var done atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(150 * time.Millisecond)
		done.Store(true)
		close(finished)
	}))
	defer srv.Close()
	w := world.New()
	w.Load([]world.Fact{{Event: contract.Event{EventID: "e1", Kind: contract.KindCreated, OccurredAt: time.Unix(0, 0)}}})
	p := &Pusher{URL: srv.URL, Secret: "s", Client: srv.Client()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx, w, func() time.Time { return time.Unix(10, 0) })
	<-started
	loadedAfterSend := false
	p.Reset(func() { loadedAfterSend = done.Load() })
	if !loadedAfterSend {
		t.Fatal("world replaced while a send was still in flight")
	}
	<-finished
}

func TestFlush_SendsEverythingDueAtOnce(t *testing.T) {
	var got contract.Envelope[contract.EventBatch]
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()
	w := world.New()
	var facts []world.Fact
	for i := range 10 {
		facts = append(facts, world.Fact{Event: contract.Event{EventID: contract.TTN(i), Kind: contract.KindCreated, OccurredAt: time.Unix(int64(i), 0)}})
	}
	w.Load(facts)
	p := &Pusher{URL: srv.URL, Secret: "s", Client: srv.Client()}
	p.Flush(context.Background(), w, time.Unix(100, 0))
	if calls != 1 || len(got.MethodProperties.Events) != 10 {
		t.Fatalf("calls=%d events=%d", calls, len(got.MethodProperties.Events))
	}
}
