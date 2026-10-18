// Package api is the backend HTTP layer (Echo v5).
package api

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"novapost/backend/config"
	"novapost/backend/store"
	"novapost/contract"
)

type Deps struct {
	Cfg   config.Config
	Store *store.Store
	Hub   *Hub
}

func NewServer(d Deps) *echo.Echo {
	e := echo.New()
	e.GET("/api/locations", func(c *echo.Context) error { return c.JSON(http.StatusOK, contract.Locations) })
	e.POST("/api/ingest/events", ingestHandler(d))
	return e
}
