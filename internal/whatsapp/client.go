package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"

	"github.com/zeyad/whatsapp-gateway/internal/logger"
	"github.com/zeyad/whatsapp-gateway/internal/metrics"
)

const (
	pairingWindow    = 5 * time.Minute
	pairingRotations = 30
	connectTimeout   = 45 * time.Second
	reconnectBase    = 5 * time.Second
	reconnectMax     = 5 * time.Minute
	persistTimeout   = 5 * time.Second

	internalPrefix = "internal"
)

type Client struct {
	id          string
	shopID      string
	displayName string

	ctx         context.Context
	wa          *whatsmeow.Client
	container   *sqlstore.Container
	repo        Repository
	notifier    Notifier
	log         *slog.Logger
	sendTimeout time.Duration
	wg          *sync.WaitGroup

	mu               sync.RWMutex
	status           string
	phone            string
	waJID            string
	lastErr          string
	qrCode           string
	qrExpiresAt      time.Time
	manualDisconnect bool
	closed           bool
	loggedOut        bool
	reconnectRunning bool
	pairingRunning   bool
	pairingCancel    context.CancelFunc

	sentOK     atomic.Int64
	sentFailed atomic.Int64
	received   atomic.Int64
}

type ClientOptions struct {
	ID          string
	ShopID      string
	DisplayName string
	RestoreJID  string
	Container   *sqlstore.Container
	Repo        Repository
	Notifier    Notifier
	Log         *slog.Logger
	SendTimeout time.Duration
	Parent      context.Context
	WG          *sync.WaitGroup
}

func NewClient(opts ClientOptions) (*Client, error) {
	var device *store.Device
	if opts.RestoreJID != "" {
		jid, err := types.ParseJID(opts.RestoreJID)
		if err != nil {
			return nil, fmt.Errorf("parse stored jid: %w", err)
		}
		device, err = opts.Container.GetDevice(opts.Parent, jid)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrNoDevice, err)
		}
	} else {
		device = opts.Container.NewDevice()
	}

	c := &Client{
		id:          opts.ID,
		shopID:      opts.ShopID,
		displayName: opts.DisplayName,
		ctx:         opts.Parent,
		container:   opts.Container,
		repo:        opts.Repo,
		notifier:    opts.Notifier,
		log:         opts.Log.With("instance", opts.ShopID),
		sendTimeout: opts.SendTimeout,
		wg:          opts.WG,
		status:      StatusDisconnected,
	}
	c.wa = whatsmeow.NewClient(device, logger.WhatsAppLogger(c.log, "client"))
	c.wa.AddEventHandler(c.handleEvent)
	metrics.Instances.WithLabelValues(StatusDisconnected).Inc()
	return c, nil
}

func (c *Client) ID() string          { return c.id }
func (c *Client) ShopID() string      { return c.shopID }
func (c *Client) DisplayName() string { return c.displayName }

func (c *Client) IsPaired() bool {
	return c.wa.Store.ID != nil
}

func (c *Client) IsConnected() bool {
	return c.wa.IsConnected()
}

func (c *Client) Snapshot() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Status{
		ID:          c.id,
		ShopID:      c.shopID,
		DisplayName: c.displayName,
		Status:      c.status,
		Phone:       c.phone,
		WAJID:       c.waJID,
		LastError:   c.lastErr,
		QRCode:      c.qrCode,
		QRExpiresAt: c.qrExpiresAt,
		Connected:   c.wa.IsConnected(),
		SentOK:      c.sentOK.Load(),
		SentFailed:  c.sentFailed.Load(),
		Received:    c.received.Load(),
	}
}

func (c *Client) Stats() Stats {
	return Stats{
		SentOK:     c.sentOK.Load(),
		SentFailed: c.sentFailed.Load(),
		Received:   c.received.Load(),
	}
}

func (c *Client) Connect(ctx context.Context) error {
	c.mu.RLock()
	closed := c.closed
	paired := c.wa.Store.ID != nil
	pairing := c.pairingRunning
	c.mu.RUnlock()

	if closed {
		return ErrInstanceClosed
	}
	if c.wa.IsConnected() {
		return nil
	}
	if !paired {
		if pairing {
			return ErrPairingInProgress
		}
		return c.StartPairing(ctx)
	}
	return c.connectSocket()
}

func (c *Client) connectSocket() error {
	c.mu.Lock()
	c.manualDisconnect = false
	c.mu.Unlock()
	c.setStatus(StatusConnecting, nil)

	if err := c.wa.Connect(); err != nil {
		if errors.Is(err, whatsmeow.ErrAlreadyConnected) {
			return nil
		}
		c.setStatus(StatusError, err)
		return fmt.Errorf("connect instance: %w", err)
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if !c.wa.WaitForConnection(connectTimeout) {
			c.log.Warn("connection attempt did not complete in time", "event", "connect", "result", "timeout")
			c.superviseReconnect()
		}
	}()
	return nil
}

