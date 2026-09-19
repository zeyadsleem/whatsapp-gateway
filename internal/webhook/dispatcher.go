package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/zeyad/whatsapp-gateway/internal/crypto"
	"github.com/zeyad/whatsapp-gateway/internal/database"
	"github.com/zeyad/whatsapp-gateway/internal/metrics"
	"github.com/zeyad/whatsapp-gateway/internal/safehttp"
	"github.com/zeyad/whatsapp-gateway/internal/whatsapp"
)

const (
	EventConnected    = "instance.connected"
	EventDisconnected = "instance.disconnected"
	EventLoggedOut    = "instance.logged_out"
	EventMessage      = "message.received"

	HeaderEvent     = "X-Webhook-Event"
	HeaderEventID   = "X-Webhook-Event-ID"
	HeaderTimestamp = "X-Webhook-Timestamp"
	HeaderSignature = "X-Webhook-Signature"
	HeaderInstance  = "X-Instance-ID"

	maxResponseBody = 4 * 1024
)

type Config struct {
	Timeout    time.Duration
	MaxRetries int
	Workers    int
	QueueSize  int
	Log        *slog.Logger
}

type InstanceProvider interface {
	GetByID(ctx context.Context, id string) (*database.Instance, error)
}

type Dispatcher struct {
	cfg      Config
	provider InstanceProvider
	box      *crypto.Box
	log      *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	queue  chan delivery
	client *http.Client
}

type delivery struct {
	instanceID string
	event      string
	eventID    string
	body       []byte
}

func NewDispatcher(cfg Config, provider InstanceProvider, box *crypto.Box) *Dispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	d := &Dispatcher{
		cfg:      cfg,
		provider: provider,
		box:      box,
		log:      cfg.Log,
		ctx:      ctx,
		cancel:   cancel,
		queue:    make(chan delivery, cfg.QueueSize),
		client:   safehttp.NewClient(cfg.Timeout),
	}
	for i := 0; i < cfg.Workers; i++ {
		d.wg.Add(1)
		go d.worker()
	}
	return d
}

func (d *Dispatcher) InstanceEvent(evt whatsapp.InstanceEvent) {
	var payload any
	ts := evt.Timestamp.UTC().Format(time.RFC3339)
	switch evt.Type {
	case whatsapp.EventConnected:
		payload = struct {
			Event      string `json:"event"`
			EventID    string `json:"event_id"`
			InstanceID string `json:"instance_id"`
			ShopID     string `json:"shop_id"`
			Phone      string `json:"phone_number"`
			Timestamp  string `json:"timestamp"`
		}{EventConnected, evt.EventID, evt.InstanceID, evt.ShopID, evt.Phone, ts}
	case whatsapp.EventDisconnected:
		payload = struct {
			Event      string `json:"event"`
			EventID    string `json:"event_id"`
			InstanceID string `json:"instance_id"`
			ShopID     string `json:"shop_id"`
			Reason     string `json:"reason,omitempty"`
			Timestamp  string `json:"timestamp"`
		}{EventDisconnected, evt.EventID, evt.InstanceID, evt.ShopID, evt.Reason, ts}
	case whatsapp.EventLoggedOut:
		payload = struct {
			Event      string `json:"event"`
			EventID    string `json:"event_id"`
			InstanceID string `json:"instance_id"`
			ShopID     string `json:"shop_id"`
			Timestamp  string `json:"timestamp"`
		}{EventLoggedOut, evt.EventID, evt.InstanceID, evt.ShopID, ts}
	default:
		return
	}
	d.enqueue(string(evt.Type), evt.EventID, evt.InstanceID, payload)
}

