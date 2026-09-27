package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/flowchartsman/swaggerui"
	"github.com/gin-gonic/gin"

	webadmin "github.com/zeyad/whatsapp-gateway/web/admin"

	"github.com/zeyad/whatsapp-gateway/internal/config"
	"github.com/zeyad/whatsapp-gateway/internal/database"
	"github.com/zeyad/whatsapp-gateway/internal/instance"
	"github.com/zeyad/whatsapp-gateway/internal/message"
	"github.com/zeyad/whatsapp-gateway/internal/ratelimit"
	"github.com/zeyad/whatsapp-gateway/internal/version"
	"github.com/zeyad/whatsapp-gateway/internal/whatsapp"
)

const (
	jsonBodyLimit    = 64 * 1024
	adminBodyLimit   = 256 * 1024
	maxResponseBytes = 16 * 1024 * 1024
)

type Deps struct {
	Config    *config.Config
	Log       *slog.Logger
	DB        *database.DB
	Keys      *database.APIKeyRepo
	Instances *instance.Service
	Messages  *message.Service
	Manager   *whatsapp.Manager
}

type Server struct {
	cfg       *config.Config
	log       *slog.Logger
	db        *database.DB
	keys      *database.APIKeyRepo
	instances *instance.Service
	messages  *message.Service
	manager   *whatsapp.Manager
	engine    *gin.Engine
	http      *http.Server
	startedAt time.Time

	shopLimiter  *ratelimit.Limiter
	adminLimiter *ratelimit.Limiter

	touchMu sync.Mutex
	touchAt map[string]time.Time
}

func New(deps Deps) *Server {
	if deps.Config.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}
	s := &Server{
		cfg:          deps.Config,
		log:          deps.Log,
		db:           deps.DB,
		keys:         deps.Keys,
		instances:    deps.Instances,
		messages:     deps.Messages,
		manager:      deps.Manager,
		startedAt:    time.Now(),
		shopLimiter:  ratelimit.New(deps.Config.RateLimitPerSecond, deps.Config.RateLimitBurst),
		adminLimiter: ratelimit.New(deps.Config.AdminRateLimitRPS, deps.Config.AdminRateLimitBurst),
		touchAt:      make(map[string]time.Time),
	}

	engine := gin.New()
	engine.RedirectTrailingSlash = true
	engine.HandleMethodNotAllowed = true
	engine.MaxMultipartMemory = 1 << 20
	engine.Use(
		s.middlewareRequestID(),
		s.middlewareLogger(),
		s.middlewareRecovery(),
		s.middlewareSecureHeaders(),
		s.middlewareCORS(),
	)
	s.engine = engine
	s.routes()
	s.http = &http.Server{
		Addr:              fmt.Sprintf(":%d", deps.Config.AppPort),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return s
}

func (s *Server) routes() {
	r := s.engine

	r.GET("/health", s.handleHealth)
	r.GET("/metrics", gin.WrapH(metricsHandler()))

	spec := openAPISpec()
	r.GET("/api/v1/openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml; charset=utf-8", spec)
	})
	docs := swaggerui.Handler(spec)
	r.GET("/docs", func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, "/docs/") })
	r.GET("/docs/*any", gin.WrapH(http.StripPrefix("/docs", docs)))

	s.registerAdminRoutes(r)
	s.registerMessageRoutes(r)
	s.registerAdminUI(r)
}

func (s *Server) registerAdminRoutes(r *gin.Engine) {
	admin := r.Group("/api/v1/admin")
	admin.Use(
		bodyLimit(adminBodyLimit),
		s.middlewareRateLimitAdmin(),
		s.middlewareAuthAdmin(),
	)
	admin.GET("/stats", s.handleAdminStats)
	admin.POST("/instances", s.handleCreateInstance)
	admin.GET("/instances", s.handleListInstances)
	admin.GET("/instances/:id", s.handleGetInstance)
	admin.PATCH("/instances/:id", s.handleUpdateInstance)
	admin.PUT("/instances/:id/webhook", s.handleUpdateWebhook)
	admin.DELETE("/instances/:id", s.handleDeleteInstance)
	admin.POST("/instances/:id/connect", s.handleConnectInstance)
	admin.GET("/instances/:id/qr", s.handleInstanceQR)
	admin.POST("/instances/:id/restart", s.handleRestartInstance)
	admin.POST("/instances/:id/disconnect", s.handleDisconnectInstance)
	admin.POST("/instances/:id/logout", s.handleLogoutInstance)
	admin.POST("/instances/:id/api-key/rotate", s.handleRotateAPIKey)
}

func (s *Server) registerMessageRoutes(r *gin.Engine) {
	group := r.Group("/api/v1/messages")
	group.Use(
		bodyLimit(s.messages.MaxUploadBytes()+1024*1024),
		s.middlewareAuthShop(),
		s.middlewareRateLimitShop(),
	)
	group.POST("/text", s.handleSendText)
	group.POST("/image", s.handleSendImage)
	group.POST("/document", s.handleSendDocument)
}

func (s *Server) registerAdminUI(r *gin.Engine) {
	dist, err := fs.Sub(webadmin.FS, "dist")
	if err != nil {
		s.log.Error("failed to mount admin UI assets", "event", "admin_ui", "result", "failure", "error", err)
		return
	}
	fileServer := http.FileServer(http.FS(dist))
	r.GET("/admin", func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, "/admin/") })
	r.GET("/admin/*filepath", func(c *gin.Context) {
		filePath := strings.TrimPrefix(c.Param("filepath"), "/")
		if filePath == "" {
			s.serveIndex(c, dist)
			return
		}
		if _, err := fs.Stat(dist, path.Clean(filePath)); err != nil {
			s.serveIndex(c, dist)
			return
		}
		c.Request.URL.Path = "/" + filePath
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			respondError(c, http.StatusNotFound, "not_found", "route not found")
			return
		}
		c.Redirect(http.StatusFound, "/admin/")
	})
}

func (s *Server) serveIndex(c *gin.Context, dist fs.FS) {
	data, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		respondError(c, http.StatusNotFound, "not_found", "admin UI is not built")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

func (s *Server) Run() error {
	s.log.Info("http server listening", "event", "startup", "port", s.cfg.AppPort, "version", version.Version)
	if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) Engine() *gin.Engine { return s.engine }
