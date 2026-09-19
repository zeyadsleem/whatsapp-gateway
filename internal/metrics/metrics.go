package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	Instances = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "whatsapp_gateway_instances",
		Help: "Number of WhatsApp instances by connection status.",
	}, []string{"status"})

	SendTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "whatsapp_gateway_send_requests_total",
		Help: "Total outgoing message send requests.",
	})

	SendFailures = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "whatsapp_gateway_send_failures_total",
		Help: "Total failed outgoing message send requests.",
	})

	ReceivedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "whatsapp_gateway_received_messages_total",
		Help: "Total incoming messages processed.",
	})

	WebhookTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "whatsapp_gateway_webhook_deliveries_total",
		Help: "Total webhook delivery attempts by result.",
	}, []string{"result"})

	WebhookDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "whatsapp_gateway_webhook_dropped_total",
		Help: "Webhook deliveries dropped because the in-memory queue was full.",
	})

	ReconnectsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "whatsapp_gateway_reconnects_total",
		Help: "Total automatic reconnect attempts.",
	})

	QRRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "whatsapp_gateway_qr_pairing_starts_total",
		Help: "Total QR pairing sessions started.",
	})

	RateLimited = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "whatsapp_gateway_rate_limited_total",
		Help: "Total requests rejected by rate limiting.",
	})
)

func init() {
	prometheus.MustRegister(
		Instances,
		SendTotal,
		SendFailures,
		ReceivedTotal,
		WebhookTotal,
		WebhookDropped,
		ReconnectsTotal,
		QRRequests,
		RateLimited,
	)
}

func Handler() http.Handler {
	return promhttp.Handler()
}
