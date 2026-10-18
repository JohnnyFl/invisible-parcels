package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"novapost/contract"
)

const maxIngestBody = 10 << 20

// ingestHandler accepts ScanEvent.push from NP (the emulator), signed with HMAC.
func ingestHandler(d Deps) echo.HandlerFunc {
	return func(c *echo.Context) error {
		body, err := io.ReadAll(io.LimitReader(c.Request().Body, maxIngestBody))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "cannot read body")
		}
		if !contract.Verify(body, d.Cfg.IngestSecret, c.Request().Header.Get("X-Signature")) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "bad signature"})
		}
		var env contract.Envelope[contract.EventBatch]
		if err := json.Unmarshal(body, &env); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid json")
		}
		n, err := d.Store.InsertEvents(c.Request().Context(), env.MethodProperties.Events, time.Now())
		if err != nil {
			return err
		}
		if n > 0 {
			d.Hub.Publish("problems", "incoming", "track")
		}
		return c.JSON(http.StatusOK, map[string]any{"success": true, "inserted": n})
	}
}