func (c *Client) StartPairing(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrInstanceClosed
	}
	if c.wa.Store.ID != nil {
		c.mu.Unlock()
		return ErrAlreadyPaired
	}
	if c.pairingRunning {
		c.mu.Unlock()
		return ErrPairingInProgress
	}
	pairCtx, cancel := context.WithTimeout(c.ctx, pairingWindow)
	c.pairingRunning = true
	c.pairingCancel = cancel
	c.mu.Unlock()

	c.setStatus(StatusConnecting, nil)
	metrics.QRRequests.Inc()
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer cancel()
		defer func() {
			c.mu.Lock()
			c.pairingRunning = false
			c.pairingCancel = nil
			c.mu.Unlock()
		}()
		c.pairingLoop(pairCtx)
	}()
	return nil
}

func (c *Client) pairingLoop(ctx context.Context) {
	rotations := 0
	for {
		if ctx.Err() != nil {
			c.setStatus(StatusDisconnected, errors.New("pairing window expired, start a new pairing"))
			return
		}
		c.mu.RLock()
		paired := c.wa.Store.ID != nil
		closed := c.closed
		c.mu.RUnlock()
		if paired || closed {
			return
		}

		ch, err := c.wa.GetQRChannel(ctx)
		if err != nil {
			if errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
				_ = c.connectSocket()
				return
			}
			c.setStatus(StatusError, err)
			return
		}
		if err := c.wa.Connect(); err != nil {
			if !errors.Is(err, whatsmeow.ErrAlreadyConnected) {
				c.setStatus(StatusError, err)
				return
			}
		}

		rotations++
		rotate := false
		for item := range ch {
			switch item.Event {
			case whatsmeow.QRChannelEventCode:
				c.setQR(item.Code, item.Timeout)
			case whatsmeow.QRChannelEventError:
				c.log.Warn("pairing error", "event", "pair", "result", "failure", "error", item.Error)
				c.setStatus(StatusError, item.Error)
				return
			case "success":
			case "timeout":
				rotate = true
			default:
			}
		}

		c.mu.RLock()
		paired = c.wa.Store.ID != nil
		closed = c.closed
		c.mu.RUnlock()
		if paired {
			c.clearQR()
			c.setStatus(StatusConnecting, nil)
			return
		}
		if closed || !rotate || rotations >= pairingRotations {
			if rotations >= pairingRotations {
				c.setStatus(StatusDisconnected, errors.New("pairing window expired, start a new pairing"))
			}
			return
		}
		select {
		case <-ctx.Done():
			c.setStatus(StatusDisconnected, errors.New("pairing window expired, start a new pairing"))
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (c *Client) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrInstanceClosed
	}
	c.manualDisconnect = true
	if c.pairingCancel != nil {
		c.pairingCancel()
	}
	c.clearQRLocked()
	c.mu.Unlock()

	c.log.Info("disconnecting instance", "event", "disconnect")
	c.wa.Disconnect()
	c.setStatus(StatusDisconnected, nil)
	return nil
}

func (c *Client) Restart(ctx context.Context) error {
	if err := c.Disconnect(ctx); err != nil && !errors.Is(err, ErrInstanceClosed) {
		return err
	}
	if !c.IsPaired() {
		return c.StartPairing(ctx)
	}
	return c.connectSocket()
}

func (c *Client) Logout(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrInstanceClosed
	}
	c.manualDisconnect = true
	if c.pairingCancel != nil {
		c.pairingCancel()
	}
	c.clearQRLocked()
	c.mu.Unlock()

	err := c.wa.Logout(ctx)
	if err != nil && !errors.Is(err, whatsmeow.ErrNotLoggedIn) {
		return fmt.Errorf("logout instance: %w", err)
	}

	c.mu.Lock()
	c.loggedOut = true
	c.phone = ""
	c.waJID = ""
	c.mu.Unlock()

	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(c.ctx), persistTimeout)
	defer cancel()
	if err := c.repo.ClearIdentity(persistCtx, c.id); err != nil {
		c.log.Error("failed to clear instance identity", "event", "logout_cleanup", "result", "failure", "error", err)
	}
	c.setStatus(StatusLoggedOut, nil)
	return nil
}

func (c *Client) Delete(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.manualDisconnect = true
	if c.pairingCancel != nil {
		c.pairingCancel()
	}
	c.clearQRLocked()
	c.mu.Unlock()

	c.wa.Disconnect()
	if c.wa.Store.ID != nil {
		if err := c.wa.Store.Delete(context.WithoutCancel(ctx)); err != nil {
			c.log.Warn("failed to delete whatsapp session state", "event", "delete_session", "result", "failure", "error", err)
		}
	}
	return nil
}

