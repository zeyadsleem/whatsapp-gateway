package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/zeyad/whatsapp-gateway/internal/database"
	"github.com/zeyad/whatsapp-gateway/internal/instance"
)

type instanceDTO struct {
	ID              string     `json:"id"`
	ShopID          string     `json:"shop_id"`
	DisplayName     string     `json:"display_name"`
	PhoneNumber     *string    `json:"phone_number"`
	Status          string     `json:"status"`
	LastError       string     `json:"last_error,omitempty"`
	WebhookURL      *string    `json:"webhook_url"`
	WebhookEnabled  bool       `json:"webhook_enabled"`
	APIKeyPrefix    string     `json:"api_key_prefix,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastConnectedAt *time.Time `json:"last_connected_at"`
}

func (s *Server) toDTO(ctx context.Context, inst *database.Instance) instanceDTO {
	dto := instanceDTO{
		ID:              inst.ID,
		ShopID:          inst.ShopID,
		DisplayName:     inst.DisplayName,
		PhoneNumber:     inst.PhoneNumber,
		Status:          inst.Status,
		WebhookURL:      inst.WebhookURL,
		WebhookEnabled:  inst.WebhookEnabled,
		CreatedAt:       inst.CreatedAt,
		UpdatedAt:       inst.UpdatedAt,
		LastConnectedAt: inst.LastConnectedAt,
	}
	if client, ok := s.manager.Get(inst.ID); ok {
		snap := client.Snapshot()
		dto.Status = snap.Status
		dto.LastError = snap.LastError
		if snap.Phone != "" {
			phone := snap.Phone
			dto.PhoneNumber = &phone
		}
	}
	if key, err := s.instances.APIKeyInfo(ctx, inst.ID); err == nil {
		dto.APIKeyPrefix = key.KeyPrefix
	}
	return dto
}

func (s *Server) loadInstance(c *gin.Context) (*database.Instance, bool) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_id", "instance id must be a UUID")
		return nil, false
	}
	inst, err := s.instances.Get(c.Request.Context(), id)
	if err != nil {
		s.fail(c, err, "failed to load instance")
		return nil, false
	}
	return inst, true
}

func (s *Server) handleCreateInstance(c *gin.Context) {
	var in instance.CreateInput
	if err := bindStrict(c, &in); err != nil {
		if isBodyTooLarge(err) {
			respondError(c, http.StatusRequestEntityTooLarge, "body_too_large", "request body is too large")
			return
		}
		respondError(c, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	result, err := s.instances.Create(c.Request.Context(), in)
	if err != nil {
		s.fail(c, err, "failed to create instance")
		return
	}
	s.log.Info("instance created via API", "instance", result.Instance.ShopID, "event", "create_instance", "result", "success")
	respond(c, http.StatusCreated, gin.H{
		"id":              result.Instance.ID,
		"shop_id":         result.Instance.ShopID,
		"display_name":    result.Instance.DisplayName,
		"status":          result.Instance.Status,
		"webhook_url":     result.Instance.WebhookURL,
		"webhook_enabled": result.Instance.WebhookEnabled,
		"created_at":      result.Instance.CreatedAt,
		"api_key":         result.APIKey,
		"webhook_secret":  result.WebhookSecret,
	})
}

func (s *Server) handleListInstances(c *gin.Context) {
	instances, err := s.instances.List(c.Request.Context())
	if err != nil {
		s.fail(c, err, "failed to list instances")
		return
	}
	out := make([]instanceDTO, 0, len(instances))
	for _, inst := range instances {
		out = append(out, s.toDTO(c.Request.Context(), inst))
	}
	respond(c, http.StatusOK, gin.H{"instances": out})
}

func (s *Server) handleGetInstance(c *gin.Context) {
	inst, ok := s.loadInstance(c)
	if !ok {
		return
	}
	respond(c, http.StatusOK, s.toDTO(c.Request.Context(), inst))
}

func (s *Server) handleUpdateInstance(c *gin.Context) {
	if _, ok := s.loadInstance(c); !ok {
		return
	}
	var in instance.UpdateInput
	if err := bindStrict(c, &in); err != nil {
		if isBodyTooLarge(err) {
			respondError(c, http.StatusRequestEntityTooLarge, "body_too_large", "request body is too large")
			return
		}
		respondError(c, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	result, err := s.instances.Update(c.Request.Context(), c.Param("id"), in)
	if err != nil {
		s.fail(c, err, "failed to update instance")
		return
	}
	resp := gin.H{"instance": s.toDTO(c.Request.Context(), result.Instance)}
	if result.WebhookSecret != "" {
		resp["webhook_secret"] = result.WebhookSecret
	}
	respond(c, http.StatusOK, resp)
}

func (s *Server) handleUpdateWebhook(c *gin.Context) {
	if _, ok := s.loadInstance(c); !ok {
		return
	}
	var in instance.UpdateInput
	if err := bindStrict(c, &in); err != nil {
		if isBodyTooLarge(err) {
			respondError(c, http.StatusRequestEntityTooLarge, "body_too_large", "request body is too large")
			return
		}
		respondError(c, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if in.DisplayName != nil {
		respondError(c, http.StatusBadRequest, "validation_error", "display_name cannot be changed through the webhook endpoint")
		return
	}
	in.RotateSecret = in.RotateSecret || c.Query("rotate_secret") == "true"
	result, err := s.instances.Update(c.Request.Context(), c.Param("id"), in)
	if err != nil {
		s.fail(c, err, "failed to update webhook")
		return
	}
	resp := gin.H{"instance": s.toDTO(c.Request.Context(), result.Instance)}
	if result.WebhookSecret != "" {
		resp["webhook_secret"] = result.WebhookSecret
	}
	respond(c, http.StatusOK, resp)
}

func (s *Server) handleDeleteInstance(c *gin.Context) {
	if _, ok := s.loadInstance(c); !ok {
		return
	}
	if err := s.instances.Delete(c.Request.Context(), c.Param("id")); err != nil {
		s.fail(c, err, "failed to delete instance")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleConnectInstance(c *gin.Context) {
	inst, ok := s.loadInstance(c)
	if !ok {
		return
	}
	if err := s.instances.Connect(c.Request.Context(), inst.ID); err != nil {
		s.fail(c, err, "failed to connect instance")
		return
	}
	respond(c, http.StatusAccepted, gin.H{"status": "connecting"})
}

func (s *Server) handleRestartInstance(c *gin.Context) {
	inst, ok := s.loadInstance(c)
	if !ok {
		return
	}
	if err := s.instances.Restart(c.Request.Context(), inst.ID); err != nil {
		s.fail(c, err, "failed to restart instance")
		return
	}
	respond(c, http.StatusAccepted, gin.H{"status": "connecting"})
}

func (s *Server) handleDisconnectInstance(c *gin.Context) {
	inst, ok := s.loadInstance(c)
	if !ok {
		return
	}
	if err := s.instances.Disconnect(c.Request.Context(), inst.ID); err != nil {
		s.fail(c, err, "failed to disconnect instance")
		return
	}
	respond(c, http.StatusAccepted, gin.H{"status": "disconnected"})
}

func (s *Server) handleLogoutInstance(c *gin.Context) {
	inst, ok := s.loadInstance(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if err := s.instances.Logout(ctx, inst.ID); err != nil {
		s.fail(c, err, "failed to log out instance")
		return
	}
	respond(c, http.StatusOK, gin.H{"status": "logged_out"})
}

func (s *Server) handleRotateAPIKey(c *gin.Context) {
	inst, ok := s.loadInstance(c)
	if !ok {
		return
	}
	key, err := s.instances.RotateAPIKey(c.Request.Context(), inst.ID)
	if err != nil {
		s.fail(c, err, "failed to rotate api key")
		return
	}
	s.log.Info("api key rotated", "instance", inst.ShopID, "event", "rotate_api_key", "result", "success")
	respond(c, http.StatusOK, gin.H{"api_key": key})
}

func (s *Server) handleAdminStats(c *gin.Context) {
	stats, err := s.instances.Stats(c.Request.Context())
	if err != nil {
		s.fail(c, err, "failed to load stats")
		return
	}
	respond(c, http.StatusOK, stats)
}
