package api

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	"novapost/backend/problems"
	"novapost/contract"
)

// traceOf builds the trace for one TTN; ok=false when the system has never seen it.
func traceOf(ctx context.Context, d Deps, ttn string) (problems.Trace, bool, error) {
	inputs, err := d.Store.Inputs(ctx, []string{ttn}, d.Sim.Now())
	if err != nil {
		return problems.Trace{}, false, err
	}
	in := inputs[0]
	if in.Parcel == nil && len(in.Events) == 0 {
		return problems.Trace{}, false, nil
	}
	return problems.TraceParcel(in), true, nil
}

func notFound(c *echo.Context) error {
	return c.JSON(http.StatusNotFound, map[string]string{"error": "Посилку не знайдено"})
}

func registerSenior(g *echo.Group, d Deps) {
	g.GET("/problems", func(c *echo.Context) error {
		ctx := c.Request().Context()
		ttns, err := d.Store.AllActiveTTNs(ctx)
		if err != nil {
			return err
		}
		rows, err := problemRows(ctx, d, ttns, true)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, rows)
	})

	g.GET("/trace/:ttn", func(c *echo.Context) error {
		ttn := c.Param("ttn")
		if !contract.TTNPattern.MatchString(ttn) {
			return badTTN(c)
		}
		tr, ok, err := traceOf(c.Request().Context(), d, ttn)
		if err != nil {
			return err
		}
		if !ok {
			return notFound(c)
		}
		return c.JSON(http.StatusOK, tr)
	})
}