func (d *Dispatcher) MessageEvent(evt whatsapp.MessageEvent) {
	payload := struct {
		Event      string `json:"event"`
		EventID    string `json:"event_id"`
		InstanceID string `json:"instance_id"`
		ShopID     string `json:"shop_id"`
		From       string `json:"from"`
		MessageID  string `json:"message_id"`
		Type       string `json:"type"`
		Text       string `json:"text"`
		Timestamp  string `json:"timestamp"`
	}{
		Event:      EventMessage,
		EventID:    evt.EventID,
		InstanceID: evt.InstanceID,
		ShopID:     evt.ShopID,
		From:       evt.From,
		MessageID:  evt.MessageID,
		Type:       evt.Type,
		Text:       evt.Text,
		Timestamp:  evt.Timestamp.UTC().Format(time.RFC3339),
	}
	d.enqueue(EventMessage, evt.EventID, evt.InstanceID, payload)
}

func (d *Dispatcher) enqueue(event, eventID, instanceID string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		d.log.Error("failed to encode webhook payload", "event", "webhook_encode", "result", "failure", "error", err)
		return
	}
	select {
	case d.queue <- delivery{instanceID: instanceID, event: event, eventID: eventID, body: body}:
	default:
		metrics.WebhookDropped.Inc()
		d.log.Warn("webhook queue full, delivery dropped", "event", "webhook_dispatch", "webhook_event", event, "result", "dropped")
	}
}

func (d *Dispatcher) worker() {
	defer d.wg.Done()
	for {
		select {
		case <-d.ctx.Done():
			return
		case item := <-d.queue:
			d.process(item)
		}
	}
}

func (d *Dispatcher) process(item delivery) {
	target, secret, enabled, err := d.resolveConfig(item.instanceID)
	if err != nil {
		d.log.Warn("webhook target not found, delivery dropped", "event", "webhook_dispatch", "webhook_event", item.event, "result", "not_found")
		return
	}
	if !enabled || target == "" {
		metrics.WebhookTotal.WithLabelValues("skipped").Inc()
		return
	}

	backoff := time.Second
	for attempt := 0; attempt <= d.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-d.ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		status, err := d.deliver(item, target, secret)
		if err == nil {
			metrics.WebhookTotal.WithLabelValues("success").Inc()
			d.log.Debug("webhook delivered", "event", "webhook_dispatch", "webhook_event", item.event, "status", status, "attempt", attempt+1, "result", "success")
			return
		}
		d.log.Warn("webhook delivery failed", "event", "webhook_dispatch", "webhook_event", item.event, "attempt", attempt+1, "result", "failure", "error", err)
	}
	metrics.WebhookTotal.WithLabelValues("failure").Inc()
}

func (d *Dispatcher) resolveConfig(instanceID string) (string, []byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(d.ctx), 3*time.Second)
	defer cancel()
	inst, err := d.provider.GetByID(ctx, instanceID)
	if err != nil {
		return "", nil, false, err
	}
	if inst.WebhookURL == nil || *inst.WebhookURL == "" || len(inst.WebhookSecretEnc) == 0 {
		return "", nil, false, nil
	}
	secret, err := d.box.Decrypt(inst.WebhookSecretEnc)
	if err != nil {
		return "", nil, false, fmt.Errorf("decrypt webhook secret: %w", err)
	}
	return *inst.WebhookURL, secret, inst.WebhookEnabled, nil
}

func (d *Dispatcher) deliver(item delivery, target string, secret []byte) (int, error) {
	ts := time.Now().UTC().Format(time.RFC3339)
	req, err := http.NewRequestWithContext(d.ctx, http.MethodPost, target, bytes.NewReader(item.body))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "whatsapp-gateway/1.0")
	req.Header.Set(HeaderEvent, item.event)
	req.Header.Set(HeaderEventID, item.eventID)
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderInstance, item.instanceID)
	req.Header.Set(HeaderSignature, Sign(secret, ts, item.body))

	resp, err := d.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBody))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// Sign returns the value for X-Webhook-Signature: HMAC-SHA256 over the string
// "<timestamp>.<raw_body>", formatted as "sha256=<hex>".
func Sign(secret []byte, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func (d *Dispatcher) Shutdown(timeout time.Duration) {
	d.cancel()
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		d.log.Warn("webhook workers did not stop in time", "event", "shutdown", "result", "timeout")
	}
}
