package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zeyad/whatsapp-gateway/internal/metrics"
	"github.com/zeyad/whatsapp-gateway/internal/version"
	"github.com/zeyad/whatsapp-gateway/internal/whatsapp"
)

func metricsHandler() http.Handler { return metrics.Handler() }

type healthInstances struct {
	Total        int `json:"total"`
	Connected    int `json:"connected"`
	Disconnected int `json:"disconnected"`
	QRRequired   int `json:"qr_required"`
}

type healthResponse struct {
	Status        string          `json:"status"`
	UptimeSeconds int64           `json:"uptime_seconds"`
	Database      string          `json:"database"`
	Instances     healthInstances `json:"instances"`
	Version       string          `json:"version"`
}

func (s *Server) handleHealth(c *gin.Context) {
	dbStatus := "up"
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		dbStatus = "down"
	}

	var connected, qrRequired, disconnected int
	statuses := s.manager.Statuses()
	for _, st := range statuses {
		switch st.Status {
		case whatsapp.StatusConnected:
			connected++
		case whatsapp.StatusQRRequired:
			qrRequired++
		default:
			disconnected++
		}
	}

	status := "ok"
	code := http.StatusOK
	if dbStatus != "up" {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}
	respond(c, code, healthResponse{
		Status:        status,
		UptimeSeconds: int64(time.Since(s.startedAt).Seconds()),
		Database:      dbStatus,
		Instances: healthInstances{
			Total:        s.manager.Count(),
			Connected:    connected,
			Disconnected: disconnected,
			QRRequired:   qrRequired,
		},
		Version: version.Version,
	})
}
