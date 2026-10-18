package contract

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSignVerify(t *testing.T) {
	body := []byte(`{"a":1}`)
	sig := Sign(body, "s")
	if !Verify(body, "s", sig) {
		t.Fatal("valid sig rejected")
	}
	if Verify(body, "other", sig) || Verify([]byte(`{"a":2}`), "s", sig) {
		t.Fatal("bad sig accepted")
	}
}

func TestClockNowAndRebase(t *testing.T) {
	r0 := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	v0 := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	c := Clock{RealBase: r0, VirtualBase: v0, Speed: 60}
	if got := c.Now(r0.Add(time.Minute)); !got.Equal(v0.Add(time.Hour)) {
		t.Fatalf("got %v", got)
	}
	p := c.Rebase(r0.Add(time.Minute), 0) // пауза
	if got := p.Now(r0.Add(time.Hour)); !got.Equal(v0.Add(time.Hour)) {
		t.Fatalf("paused clock moved: %v", got)
	}
}

func TestResponseEmptyArrays(t *testing.T) {
	b, _ := json.Marshal(Response[TrackingStatus]{Success: true})
	if !strings.Contains(string(b), `"errors":[]`) || !strings.Contains(string(b), `"data":[]`) {
		t.Fatal(string(b))
	}
}

func TestTTN(t *testing.T) {
	if TTN(12) != "59000000000012" || !TTNPattern.MatchString(TTN(12)) {
		t.Fatal(TTN(12))
	}
}

func TestLocations(t *testing.T) {
	l, ok := LocationByID("branch-kyiv-12")
	if !ok || l.Kind != "branch" || l.City != "Київ" || len(Locations) != 6 {
		t.Fatalf("%+v %v", l, ok)
	}
}
