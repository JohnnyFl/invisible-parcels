// Package api serves the NP v2.0 tracking endpoint and the demo admin API.
package api

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"novapost/contract"
	"novapost/emulator/scenarios"
	"novapost/emulator/world"
)

type ClockHolder struct {
	mu sync.Mutex
	c  contract.Clock
}

func (h *ClockHolder) Get() contract.Clock {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.c
}

func (h *ClockHolder) Set(c contract.Clock) {
	h.mu.Lock()
	h.c = c
	h.mu.Unlock()
}

func (h *ClockHolder) Now() time.Time { return h.Get().Now(time.Now()) }

const maxDocuments = 100

// NewServer wires the routes. reset(load) must run load (replace the world) and
// drop queued pushes atomically.
func NewServer(w *world.World, clk *ClockHolder, reset func(load func())) *echo.Echo {
	e := echo.New()

	e.POST("/v2.0/json/", func(c *echo.Context) error {
		var req contract.Envelope[contract.TrackingRequest]
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusOK, contract.Response[contract.TrackingStatus]{Errors: []string{"Invalid request"}})
		}
		if req.ModelName != "TrackingDocument" || req.CalledMethod != "getStatusDocuments" {
			return c.JSON(http.StatusOK, contract.Response[contract.TrackingStatus]{Errors: []string{"Method not supported"}})
		}
		if len(req.MethodProperties.Documents) > maxDocuments {
			return c.JSON(http.StatusOK, contract.Response[contract.TrackingStatus]{Errors: []string{"Documents limit is 100"}})
		}
		now := clk.Now()
		data := make([]contract.TrackingStatus, 0, len(req.MethodProperties.Documents))
		for _, d := range req.MethodProperties.Documents {
			st, ok := w.Track(d.DocumentNumber, now)
			if !ok {
				st = contract.TrackingStatus{Number: d.DocumentNumber, StatusCode: contract.StatusNotFound, Status: contract.StatusText(contract.StatusNotFound)}
			}
			data = append(data, st)
		}
		return c.JSON(http.StatusOK, contract.Response[contract.TrackingStatus]{Success: true, Data: data})
	})

	e.POST("/admin/clock", func(c *echo.Context) error {
		var req struct {
			VirtualNow time.Time `json:"virtualNow"`
			Speed      float64   `json:"speed"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		clk.Set(contract.Clock{RealBase: time.Now(), VirtualBase: req.VirtualNow, Speed: req.Speed})
		return c.NoContent(http.StatusNoContent)
	})

	e.POST("/admin/reset", func(c *echo.Context) error {
		reset(func() { w.Load(nil) })
		return c.NoContent(http.StatusNoContent)
	})

	e.POST("/admin/scenarios/:id", func(c *echo.Context) error {
		id, _ := strconv.Atoi(c.Param("id"))
		facts, err := scenarios.Build(id)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		reset(func() { w.Load(facts) })
		return c.JSON(http.StatusOK, map[string]any{"facts": len(facts)})
	})

	e.GET("/admin/physical", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string][]string{"ttns": w.Physical(c.QueryParam("location"), clk.Now())})
	})

	e.POST("/admin/operator-event", func(c *echo.Context) error {
		var ev contract.Event
		if err := c.Bind(&ev); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		w.Apply(ev)
		return c.NoContent(http.StatusNoContent)
	})

	return e
}
