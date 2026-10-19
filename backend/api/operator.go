package api

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"novapost/backend/auth"
	"novapost/backend/problems"
	"novapost/backend/sim"
	"novapost/backend/store"
	"novapost/contract"
)

type ProblemRow struct {
	TTN         string             `json:"ttn"`
	Level       problems.Severity  `json:"level"`
	Problems    []problems.Problem `json:"problems"`
	Destination string             `json:"destination"`
	Location    string             `json:"location"`
	Check       *CheckState        `json:"check,omitempty"` // senior view only
}

// CheckState is the latest "please look for it" request and its answer.
type CheckState struct {
	Status string    `json:"status"` // requested | found | not_found
	Branch string    `json:"branch"`
	At     time.Time `json:"at"`
}

func checkOf(evs []contract.Event) *CheckState {
	var c *CheckState
	for _, e := range evs {
		switch e.Kind {
		case contract.KindCheckRequested:
			c = &CheckState{Status: "requested", Branch: e.LocationID, At: e.OccurredAt}
		case contract.KindCheckFound:
			c = &CheckState{Status: "found", Branch: e.LocationID, At: e.OccurredAt}
		case contract.KindCheckNotFound:
			c = &CheckState{Status: "not_found", Branch: e.LocationID, At: e.OccurredAt}
		}
	}
	return c
}

// problemRows classifies TTNs and keeps the non-green ones, worst first, then oldest.
// The final destination is shown only to the senior shift (spec §8: an operator's
// scan must not reveal where a parcel is going).
func problemRows(ctx context.Context, d Deps, ttns []string, withDestination bool) ([]ProblemRow, error) {
	out := []ProblemRow{}
	if d.Sim.Mode() == sim.ModeAsIs {
		return out, nil // today's systems have no such screen
	}
	inputs, err := d.Store.Inputs(ctx, ttns, d.Sim.Now())
	if err != nil {
		return nil, err
	}
	for _, in := range inputs {
		ps := problems.Classify(in)
		lvl := problems.Level(ps)
		if lvl == problems.Green {
			continue
		}
		row := ProblemRow{TTN: in.TTN, Level: lvl, Problems: ps, Location: lastLocation(in.Events)}
		if in.Parcel != nil && withDestination {
			row.Destination = in.Parcel.DestinationID
		}
		if withDestination {
			row.Check = checkOf(in.Events)
		}
		out = append(out, row)
	}
	slices.SortFunc(out, func(a, b ProblemRow) int {
		return cmp.Or(cmp.Compare(b.Level, a.Level), oldest(a).Compare(oldest(b)), cmp.Compare(a.TTN, b.TTN))
	})
	return out, nil
}

func oldest(r ProblemRow) time.Time {
	t := r.Problems[0].Since
	for _, p := range r.Problems {
		if p.Since.Before(t) {
			t = p.Since
		}
	}
	return t
}

func lastLocation(evs []contract.Event) string {
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].TTN != "" && evs[i].LocationID != "" {
			return evs[i].LocationID
		}
	}
	return ""
}

// limiter is a per-branch sliding window over real time.
// ponytail: in-process, resets on restart; move to Postgres/Redis for several instances.
type limiter struct {
	mu    sync.Mutex
	limit int
	hits  map[string][]time.Time
}

func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.limit {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)
	return true
}

func newEventID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return "op-" + hex.EncodeToString(b)
}

// journal writes an event to the backend journal only (decisions, not movement).
func journal(ctx context.Context, d Deps, source, kind, ttn, location string, payload any) (contract.Event, error) {
	e := contract.Event{EventID: newEventID(), Kind: kind, TTN: ttn, LocationID: location, OccurredAt: d.Sim.Now(), Source: source}
	if payload != nil {
		e.Payload, _ = json.Marshal(payload)
	}
	if _, err := d.Store.InsertEvents(ctx, []contract.Event{e}, time.Now()); err != nil {
		return e, err
	}
	d.Hub.Publish("problems", "incoming", "track", "physical", "tasks")
	return e, nil
}

