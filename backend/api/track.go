package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"novapost/contract"
)

type TrackDetails struct {
	City      string `json:"city"`
	Warehouse string `json:"warehouse"`
}

type Track struct {
	TTN        string        `json:"ttn"`
	StatusCode string        `json:"statusCode"`
	Status     string        `json:"status"`
	ETA        *time.Time    `json:"eta,omitempty"`
	IsForecast bool          `json:"isForecast"`
	Details    *TrackDetails `json:"details,omitempty"`
}

// samePhone compares the last 9 digits, so "+380670000001" matches "0670000001".
func samePhone(a, b string) bool {
	digits := func(s string) string {
		var sb strings.Builder
		for _, r := range s {
			if r >= '0' && r <= '9' {
				sb.WriteRune(r)
			}
		}
		d := sb.String()
		return d[max(0, len(d)-9):]
	}
	da, db := digits(a), digits(b)
	return len(da) == 9 && da == db
}

// track is what the client app shows: the NP snapshot. ok=false when NP has no status yet.
func track(ctx context.Context, d Deps, ttn string, withDetails bool, phone string) (Track, bool, error) {
	code, text, sched, ok, err := d.Store.NPStatus(ctx, ttn)
	if err != nil || !ok {
		return Track{}, false, err
	}
	t := Track{TTN: ttn, StatusCode: code, Status: text, IsForecast: code == contract.StatusInTransit || code == contract.StatusInRecipientCity}
	if sched != nil {
		u := sched.UTC()
		t.ETA = &u
	}
	p, _, err := d.Store.Parcel(ctx, ttn)
	if err != nil {
		return Track{}, false, err
	}
	if p != nil && (withDetails || samePhone(phone, p.RecipientPhone) || samePhone(phone, p.SenderPhone)) {
		if l, ok := contract.LocationByID(p.DestinationID); ok {
			t.Details = &TrackDetails{City: l.City, Warehouse: l.Name}
		}
	}
	return t, true, nil
}

func trackHandler(d Deps) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ttn := c.Param("ttn")
		if !contract.TTNPattern.MatchString(ttn) {
			return badTTN(c)
		}
		t, ok, err := track(c.Request().Context(), d, ttn, false, c.QueryParam("phone"))
		if err != nil {
			return err
		}
		if !ok {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Посилку не знайдено"})
		}
		return c.JSON(http.StatusOK, t)
	}
}
