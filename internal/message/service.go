package message

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/zeyad/whatsapp-gateway/internal/metrics"
	"github.com/zeyad/whatsapp-gateway/internal/safehttp"
	"github.com/zeyad/whatsapp-gateway/internal/whatsapp"
)

var ErrInstanceNotFound = whatsapp.ErrNotFound

type Service struct {
	manager   *whatsapp.Manager
	maxUpload int64
	fetch     *http.Client
	log       *slog.Logger
}

func NewService(manager *whatsapp.Manager, maxUpload int64, log *slog.Logger) *Service {
	return &Service{
		manager:   manager,
		maxUpload: maxUpload,
		fetch:     safehttp.NewClient(15 * time.Second),
		log:       log,
	}
}

func (s *Service) MaxUploadBytes() int64 { return s.maxUpload }

func (s *Service) SendText(ctx context.Context, instanceID, to, text string) (*whatsapp.SendResult, error) {
	phone, err := NormalizePhone(to)
	if err != nil {
		return nil, err
	}
	if err := ValidateText(text); err != nil {
		return nil, err
	}
	client, err := s.client(instanceID)
	if err != nil {
		return nil, err
	}
	metrics.SendTotal.Inc()
	start := time.Now()
	res, err := client.SendText(ctx, phone, text)
	s.logResult(instanceID, "text", start, err)
	return res, err
}

func (s *Service) SendImageBytes(ctx context.Context, instanceID, to, caption string, data []byte) (*whatsapp.SendResult, error) {
	phone, err := NormalizePhone(to)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > s.maxUpload {
		return nil, ErrMediaTooLarge
	}
	mime, err := ValidateImageData(data)
	if err != nil {
		return nil, err
	}
	client, err := s.client(instanceID)
	if err != nil {
		return nil, err
	}
	metrics.SendTotal.Inc()
	start := time.Now()
	res, err := client.SendImage(ctx, phone, data, mime, caption)
	s.logResult(instanceID, "image", start, err)
	return res, err
}

func (s *Service) SendImageURL(ctx context.Context, instanceID, to, caption, rawURL string) (*whatsapp.SendResult, error) {
	phone, err := NormalizePhone(to)
	if err != nil {
		return nil, err
	}
	data, _, err := s.fetchMedia(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return s.SendImageBytes(ctx, instanceID, phone, caption, data)
}

func (s *Service) SendDocumentBytes(ctx context.Context, instanceID, to, caption, filename, mime string, data []byte) (*whatsapp.SendResult, error) {
	phone, err := NormalizePhone(to)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > s.maxUpload {
		return nil, ErrMediaTooLarge
	}
	normalized, err := NormalizeDocumentMime(mime)
	if err != nil {
		return nil, err
	}
	client, err := s.client(instanceID)
	if err != nil {
		return nil, err
	}
	metrics.SendTotal.Inc()
	start := time.Now()
	res, err := client.SendDocument(ctx, phone, data, filename, normalized, caption)
	s.logResult(instanceID, "document", start, err)
	return res, err
}

func (s *Service) SendDocumentURL(ctx context.Context, instanceID, to, caption, filename, mime, rawURL string) (*whatsapp.SendResult, error) {
	phone, err := NormalizePhone(to)
	if err != nil {
		return nil, err
	}
	data, detected, err := s.fetchMedia(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	if mime == "" {
		mime = detected
	}
	return s.SendDocumentBytes(ctx, instanceID, phone, caption, filename, mime, data)
}

func (s *Service) client(instanceID string) (*whatsapp.Client, error) {
	client, ok := s.manager.Get(instanceID)
	if !ok {
		return nil, ErrInstanceNotFound
	}
	return client, nil
}

func (s *Service) fetchMedia(ctx context.Context, rawURL string) ([]byte, string, error) {
	if err := safehttp.ValidateURLString(rawURL); err != nil {
		return nil, "", fmt.Errorf("media url: %w", err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build media request: %w", err)
	}
	resp, err := s.fetch.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch media: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", fmt.Errorf("media fetch returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, s.maxUpload+1))
	if err != nil {
		return nil, "", fmt.Errorf("read media: %w", err)
	}
	if int64(len(data)) > s.maxUpload {
		return nil, "", ErrMediaTooLarge
	}
	mime := strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0]
	return data, strings.TrimSpace(strings.ToLower(mime)), nil
}

func (s *Service) logResult(instanceID, kind string, start time.Time, err error) {
	duration := time.Since(start).Milliseconds()
	if err != nil {
		metrics.SendFailures.Inc()
		s.log.Warn("send message failed",
			"instance", instanceID, "event", "send_message", "type", kind,
			"duration_ms", duration, "result", "failure", "error", sanitizeError(err))
		return
	}
	s.log.Info("send message completed",
		"instance", instanceID, "event", "send_message", "type", kind,
		"duration_ms", duration, "result", "success")
}

func sanitizeError(err error) string {
	switch {
	case errors.Is(err, whatsapp.ErrNotConnected):
		return "instance not connected"
	case errors.Is(err, whatsapp.ErrNotPaired):
		return "instance not paired"
	case errors.Is(err, whatsapp.ErrInstanceClosed):
		return "instance closed"
	default:
		return err.Error()
	}
}
