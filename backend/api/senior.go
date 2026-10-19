package api

import (
	"context"
	"net/http"
	"strings"
	"time"

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

	// "Доручити перевірку відділенню": the branch operator gets a task to look for the parcel.
	g.POST("/parcels/:ttn/check", func(c *echo.Context) error {
		ttn := c.Param("ttn")
		if !contract.TTNPattern.MatchString(ttn) {
			return badTTN(c)
		}
		var req struct {
			BranchID string `json:"branchId"`
			Note     string `json:"note"`
		}
		c.Bind(&req)
		if l, ok := contract.LocationByID(req.BranchID); !ok || l.Kind != "branch" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Оберіть відділення"})
		}
		if len([]rune(req.Note)) > 200 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Коментар до 200 символів"})
		}
		ctx := c.Request().Context()
		if _, err := journal(ctx, d, "senior", contract.KindCheckRequested, ttn, req.BranchID, contract.CheckPayload{Note: req.Note}); err != nil {
			return err
		}
		d.Store.Audit(ctx, time.Now(), "senior", "", "check_request", ttn, req.BranchID)
		return c.JSON(http.StatusOK, map[string]string{"result": "ok"})
	})

	// "Підтвердити повернення": the senior shift checked a risky return and accepts it.
	g.POST("/parcels/:ttn/confirm-return", func(c *echo.Context) error {
		ttn := c.Param("ttn")
		if !contract.TTNPattern.MatchString(ttn) {
			return badTTN(c)
		}
		var req struct {
			Comment string `json:"comment"`
		}
		c.Bind(&req)
		if strings.TrimSpace(req.Comment) == "" || len([]rune(req.Comment)) > 300 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Опишіть, чому повернення обґрунтоване"})
		}
		ctx := c.Request().Context()
		if _, err := journal(ctx, d, "senior", contract.KindReturnConfirmed, ttn, "", contract.ConfirmPayload{Comment: req.Comment}); err != nil {
			return err
		}
		d.Store.Audit(ctx, time.Now(), "senior", "", "confirm_return", ttn, "ok")
		return c.JSON(http.StatusOK, map[string]string{"result": "ok"})
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
