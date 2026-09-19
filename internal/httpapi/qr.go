package httpapi

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/zeyad/whatsapp-gateway/internal/whatsapp"
)

type qrResponse struct {
	Status      string `json:"status"`
	Connected   bool   `json:"connected"`
	QRCode      string `json:"qr_code,omitempty"`
	QRPNGBase64 string `json:"qr_png_base64,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
}

func generateQRPNG(code string) ([]byte, error) {
	return qrcode.Encode(code, qrcode.Medium, 512)
}

func (s *Server) handleInstanceQR(c *gin.Context) {
	inst, ok := s.loadInstance(c)
	if !ok {
		return
	}
	client, ok := s.manager.Get(inst.ID)
	if !ok {
		respondError(c, http.StatusNotFound, "not_found", "instance not found in manager")
		return
	}

	snap := client.Snapshot()
	if snap.Status == whatsapp.StatusConnected {
		respond(c, http.StatusOK, qrResponse{Status: snap.Status, Connected: true})
		return
	}

	code, expires, err := client.QR()
	if err != nil {
		if snap.Status == whatsapp.StatusConnecting || snap.Status == whatsapp.StatusQRRequired {
			respond(c, http.StatusAccepted, qrResponse{Status: snap.Status, Connected: false})
			return
		}
		if err := client.Connect(c.Request.Context()); err != nil {
			s.fail(c, err, "failed to start pairing")
			return
		}
		respond(c, http.StatusAccepted, qrResponse{Status: whatsapp.StatusConnecting, Connected: false})
		return
	}

	if c.Query("format") == "png" {
		png, err := generateQRPNG(code)
		if err != nil {
			s.fail(c, err, "failed to render QR code")
			return
		}
		c.Data(http.StatusOK, "image/png", png)
		return
	}

	png, err := generateQRPNG(code)
	if err != nil {
		s.fail(c, err, "failed to render QR code")
		return
	}
	respond(c, http.StatusOK, qrResponse{
		Status:      whatsapp.StatusQRRequired,
		Connected:   false,
		QRCode:      code,
		QRPNGBase64: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		ExpiresAt:   expires.UTC().Format(time.RFC3339),
	})
}
