// Package contract holds the wire format shared by the emulator ("Nova Poshta")
// and the backend: the NP v2.0 envelope, scan events and seed locations.
package contract

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
	_ "time/tzdata" // distroless images have no tzdata; NP dates are Europe/Kyiv
)

// Envelope is the NP API v2.0 request body.
type Envelope[T any] struct {
	APIKey           string `json:"apiKey,omitempty"`
	ModelName        string `json:"modelName"`
	CalledMethod     string `json:"calledMethod"`
	MethodProperties T      `json:"methodProperties"`
}

// Response is the NP API v2.0 response body.
type Response[T any] struct {
	Success  bool     `json:"success"`
	Data     []T      `json:"data"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
	Info     []string `json:"info"`
}

// MarshalJSON renders nil slices as [] like the real API does.
func (r Response[T]) MarshalJSON() ([]byte, error) {
	type plain Response[T]
	p := plain(r)
	if p.Data == nil {
		p.Data = []T{}
	}
	for _, s := range []*[]string{&p.Errors, &p.Warnings, &p.Info} {
		if *s == nil {
			*s = []string{}
		}
	}
	return json.Marshal(p)
}

type TrackingRequest struct {
	Documents []Document `json:"Documents"`
}

type Document struct {
	DocumentNumber string
	Phone          string
}

type TrackingStatus struct {
	Number                string
	StatusCode            string
	Status                string
	ScheduledDeliveryDate string
	WarehouseRecipient    string
	CityRecipient         string
}

// NPDateLayout is the date format NP uses, e.g. "18-10-2026 15:00:00".
const NPDateLayout = "02-01-2006 15:04:05"

// Kyiv is the time zone of NP dates.
var Kyiv = mustLoad("Europe/Kyiv")

func mustLoad(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

// Event is one scan / trip / operator event. Proposed contract (NP's internal format is not public).
type Event struct {
	EventID    string `json:"EventId"`
	Kind       string
	TTN        string
	TripID     string `json:"TripId"`
	LocationID string `json:"LocationId"`
	OccurredAt time.Time
	Source     string
	Payload    json.RawMessage `json:",omitempty"`
}

// EventBatch is the methodProperties of a ScanEvent.push envelope.
type EventBatch struct {
	Events []Event
}

type RouteLeg struct {
	Seq              int
	FromID           string
	ToID             string
	TripID           string
	PlannedDeparture time.Time
	PlannedArrival   time.Time
}

type CreatedPayload struct {
	SenderPhone    string
	RecipientPhone string
	OriginID       string
	DestinationID  string
	RouteLegs      []RouteLeg
}

type IncidentPayload struct{ Cause string }
type RedirectPayload struct{ NextLocationID string }
type ReturnPayload struct{ Reason, Comment string }

const (
	KindCreated         = "created"
	KindAccepted        = "accepted"
	KindLoaded          = "loaded"
	KindTripDeparted    = "trip_departed"
	KindTripArrived     = "trip_arrived"
	KindArrivedHub      = "arrived_hub"
	KindSorted          = "sorted"
	KindUnloaded        = "unloaded"
	KindArrivedBranch   = "arrived_branch"
	KindIssued          = "issued"
	KindRedirected      = "redirected"
	KindReturnInitiated = "return_initiated"
	KindDamaged         = "damaged"
	KindIncident        = "incident"
	KindOperatorScan    = "operator_scan"
	KindHold            = "hold"

	// management events: decisions about a parcel, not its movement
	KindCheckRequested  = "check_requested"  // senior asks a branch to look for the parcel
	KindCheckFound      = "check_found"      // operator: found it
	KindCheckNotFound   = "check_not_found"  // operator: not found
	KindReturnConfirmed = "return_confirmed" // senior: the return is justified
)

// IsManagement: these events record decisions, they are not scans and never move a parcel.
func IsManagement(kind string) bool {
	switch kind {
	case KindCheckRequested, KindCheckFound, KindCheckNotFound, KindReturnConfirmed:
		return true
	}
	return false
}

type CheckPayload struct{ Note string }
type ConfirmPayload struct{ Comment string }

// NP StatusCode values (from public integrations; verify against official docs).
const (
	StatusCreated         = "1"
	StatusNotFound        = "3"
	StatusAccepted        = "4"
	StatusInTransit       = "5"
	StatusInRecipientCity = "6"
	StatusArrived         = "7"
	StatusIssued          = "9"
	StatusRedirected      = "104"
	StatusReturn          = "106"
)

var statusTexts = map[string]string{
	StatusCreated:         "Відправник створив накладну, але ще не надав до відправки",
	StatusNotFound:        "Номер не знайдено",
	StatusAccepted:        "Відправлення у місті відправника",
	StatusInTransit:       "Відправлення прямує до міста одержувача",
	StatusInRecipientCity: "Відправлення у місті одержувача, орієнтовна доставка до відділення",
	StatusArrived:         "Прибув у відділення",
	StatusIssued:          "Відправлення отримано",
	StatusRedirected:      "Змінено адресу",
	StatusReturn:          "Створено зворотну накладну",
}

func StatusText(code string) string { return statusTexts[code] }

type Location struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	City string `json:"city"`
	Name string `json:"name"`
}

var Locations = []Location{
	{"branch-kyiv-12", "branch", "Київ", "Відділення №12"},
	{"branch-lviv-5", "branch", "Львів", "Відділення №5"},
	{"branch-kharkiv-3", "branch", "Харків", "Відділення №3"},
	{"hub-kyiv", "hub", "Київ", "Сортувальний центр Київ"},
	{"hub-lviv", "hub", "Львів", "Сортувальний центр Львів"},
	{"hub-kharkiv", "hub", "Харків", "Сортувальний центр Харків"},
}

func LocationByID(id string) (Location, bool) {
	for _, l := range Locations {
		if l.ID == id {
			return l, true
		}
	}
	return Location{}, false
}

// ScenarioStart is virtual time zero of every scenario.
var ScenarioStart = time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)

var TTNPattern = regexp.MustCompile(`^59\d{12}$`)

func TTN(n int) string { return fmt.Sprintf("59%012d", n) }
