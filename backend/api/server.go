// Package api is the backend HTTP layer (Echo v5).
package api

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"novapost/backend/auth"
	"novapost/backend/config"
	"novapost/backend/emuclient"
	"novapost/backend/sim"
	"novapost/backend/store"
	"novapost/contract"
)

type Deps struct {
	Cfg   config.Config
	Store *store.Store
	Hub   *Hub
	Sim   *sim.Sim
	Emu   *emuclient.Client
}

func NewServer(d Deps) *echo.Echo {
	e := echo.New()
	e.GET("/api/locations", func(c *echo.Context) error { return c.JSON(http.StatusOK, contract.Locations) })
	e.POST("/api/ingest/events", ingestHandler(d))

	login, logout, me := auth.Handlers(d.Cfg)
	e.POST("/api/login", login)
	e.POST("/api/logout", logout)
	e.GET("/api/me", me)

	sg := e.Group("/api/sim", auth.Middleware(d.Cfg.AppSecret, "admin"))
	sg.GET("/clock", simClock(d))
	sg.POST("/clock", simSetClock(d))
	sg.POST("/mode", simSetMode(d))
	sg.POST("/reset", simReset(d))
	sg.POST("/scenarios/:id", simScenario(d))
	sg.GET("/split/:ttn", simSplit(d))
	sg.GET("/physical", simPhysical(d))
	sg.POST("/accept/:ttn", simAccept(d))
	sg.POST("/parcels/random", simRandomParcels(d))

	registerOperator(e.Group("/api/operator", auth.Middleware(d.Cfg.AppSecret, "operator")), d)
	registerSenior(e.Group("/api/senior", auth.Middleware(d.Cfg.AppSecret, "senior")), d)
	e.GET("/api/track/:ttn", trackHandler(d))
	anyRole := auth.Middleware(d.Cfg.AppSecret, "admin", "senior", "operator")
	e.GET("/api/stream", streamHandler(d.Hub), anyRole)
	e.GET("/api/clock", simClock(d), anyRole)
	return e
}
