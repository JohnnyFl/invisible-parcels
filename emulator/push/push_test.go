package push

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"novapost/contract"
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
