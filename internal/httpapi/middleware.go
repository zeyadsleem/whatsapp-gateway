package httpapi

import (
	"context"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/zeyad/whatsapp-gateway/internal/auth"
	"github.com/zeyad/whatsapp-gateway/internal/database"
	"github.com/zeyad/whatsapp-gateway/internal/metrics"
)

const (
	ctxRequestID  = "request_id"
	ctxInstance   = "instance"
	ctxKeyHash    = "api_key_hash"
	adminUserID   = "admin"
	touchInterval = time.Minute
)

func requestID(c *gin.Context) string {
	if v, ok := c.Get(ctxRequestID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func instanceFrom(c *gin.Context) *database.Instance {
	if v, ok := c.Get(ctxInstance); ok {
		if inst, ok := v.(*database.Instance); ok {
			return inst
		}
	}
	return nil
}

func (s *Server) middlewareRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" || len(id) > 128 {
			id = uuid.NewString()
		}
		c.Set(ctxRequestID, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func (s *Server) middlewareLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		reqLog := s.log.With("request_id", requestID(c))
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		attrs := []any{
			"event", "http_request",
			"method", c.Request.Method,
			"route", route,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"result", resultForStatus(c.Writer.Status()),
		}
		if inst := instanceFrom(c); inst != nil {
			attrs = append(attrs, "instance", inst.ShopID)
		}
		if c.Writer.Status() >= 500 {
			reqLog.Error("http request failed", attrs...)
		} else {
			reqLog.Info("http request", attrs...)
		}
	}
}

func resultForStatus(status int) string {
	if status >= 400 {
		return "failure"
	}
	return "success"
}

func (s *Server) middlewareRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("panic recovered",
					"request_id", requestID(c),
					"event", "panic",
					"result", "failure",
					"error", r,
					"stack", string(debug.Stack()))
				respondError(c, http.StatusInternalServerError, "internal_error", "internal server error")
			}
		}()
		c.Next()
	}
}

func (s *Server) middlewareSecureHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
		if s.cfg.IsProduction() {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		c.Next()
	}
}

func (s *Server) middlewareCORS() gin.HandlerFunc {
	allowed := make(map[string]bool, len(s.cfg.AllowedOrigins))
	allowAll := false
	for _, o := range s.cfg.AllowedOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[strings.TrimRight(o, "/")] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" || (!allowAll && !allowed[strings.TrimRight(origin, "/")]) {
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Vary", "Origin")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
		h.Set("Access-Control-Max-Age", "600")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func bodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBytes {
			respondError(c, http.StatusRequestEntityTooLarge, "body_too_large", "request body is too large")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

func (s *Server) middlewareAuthAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := auth.ExtractBearer(c.GetHeader("Authorization"))
		if !ok || !auth.VerifyKey(key, auth.HashKey(s.cfg.AdminAPIKey)) {
			respondError(c, http.StatusUnauthorized, "unauthorized", "invalid or missing admin API key")
			return
		}
		c.Next()
	}
}

func (s *Server) middlewareRateLimitAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.adminLimiter.Allow(adminUserID) {
			metrics.RateLimited.Inc()
			respondError(c, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
		c.Next()
	}
}

func (s *Server) middlewareAuthShop() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := auth.ExtractBearer(c.GetHeader("Authorization"))
		if !ok {
			respondError(c, http.StatusUnauthorized, "unauthorized", "missing shop API key")
			return
		}
		hash := auth.HashKey(raw)
		inst, err := s.keys.FindInstanceByHash(c.Request.Context(), hash)
		if err != nil {
			respondError(c, http.StatusUnauthorized, "unauthorized", "invalid shop API key")
			return
		}
		c.Set(ctxInstance, inst)
		c.Set(ctxKeyHash, hash)
		s.touchKey(hash)
		c.Next()
	}
}

func (s *Server) touchKey(hash string) {
	now := time.Now()
	s.touchMu.Lock()
	if last, ok := s.touchAt[hash]; ok && now.Sub(last) < touchInterval {
		s.touchMu.Unlock()
		return
	}
	s.touchAt[hash] = now
	if len(s.touchAt) > 4096 {
		for k, t := range s.touchAt {
			if now.Sub(t) > touchInterval {
				delete(s.touchAt, k)
			}
		}
	}
	s.touchMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.keys.TouchLastUsed(ctx, hash); err != nil {
			s.log.Warn("failed to update api key last_used_at", "event", "touch_key", "result", "failure", "error", err)
		}
	}()
}

func (s *Server) middlewareRateLimitShop() gin.HandlerFunc {
	return func(c *gin.Context) {
		hash, _ := c.Get(ctxKeyHash)
		key, _ := hash.(string)
		if !s.shopLimiter.Allow(key) {
			metrics.RateLimited.Inc()
			inst := instanceFrom(c)
			fields := []any{"event", "rate_limit", "result", "failure"}
			if inst != nil {
				fields = append(fields, "instance", inst.ShopID)
			}
			s.log.Warn("shop request rate limited", fields...)
			respondError(c, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
		c.Next()
	}
}
