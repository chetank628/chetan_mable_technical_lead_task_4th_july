// Package handlers wires the HTTP surface: ingest, auth, health, metrics, and
// analytics. It depends on the store, the ingest worker, and the authenticator,
// keeping transport concerns (CORS, cookies, status codes) out of those layers.
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mable/mono/api/internal/auth"
	"github.com/mable/mono/api/internal/config"
	"github.com/mable/mono/api/internal/ingest"
	"github.com/mable/mono/api/internal/store"
)

// Handler bundles the dependencies every route needs.
type Handler struct {
	cfg    config.Config
	store  *store.Store
	worker *ingest.Worker
	auth   *auth.Authenticator
}

// New builds a Handler.
func New(cfg config.Config, st *store.Store, w *ingest.Worker, a *auth.Authenticator) *Handler {
	return &Handler{cfg: cfg, store: st, worker: w, auth: a}
}

// Router constructs the gin engine with all routes and middleware mounted.
func (h *Handler) Router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(h.cors())

	r.GET("/health", h.Health)
	r.GET("/metrics", h.Metrics)

	api := r.Group("/api")
	{
		api.POST("/events", h.Ingest)

		api.POST("/auth/signup", h.Signup)
		api.POST("/auth/login", h.Login)
		api.POST("/auth/logout", h.Logout)

		authed := api.Group("")
		authed.Use(h.auth.RequireAuth())
		{
			authed.GET("/auth/me", h.Me)
			authed.GET("/stats", h.Stats)
		}
	}
	return r
}

// cors returns a credentialed CORS middleware locked to the configured SPA
// origin. A wildcard origin is never sent alongside credentials (the browser
// would reject it), which is also validated at config load in prod.
func (h *Handler) cors() gin.HandlerFunc {
	origin := h.cfg.CORSOrigin
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Header("Vary", "Origin")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