// record writes an operator scan/action to the journal and mirrors it into the NP world.
func record(ctx context.Context, d Deps, kind, ttn, branch string, payload any) error {
	e, err := journal(ctx, d, "operator", kind, ttn, branch, payload)
	if err != nil {
		return err
	}
	if err := d.Emu.OperatorEvent(ctx, e); err != nil {
		log.Printf("emulator: operator event not mirrored: %v", err)
	}
	return nil
}

func hubOfCity(branchID string) string {
	b, _ := contract.LocationByID(branchID)
	for _, l := range contract.Locations {
		if l.Kind == "hub" && l.City == b.City {
			return l.ID
		}
	}
	return ""
}

// nextLocation: the next stop of the parcel's plan after this branch, or the
// city hub when the branch is not on its route at all.
func nextLocation(legs []contract.RouteLeg, branch string) string {
	for _, l := range legs {
		if l.FromID == branch {
			return l.ToID
		}
	}
	return hubOfCity(branch)
}

func badTTN(c *echo.Context) error {
	return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid TTN"})
}

func registerOperator(g *echo.Group, d Deps) {
	lim := &limiter{limit: d.Cfg.LookupLimitPerHour, hits: map[string][]time.Time{}}

	g.GET("/problems", func(c *echo.Context) error {
		ctx := c.Request().Context()
		ttns, err := d.Store.BranchTTNs(ctx, auth.FromContext(c).BranchID)
		if err != nil {
			return err
		}
		rows, err := problemRows(ctx, d, ttns, false)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, rows)
	})

	// An operator may trace only parcels addressed to, routed through or seen at their
	// branch — the same set they already see — so the route of others' parcels stays hidden (§8).
	g.GET("/trace/:ttn", func(c *echo.Context) error {
		ttn := c.Param("ttn")
		if !contract.TTNPattern.MatchString(ttn) {
			return badTTN(c)
		}
		ctx := c.Request().Context()
		s := auth.FromContext(c)
		mine, err := d.Store.BranchTTNs(ctx, s.BranchID)
		if err != nil {
			return err
		}
		d.Store.Audit(ctx, time.Now(), s.Role, s.BranchID, "trace", ttn, fmt.Sprint(slices.Contains(mine, ttn)))
		if !slices.Contains(mine, ttn) {
			return notFound(c)
		}
		tr, ok, err := traceOf(ctx, d, ttn)
		if err != nil {
			return err
		}
		if !ok {
			return notFound(c)
		}
		return c.JSON(http.StatusOK, tr)
	})

	g.GET("/tasks", func(c *echo.Context) error {
		tasks, err := d.Store.OpenTasks(c.Request().Context(), auth.FromContext(c).BranchID)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, nonNil(tasks))
	})

	g.POST("/tasks/:ttn/answer", func(c *echo.Context) error {
		ttn := c.Param("ttn")
		if !contract.TTNPattern.MatchString(ttn) {
			return badTTN(c)
		}
		var req struct {
			Found bool `json:"found"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
		}
		ctx := c.Request().Context()
		s := auth.FromContext(c)
		tasks, err := d.Store.OpenTasks(ctx, s.BranchID)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(tasks, func(t store.Task) bool { return t.TTN == ttn }) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Доручення не знайдено"})
		}
		kind := contract.KindCheckNotFound
		if req.Found {
			kind = contract.KindCheckFound
		}
		if _, err := journal(ctx, d, "operator", kind, ttn, s.BranchID, nil); err != nil {
			return err
		}
		d.Store.Audit(ctx, time.Now(), s.Role, s.BranchID, "check_answer", ttn, kind)
		return c.JSON(http.StatusOK, map[string]string{"result": "ok"})
	})

	g.GET("/incoming", func(c *echo.Context) error {
		rows, err := d.Store.Incoming(c.Request().Context(), auth.FromContext(c).BranchID, d.Sim.Mode() == sim.ModeWithSolution)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, nonNil(rows))
	})

	// Demo-only shelf: what physically lies here, unregistered parcels first —
	// those are the "invisible" ones the product is about.
	g.GET("/physical", func(c *echo.Context) error {
		ctx := c.Request().Context()
		branch := auth.FromContext(c).BranchID
		ttns, err := d.Emu.Physical(ctx, branch)
		if err != nil {
			return errEmulator(c, err)
		}
		known, err := d.Store.RegisteredAt(ctx, branch, ttns)
		if err != nil {
			return err
		}
		type shelfRow struct {
			TTN        string `json:"ttn"`
			Registered bool   `json:"registered"`
		}
		rows := make([]shelfRow, len(ttns))
		for i, t := range ttns {
			rows[i] = shelfRow{t, known[t]}
		}
		slices.SortStableFunc(rows, func(a, b shelfRow) int {
			return cmp.Compare(boolInt(a.Registered), boolInt(b.Registered))
		})
		return c.JSON(http.StatusOK, rows)
	})

	// Q11: only for an operator of a branch, minimal answer, rate limited, audited.
	g.POST("/scan", func(c *echo.Context) error {
		var req struct {
			TTN string `json:"ttn"`
		}
		c.Bind(&req)
		if !contract.TTNPattern.MatchString(req.TTN) {
			return badTTN(c)
		}
		ctx := c.Request().Context()
		s := auth.FromContext(c)
		if !lim.allow(s.BranchID, time.Now()) {
			d.Store.Audit(ctx, time.Now(), s.Role, s.BranchID, "scan", req.TTN, "rate_limited")
			return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "Перевищено ліміт пошуків"})
		}
		if err := record(ctx, d, contract.KindOperatorScan, req.TTN, s.BranchID, nil); err != nil {
			return err
		}
		p, legs, err := d.Store.Parcel(ctx, req.TTN)
		if err != nil {
			return err
		}
		res := map[string]string{}
		switch {
		case p != nil && p.DestinationID == s.BranchID:
			res["outcome"] = "accept"
		case d.Sim.Mode() == sim.ModeAsIs:
			res["outcome"] = "not_found"
		case p == nil:
			res["outcome"] = "hold"
		default:
			res["outcome"], res["nextLocation"] = "redirect", nextLocation(legs, s.BranchID)
		}
		d.Store.Audit(ctx, time.Now(), s.Role, s.BranchID, "scan", req.TTN, res["outcome"])
		return c.JSON(http.StatusOK, res)
	})

	g.POST("/parcels/:ttn/:action", func(c *echo.Context) error {
		ttn, action := c.Param("ttn"), c.Param("action")
		if !contract.TTNPattern.MatchString(ttn) {
			return badTTN(c)
		}
		ctx := c.Request().Context()
		s := auth.FromContext(c)
		var kind string
		var payload any
		switch action {
		case "accept":
			kind = contract.KindArrivedBranch
		case "hold":
			kind = contract.KindHold
		case "redirect":
			_, legs, err := d.Store.Parcel(ctx, ttn)
			if err != nil {
				return err
			}
			kind, payload = contract.KindRedirected, contract.RedirectPayload{NextLocationID: nextLocation(legs, s.BranchID)}
		case "return":
			var r contract.ReturnPayload
			c.Bind(&r)
			if d.Sim.Mode() == sim.ModeWithSolution {
				if r.Reason != "damaged" && r.Reason != "refused" && r.Reason != "other" {
					return c.JSON(http.StatusBadRequest, map[string]string{"error": "Вкажіть причину повернення"})
				}
				if r.Reason == "other" && r.Comment == "" {
					return c.JSON(http.StatusBadRequest, map[string]string{"error": "Опишіть причину"})
				}
			}
			kind, payload = contract.KindReturnInitiated, r
		default:
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "unknown action"})
		}
		if err := record(ctx, d, kind, ttn, s.BranchID, payload); err != nil {
			return err
		}
		d.Store.Audit(ctx, time.Now(), s.Role, s.BranchID, action, ttn, "ok")
		return c.JSON(http.StatusOK, map[string]string{"result": "ok"})
	})
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
