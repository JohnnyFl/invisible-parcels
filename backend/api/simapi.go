package api

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"

	"novapost/backend/sim"
	"novapost/contract"
)

func errEmulator(c *echo.Context, err error) error {
	log.Printf("emulator: %v", err)
	return c.JSON(http.StatusBadGateway, map[string]string{"error": "emulator unavailable"})
}

func clockJSON(d Deps) map[string]any {
	return map[string]any{
		"virtualNow": d.Sim.Now().UTC().Truncate(time.Second).Format(time.RFC3339),
		"speed":      d.Sim.Clock().Speed,
		"mode":       d.Sim.Mode(),
	}
}

func simClock(d Deps) echo.HandlerFunc {
	return func(c *echo.Context) error { return c.JSON(http.StatusOK, clockJSON(d)) }
}

func simSetClock(d Deps) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var req struct {
			Speed  *float64   `json:"speed"`
			JumpTo *time.Time `json:"jumpTo"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		ctx := c.Request().Context()
		if req.JumpTo != nil {
			if err := d.Sim.Jump(ctx, *req.JumpTo); err != nil {
				return errEmulator(c, err)
			}
		}
		if req.Speed != nil {
			if *req.Speed < 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "speed must be >= 0")
			}
			if err := d.Sim.SetSpeed(ctx, *req.Speed); err != nil {
				return errEmulator(c, err)
			}
		}
		d.Hub.Publish("clock", "problems", "incoming")
		return c.JSON(http.StatusOK, clockJSON(d))
	}
}

func simSetMode(d Deps) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var req struct {
			Mode sim.Mode `json:"mode"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if err := d.Sim.SetMode(c.Request().Context(), req.Mode); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		d.Hub.Publish("clock", "problems", "incoming")
		return c.JSON(http.StatusOK, clockJSON(d))
	}
}

func simReset(d Deps) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		if err := d.Emu.Reset(ctx); err != nil {
			return errEmulator(c, err)
		}
		if err := d.Store.Reset(ctx); err != nil {
			return err
		}
		if err := d.Sim.SetSpeed(ctx, 0); err != nil {
			return errEmulator(c, err)
		}
		d.Hub.Publish("clock", "problems", "incoming", "track")
		return c.JSON(http.StatusOK, clockJSON(d))
	}
}

// simScenario: reset both sides, load the scenario, rewind to its start and run at ×60.
func simScenario(d Deps) echo.HandlerFunc {
	return func(c *echo.Context) error {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil || id < 1 || id > 5 {
			return echo.NewHTTPError(http.StatusBadRequest, "scenario must be 1..5")
		}
		ctx := c.Request().Context()
		if err := d.Emu.Reset(ctx); err != nil {
			return errEmulator(c, err)
		}
		if err := d.Store.Reset(ctx); err != nil {
			return err
		}
		if err := d.Emu.LoadScenario(ctx, id); err != nil {
			return errEmulator(c, err)
		}
		if err := d.Sim.Jump(ctx, contract.ScenarioStart); err != nil {
			return errEmulator(c, err)
		}
		if err := d.Sim.SetSpeed(ctx, 60); err != nil {
			return errEmulator(c, err)
		}
		d.Hub.Publish("clock", "problems", "incoming", "track")
		return c.JSON(http.StatusOK, clockJSON(d))
	}
}
