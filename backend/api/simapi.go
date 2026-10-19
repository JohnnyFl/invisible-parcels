package api

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"

	"novapost/backend/problems"
	"novapost/backend/sim"
	"novapost/contract"
)

// allKeys: a clock, mode or scenario change can affect every screen.
var allKeys = []string{"clock", "problems", "incoming", "track", "physical"}

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
		d.Hub.Publish(allKeys...)
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
		d.Hub.Publish(allKeys...)
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
		if err := d.Sim.SetScenario(ctx, 0); err != nil {
			return err
		}
		if err := d.Sim.SetSpeed(ctx, 0); err != nil {
			return errEmulator(c, err)
		}
		d.Hub.Publish(allKeys...)
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
		// pause at the scenario start before loading, so the emulator never pushes
		// the new timeline against the previous run's clock
		if err := d.Sim.SetSpeed(ctx, 0); err != nil {
			return errEmulator(c, err)
		}
		if err := d.Sim.Jump(ctx, contract.ScenarioStart); err != nil {
			return errEmulator(c, err)
		}
		if err := d.Emu.LoadScenario(ctx, id); err != nil {
			return errEmulator(c, err)
		}
		if err := d.Sim.SetScenario(ctx, id); err != nil {
			return err
		}
		if err := d.Sim.SetSpeed(ctx, 60); err != nil {
			return errEmulator(c, err)
		}
		d.Hub.Publish(allKeys...)
		return c.JSON(http.StatusOK, clockJSON(d))
	}
}

// simSplit: what the client sees vs what the destination branch operator sees, now.
func simSplit(d Deps) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ttn := c.Param("ttn")
		if !contract.TTNPattern.MatchString(ttn) {
			return badTTN(c)
		}
		ctx := c.Request().Context()
		client, _, err := track(ctx, d, ttn, true, "")
		if err != nil {
			return err
		}
		op := map[string]any{"visibleInIncoming": false, "level": problems.Green, "problems": []problems.Problem{}}
		p, _, err := d.Store.Parcel(ctx, ttn)
		if err != nil {
			return err
		}
		if p != nil {
			op["branch"] = p.DestinationID
			incoming, err := d.Store.Incoming(ctx, p.DestinationID, d.Sim.Mode() == sim.ModeWithSolution)
			if err != nil {
				return err
			}
			for _, r := range incoming {
				if r.TTN == ttn {
					op["visibleInIncoming"] = true
				}
			}
		}
		rows, err := problemRows(ctx, d, []string{ttn}, true)
		if err != nil {
			return err
		}
		if len(rows) == 1 {
			op["level"], op["problems"] = rows[0].Level, rows[0].Problems
		}
		return c.JSON(http.StatusOK, map[string]any{"client": client, "operator": op, "mode": d.Sim.Mode()})
	}
}

func simPhysical(d Deps) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ttns, err := d.Emu.Physical(c.Request().Context(), c.QueryParam("location"))
		if err != nil {
			return errEmulator(c, err)
		}
		return c.JSON(http.StatusOK, nonNil(ttns))
	}
}

// simAccept plays the operator for the guided tour: the parcel is accepted at its destination.
func simAccept(d Deps) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ttn := c.Param("ttn")
		if !contract.TTNPattern.MatchString(ttn) {
			return badTTN(c)
		}
		ctx := c.Request().Context()
		p, _, err := d.Store.Parcel(ctx, ttn)
		if err != nil {
			return err
		}
		if p == nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Посилку не знайдено"})
		}
		if err := record(ctx, d, contract.KindArrivedBranch, ttn, p.DestinationID, nil); err != nil {
			return err
		}
		return c.JSON(http.StatusOK, map[string]string{"result": "ok"})
	}
}
