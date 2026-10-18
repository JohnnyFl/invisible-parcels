package api

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

func registerSenior(g *echo.Group, d Deps) {
	g.GET("/problems", func(c *echo.Context) error {
		ctx := c.Request().Context()
		ttns, err := d.Store.AllActiveTTNs(ctx)
		if err != nil {
			return err
		}
		rows, err := problemRows(ctx, d, ttns)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, rows)
	})
}
