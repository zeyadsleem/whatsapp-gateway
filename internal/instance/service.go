package instance

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/zeyad/whatsapp-gateway/internal/auth"
	"github.com/zeyad/whatsapp-gateway/internal/crypto"
	"github.com/zeyad/whatsapp-gateway/internal/database"
	"github.com/zeyad/whatsapp-gateway/internal/safehttp"
	"github.com/zeyad/whatsapp-gateway/internal/whatsapp"
)

var (
	shopIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	ErrValidation = errors.New("validation error")
)

func invalidInput(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

type Service struct {
	repo    *database.InstanceRepo
	keys    *database.APIKeyRepo
	manager *whatsapp.Manager
	box     *crypto.Box
	log     *slog.Logger
}

func NewService(repo *database.InstanceRepo, keys *database.APIKeyRepo, manager *whatsapp.Manager, box *crypto.Box, log *slog.Logger) *Service {
	return &Service{repo: repo, keys: keys, manager: manager, box: box, log: log}
}

// Restore registers every persisted instance before the API starts accepting
// requests. Only paired instances outside the error/logged-out states should
// reconnect automatically; the others remain available for manual pairing.
func (s *Service) Restore(ctx context.Context) ([]whatsapp.InstanceInfo, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load persisted instances: %w", err)
	}
	reconnect := make([]whatsapp.InstanceInfo, 0, len(rows))
	for _, inst := range rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info := whatsapp.InstanceInfo{
			ID: inst.ID, ShopID: inst.ShopID, DisplayName: inst.DisplayName,
		}
		if inst.WAJID != nil {
			info.WAJID = *inst.WAJID
		}
		if _, err := s.manager.Register(info); err != nil {
			return nil, fmt.Errorf("restore instance %s: %w", inst.ID, err)
		}
		if info.WAJID != "" && inst.Status != database.StatusLoggedOut && inst.Status != database.StatusError {
			reconnect = append(reconnect, info)
		}
	}
	return reconnect, nil
}

type CreateInput struct {
	ShopID         string `json:"shop_id"`
	DisplayName    string `json:"display_name"`
	WebhookURL     string `json:"webhook_url"`
	WebhookEnabled *bool  `json:"webhook_enabled"`
}

type CreateResult struct {
	Instance      *database.Instance `json:"instance"`
	APIKey        string             `json:"api_key"`
	WebhookSecret string             `json:"webhook_secret,omitempty"`
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*CreateResult, error) {
	in.ShopID = strings.TrimSpace(in.ShopID)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.WebhookURL = strings.TrimSpace(in.WebhookURL)

	if !shopIDPattern.MatchString(in.ShopID) {
		return nil, invalidInput("shop_id must match %s", shopIDPattern.String())
	}
	if in.DisplayName == "" || len([]rune(in.DisplayName)) > 128 {
		return nil, invalidInput("display_name must be between 1 and 128 characters")
	}
	if in.WebhookURL != "" {
		if err := safehttp.ValidateURLString(in.WebhookURL); err != nil {
			return nil, invalidInput("webhook_url: %v", err)
		}
	}

	total, err := s.repo.Count(ctx)
	if err != nil {
		return nil, err
	}
	if total >= s.manager.MaxInstances() {
		return nil, whatsapp.ErrMaxInstances
	}

	rawKey, keyHash, keyPrefix, err := auth.GenerateShopKey()
	if err != nil {
		return nil, err
	}
	webhookSecret, err := generateWebhookSecret()
	if err != nil {
		return nil, err
	}
	var secretEnc []byte
	if in.WebhookURL != "" {
		secretEnc, err = s.box.Encrypt([]byte(webhookSecret))
		if err != nil {
			return nil, fmt.Errorf("encrypt webhook secret: %w", err)
		}
	}

	enabled := true
	if in.WebhookEnabled != nil {
		enabled = *in.WebhookEnabled
	}
	inst := &database.Instance{
		ID:               uuid.NewString(),
		ShopID:           in.ShopID,
		DisplayName:      in.DisplayName,
		Status:           database.StatusDisconnected,
		WebhookSecretEnc: secretEnc,
		WebhookEnabled:   enabled,
	}
	if in.WebhookURL != "" {
		inst.WebhookURL = &in.WebhookURL
	}
	if err := s.repo.Create(ctx, inst); err != nil {
		return nil, err
	}
	if err := s.keys.Upsert(ctx, &database.APIKey{
		ID:         uuid.NewString(),
		InstanceID: inst.ID,
		KeyHash:    keyHash,
		KeyPrefix:  keyPrefix,
	}); err != nil {
		if cleanupErr := s.repo.Delete(ctx, inst.ID); cleanupErr != nil {
			s.log.Error("failed to roll back instance after key error", "instance", inst.ShopID, "event", "create_rollback", "result", "failure", "error", cleanupErr)
		}
		return nil, err
	}
	if _, err := s.manager.Register(whatsapp.InstanceInfo{
		ID:          inst.ID,
		ShopID:      inst.ShopID,
		DisplayName: inst.DisplayName,
	}); err != nil {
		if cleanupErr := s.repo.Delete(ctx, inst.ID); cleanupErr != nil {
			s.log.Error("failed to roll back instance after register error", "instance", inst.ShopID, "event", "create_rollback", "result", "failure", "error", cleanupErr)
		}
		return nil, err
	}

	created, err := s.repo.GetByID(ctx, inst.ID)
	if err != nil {
		return nil, err
	}
	out := &CreateResult{Instance: created, APIKey: rawKey}
	if in.WebhookURL != "" {
		out.WebhookSecret = webhookSecret
	}
	s.log.Info("instance created", "instance", inst.ShopID, "event", "create_instance", "result", "success")
	return out, nil
}

