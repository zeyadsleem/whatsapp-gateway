package database

import "time"

type InstanceStatus = string

const (
	StatusDisconnected InstanceStatus = "disconnected"
	StatusConnecting   InstanceStatus = "connecting"
	StatusQRRequired   InstanceStatus = "qr_required"
	StatusConnected    InstanceStatus = "connected"
	StatusReconnecting InstanceStatus = "reconnecting"
	StatusLoggedOut    InstanceStatus = "logged_out"
	StatusError        InstanceStatus = "error"
)

func ValidStatus(s string) bool {
	switch s {
	case StatusDisconnected, StatusConnecting, StatusQRRequired, StatusConnected,
		StatusReconnecting, StatusLoggedOut, StatusError:
		return true
	}
	return false
}

type Instance struct {
	ID               string     `json:"id"`
	ShopID           string     `json:"shop_id"`
	DisplayName      string     `json:"display_name"`
	WAJID            *string    `json:"-"`
	PhoneNumber      *string    `json:"phone_number"`
	Status           string     `json:"status"`
	WebhookURL       *string    `json:"webhook_url,omitempty"`
	WebhookSecretEnc []byte     `json:"-"`
	WebhookEnabled   bool       `json:"webhook_enabled"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	LastConnectedAt  *time.Time `json:"last_connected_at"`
}

type APIKey struct {
	ID         string
	InstanceID string
	KeyHash    string
	KeyPrefix  string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}
