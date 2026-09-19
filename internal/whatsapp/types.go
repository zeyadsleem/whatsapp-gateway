package whatsapp

import (
	"context"
	"errors"
	"time"

	"database/sql"

	"go.mau.fi/whatsmeow/store/sqlstore"
)

const (
	StatusDisconnected = "disconnected"
	StatusConnecting   = "connecting"
	StatusQRRequired   = "qr_required"
	StatusConnected    = "connected"
	StatusReconnecting = "reconnecting"
	StatusLoggedOut    = "logged_out"
	StatusError        = "error"
)

var (
	ErrNotFound          = errors.New("instance not found")
	ErrNotConnected      = errors.New("instance is not connected")
	ErrNotPaired         = errors.New("instance is not paired yet")
	ErrAlreadyPaired     = errors.New("instance is already paired")
	ErrInstanceClosed    = errors.New("instance is closed")
	ErrPairingInProgress = errors.New("pairing already in progress")
	ErrNotPairing        = errors.New("instance is not in pairing mode")
	ErrMaxInstances      = errors.New("maximum number of instances reached")
	ErrNoDevice          = errors.New("no whatsapp session found for instance")
)

type EventType string

const (
	EventConnected    EventType = "instance.connected"
	EventDisconnected EventType = "instance.disconnected"
	EventLoggedOut    EventType = "instance.logged_out"
	EventMessage      EventType = "message.received"
)

type InstanceEvent struct {
	Type       EventType
	InstanceID string
	ShopID     string
	Phone      string
	Reason     string
	Timestamp  time.Time
	EventID    string
}

type MessageEvent struct {
	InstanceID string
	ShopID     string
	From       string
	MessageID  string
	Type       string
	Text       string
	Timestamp  time.Time
	EventID    string
}

type Notifier interface {
	InstanceEvent(evt InstanceEvent)
	MessageEvent(evt MessageEvent)
}

type Repository interface {
	UpdateStatus(ctx context.Context, id, status string) error
	SetIdentity(ctx context.Context, id, waJID, phone string) error
	ClearIdentity(ctx context.Context, id string) error
	MarkConnected(ctx context.Context, id string, at time.Time) error
}

type SendResult struct {
	MessageID string
	Timestamp time.Time
}

type Status struct {
	ID          string    `json:"id"`
	ShopID      string    `json:"shop_id"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	Phone       string    `json:"phone_number,omitempty"`
	WAJID       string    `json:"-"`
	LastError   string    `json:"last_error,omitempty"`
	QRCode      string    `json:"qr_code,omitempty"`
	QRExpiresAt time.Time `json:"qr_expires_at,omitempty"`
	Connected   bool      `json:"connected"`
	SentOK      int64     `json:"sent_ok"`
	SentFailed  int64     `json:"sent_failed"`
	Received    int64     `json:"received"`
}

type Stats struct {
	SentOK     int64 `json:"sent_ok"`
	SentFailed int64 `json:"sent_failed"`
	Received   int64 `json:"received"`
}

type clientConfig struct {
	ID          string
	ShopID      string
	DisplayName string
	RestoreJID  string
	Container   *sqlstore.Container
	WAStore     *sql.DB
}