func (s *Service) List(ctx context.Context) ([]*database.Instance, error) {
	return s.repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (*database.Instance, error) {
	return s.repo.GetByID(ctx, id)
}

type UpdateInput struct {
	DisplayName    *string `json:"display_name"`
	WebhookURL     *string `json:"webhook_url"`
	WebhookEnabled *bool   `json:"webhook_enabled"`
	RotateSecret   bool    `json:"rotate_webhook_secret"`
}

type UpdateResult struct {
	Instance      *database.Instance `json:"instance"`
	WebhookSecret string             `json:"webhook_secret,omitempty"`
}

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*UpdateResult, error) {
	inst, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	displayName := inst.DisplayName
	if in.DisplayName != nil {
		displayName = strings.TrimSpace(*in.DisplayName)
		if displayName == "" || len([]rune(displayName)) > 128 {
			return nil, invalidInput("display_name must be between 1 and 128 characters")
		}
	}
	webhookURL := ""
	if inst.WebhookURL != nil {
		webhookURL = *inst.WebhookURL
	}
	if in.WebhookURL != nil {
		webhookURL = strings.TrimSpace(*in.WebhookURL)
		if webhookURL != "" {
			if err := safehttp.ValidateURLString(webhookURL); err != nil {
				return nil, invalidInput("webhook_url: %v", err)
			}
		}
	}
	enabled := inst.WebhookEnabled
	if in.WebhookEnabled != nil {
		enabled = *in.WebhookEnabled
	}

	secretEnc := inst.WebhookSecretEnc
	var newSecret string
	if in.RotateSecret || (webhookURL != "" && len(secretEnc) == 0) {
		newSecret, err = generateWebhookSecret()
		if err != nil {
			return nil, err
		}
		secretEnc, err = s.box.Encrypt([]byte(newSecret))
		if err != nil {
			return nil, fmt.Errorf("encrypt webhook secret: %w", err)
		}
	}
	if err := s.repo.UpdateMeta(ctx, id, displayName, webhookURL, secretEnc, enabled); err != nil {
		return nil, err
	}
	updated, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &UpdateResult{Instance: updated, WebhookSecret: newSecret}, nil
}

func (s *Service) RotateAPIKey(ctx context.Context, id string) (string, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return "", err
	}
	rawKey, keyHash, keyPrefix, err := auth.GenerateShopKey()
	if err != nil {
		return "", err
	}
	if err := s.keys.Upsert(ctx, &database.APIKey{
		ID:         uuid.NewString(),
		InstanceID: id,
		KeyHash:    keyHash,
		KeyPrefix:  keyPrefix,
	}); err != nil {
		return "", err
	}
	return rawKey, nil
}

func (s *Service) APIKeyInfo(ctx context.Context, id string) (*database.APIKey, error) {
	return s.keys.GetByInstanceID(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	inst, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if client, ok := s.manager.Get(id); ok {
		if err := client.Delete(ctx); err != nil {
			s.log.Warn("failed to clean up whatsapp session", "instance", inst.ShopID, "event", "delete_session", "result", "failure", "error", err)
		}
	}
	s.manager.Remove(id)
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.log.Info("instance deleted", "instance", inst.ShopID, "event", "delete_instance", "result", "success")
	return nil
}

func (s *Service) Connect(ctx context.Context, id string) error {
	client, ok := s.manager.Get(id)
	if !ok {
		return whatsapp.ErrNotFound
	}
	return client.Connect(ctx)
}

func (s *Service) Disconnect(ctx context.Context, id string) error {
	client, ok := s.manager.Get(id)
	if !ok {
		return whatsapp.ErrNotFound
	}
	return client.Disconnect(ctx)
}

func (s *Service) Restart(ctx context.Context, id string) error {
	client, ok := s.manager.Get(id)
	if !ok {
		return whatsapp.ErrNotFound
	}
	return client.Restart(ctx)
}

func (s *Service) Logout(ctx context.Context, id string) error {
	client, ok := s.manager.Get(id)
	if !ok {
		return whatsapp.ErrNotFound
	}
	return client.Logout(ctx)
}

func (s *Service) QR(ctx context.Context, id string) (string, time.Time, string, error) {
	client, ok := s.manager.Get(id)
	if !ok {
		return "", time.Time{}, "", whatsapp.ErrNotFound
	}
	snap := client.Snapshot()
	if snap.Status == whatsapp.StatusConnected {
		return "", time.Time{}, snap.Status, nil
	}
	code, expires, err := client.QR()
	if err != nil {
		return "", time.Time{}, snap.Status, err
	}
	return code, expires, snap.Status, nil
}

func (s *Service) Stats(ctx context.Context) (Stats, error) {
	counts, err := s.repo.CountByStatus(ctx)
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{ByStatus: counts}
	for _, status := range []string{
		database.StatusConnected, database.StatusDisconnected, database.StatusQRRequired,
		database.StatusConnecting, database.StatusReconnecting, database.StatusLoggedOut, database.StatusError,
	} {
		stats.Total += counts[status]
	}
	for _, client := range s.manager.List() {
		st := client.Stats()
		stats.SentOK += st.SentOK
		stats.SentFailed += st.SentFailed
		stats.Received += st.Received
	}
	return stats, nil
}

type Stats struct {
	Total      int            `json:"total"`
	ByStatus   map[string]int `json:"by_status"`
	SentOK     int64          `json:"sent_ok"`
	SentFailed int64          `json:"sent_failed"`
	Received   int64          `json:"received"`
}

func generateWebhookSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return "whsec_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