func (c *Client) Shutdown() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	if c.pairingCancel != nil {
		c.pairingCancel()
	}
	c.mu.Unlock()
	c.wa.Disconnect()
}

func (c *Client) QR() (string, time.Time, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.qrCode == "" {
		return "", time.Time{}, errors.New("no QR code available")
	}
	return c.qrCode, c.qrExpiresAt, nil
}

func (c *Client) setQR(code string, timeout time.Duration) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	c.mu.Lock()
	c.qrCode = code
	c.qrExpiresAt = time.Now().Add(timeout)
	c.mu.Unlock()
	c.setStatus(StatusQRRequired, nil)
}

func (c *Client) clearQR() {
	c.mu.Lock()
	c.clearQRLocked()
	c.mu.Unlock()
}

func (c *Client) clearQRLocked() {
	c.qrCode = ""
	c.qrExpiresAt = time.Time{}
}

func (c *Client) setStatus(status string, cause error) {
	c.mu.Lock()
	old := c.status
	c.status = status
	if cause != nil {
		c.lastErr = cause.Error()
	} else if status != StatusError {
		c.lastErr = ""
	}
	c.mu.Unlock()

	if old == status && cause == nil {
		return
	}
	if old != "" {
		metrics.Instances.WithLabelValues(old).Dec()
	}
	metrics.Instances.WithLabelValues(status).Inc()
	c.log.Info("instance state changed", "event", "state_change", "from", old, "to", status, "result", statusResult(status))
	c.persistStatus(status)
	c.emitStateEvents(old, status)
}

func statusResult(status string) string {
	if status == StatusError || status == StatusLoggedOut {
		return "failure"
	}
	return "success"
}

func (c *Client) persistStatus(status string) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.ctx), persistTimeout)
		defer cancel()
		if err := c.repo.UpdateStatus(ctx, c.id, status); err != nil {
			c.log.Error("failed to persist instance status", "event", "persist_status", "result", "failure", "error", err)
		}
	}()
}

func (c *Client) emitStateEvents(old, new string) {
	if c.notifier == nil {
		return
	}
	now := time.Now()
	base := InstanceEvent{
		InstanceID: c.id,
		ShopID:     c.shopID,
		Phone:      c.Snapshot().Phone,
		Timestamp:  now,
		EventID:    uuid.NewString(),
	}
	switch {
	case new == StatusConnected && old != StatusConnected:
		base.Type = EventConnected
		c.notifier.InstanceEvent(base)
	case old == StatusConnected && (new == StatusReconnecting || new == StatusDisconnected):
		base.Type = EventDisconnected
		base.Reason = c.Snapshot().LastError
		c.notifier.InstanceEvent(base)
	case new == StatusLoggedOut && old != StatusLoggedOut:
		base.Type = EventLoggedOut
		c.notifier.InstanceEvent(base)
	}
}

func (c *Client) superviseReconnect() {
	c.mu.Lock()
	if c.reconnectRunning || c.closed || c.manualDisconnect || c.loggedOut || c.wa.Store.ID == nil {
		c.mu.Unlock()
		return
	}
	c.reconnectRunning = true
	c.mu.Unlock()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer func() {
			c.mu.Lock()
			c.reconnectRunning = false
			c.mu.Unlock()
		}()
		attempt := 0
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(reconnectDelay(attempt)):
			}
			c.mu.RLock()
			stop := c.closed || c.manualDisconnect || c.loggedOut
			c.mu.RUnlock()
			if stop {
				return
			}
			if c.wa.IsConnected() {
				return
			}
			attempt++
			metrics.ReconnectsTotal.Inc()
			c.setStatus(StatusReconnecting, nil)
			c.log.Info("reconnecting instance", "event", "reconnect", "attempt", attempt)
			if err := c.wa.Connect(); err != nil {
				if errors.Is(err, whatsmeow.ErrAlreadyConnected) {
					return
				}
				c.setStatus(StatusReconnecting, err)
				continue
			}
			if c.wa.WaitForConnection(connectTimeout) {
				return
			}
			c.setStatus(StatusReconnecting, errors.New("connection attempt timed out"))
		}
	}()
}

func reconnectDelay(attempt int) time.Duration {
	d := reconnectBase
	for i := 0; i < attempt && d < reconnectMax; i++ {
		d *= 2
	}
	if d > reconnectMax {
		d = reconnectMax
	}
	jitter := (rand.Float64()*2 - 1) * 0.2
	return d + time.Duration(float64(d)*jitter)
}
