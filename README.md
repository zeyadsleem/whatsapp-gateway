# WhatsApp Gateway

A lightweight, multi-tenant WhatsApp gateway for shops. Each shop links its own
WhatsApp number as a **Linked Device** via QR, then its systems (POS,
attendance, booking, notifications) send messages through a per-shop REST API
and receive incoming messages through signed webhooks.

Built in Go with the official [`go.mau.fi/whatsmeow`](https://github.com/tulir/whatsmeow)
library, PostgreSQL and an Arabic RTL admin dashboard. No Evolution API, no
Baileys, no License server, no telemetry, no external services.

**Privacy-first:** the gateway never stores message text, chat history,
conversations or contact books. See [PRIVACY.md](PRIVACY.md).

## Features

- Multi-instance manager (designed for 50+ concurrent accounts, configurable to 100+)
- QR pairing with live rotation, expiry countdown and automatic refresh
- Persistent sessions: restart the container and sessions reconnect without a new QR
- Automatic reconnect with exponential backoff and jitter
- Send text, images and documents (multipart upload or URL fetch, SSRF-protected)
- Incoming messages forwarded to per-shop webhooks with HMAC-SHA256 signatures
- Per-shop API keys (SHA-256 hashed at rest, shown once) with tenant isolation
- Admin API key, rate limiting per key, request IDs, structured logs
- Prometheus metrics, health endpoint, graceful shutdown
- Arabic RTL admin dashboard (React + Vite + TypeScript), swagger UI, OpenAPI spec

## Quick start

```bash
cp .env.example .env
# edit .env: set POSTGRES_PASSWORD, ADMIN_API_KEY, APP_ENCRYPTION_KEY
docker compose up -d
```

Generate the two secrets with:

```bash
openssl rand -base64 48   # ADMIN_API_KEY
openssl rand -base64 48   # APP_ENCRYPTION_KEY
```

Then open:

| URL | Purpose |
|---|---|
| `http://localhost:8080/admin/` | Arabic admin dashboard (login with `ADMIN_API_KEY`) |
| `http://localhost:8080/health` | Health check |
| `http://localhost:8080/docs` | Swagger UI |
| `http://localhost:8080/metrics` | Prometheus metrics |
| `http://localhost:8080/api/v1/openapi.yaml` | OpenAPI 3.0 spec |

### First steps

1. Log in to the dashboard with your `ADMIN_API_KEY`.
2. Click **إضافة اتصال واتساب** and enter the shop id, display name and webhook URL.
3. Save the **API key** and **webhook secret** shown once, then click **الانتقال إلى الربط**.
4. On the shop owner's phone: WhatsApp → Linked Devices → Link a Device → scan the QR.
5. The dashboard shows the shop WhatsApp number and **متصل**.

## Sending a message

```bash
curl -X POST https://wa.example.com/api/v1/messages/text \
  -H "Authorization: Bearer $SHOP_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"to":"201001234567","text":"موعدك غدًا الساعة 6. رد 1 للتأكيد."}'
```

```json
{"success": true, "message_id": "3EB0...", "timestamp": "2026-09-18T12:00:00Z"}
```

The shop API key determines the tenant. There is no way to select another
shop's instance from the shop API. See [docs/API.md](docs/API.md) for all
endpoints, media sending and webhook verification examples.

## Receiving a message (webhook)

When a customer replies `1`, the shop receives:

```json
{
  "event": "message.received",
  "event_id": "b0f2...",
  "instance_id": "6d1e...",
  "shop_id": "shop_125",
  "from": "201001234567",
  "message_id": "3EB0...",
  "type": "text",
  "text": "1",
  "timestamp": "2026-09-18T12:00:05Z"
}
```

Other events: `instance.connected`, `instance.disconnected`, `instance.logged_out`.
Every request carries:

```
X-Webhook-Event: message.received
X-Webhook-Timestamp: 2026-09-18T12:00:05Z
X-Webhook-Signature: sha256=<hmac>
X-Instance-ID: <instance uuid>
X-Webhook-Event-ID: <unique, stable across retries>
```

Signature: `HMAC-SHA256(secret, timestamp + "." + rawBody)`.

Delivery behavior: 5s timeout, up to 3 retries with exponential backoff,
bounded in-memory queue. Webhooks are **not** persisted; if the process stops
during a retry, that event is dropped by design.

## Production deployment

- Put the app behind a reverse proxy / Cloudflare Tunnel that terminates TLS.
  The compose file binds `127.0.0.1:8080` by default; set `APP_BIND=0.0.0.0`
  only when the network is trusted.
- Firewall `/metrics` and `/docs` if they should not be public.
- Back up the PostgreSQL volume (`pgdata`). A backup contains WhatsApp session
  keys and shop metadata — treat it as sensitive.
- Migrations run automatically at startup inside an advisory lock.
- Updates: `docker compose pull && docker compose build && docker compose up -d`.
  Sessions survive restarts; no QR re-pairing is needed.
- The WhatsApp multi-device protocol changes over time. See
  [docs/UPDATING-WHATSMEOW.md](docs/UPDATING-WHATSMEOW.md) for the safe
  dependency update procedure.
- Security guidance lives in [SECURITY.md](SECURITY.md).

## Development

Requires Go 1.24+, Node 22+ with pnpm, and a PostgreSQL instance for
integration tests.

```bash
make web            # build the admin dashboard into web/admin/dist
make build          # build the Go binary
make run            # run locally (needs .env)
make test           # unit tests (integration tests skip without TEST_DATABASE_URL)
make test-race      # race detector
make test-integration TEST_DATABASE_URL=postgres://user:pass@localhost:5432/wg_test
make privacy-audit  # inspect a running database for forbidden tables/columns
```

Integration tests create and drop an isolated database per test package, so
`TEST_DATABASE_URL` needs `CREATEDB` privileges.

Architecture and database design: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Project layout

```
cmd/server/           entrypoint
internal/config/      environment configuration
internal/database/    pgx pool, migrations, repositories
internal/auth/        API key generation, hashing, verification
internal/crypto/      AES-256-GCM for webhook secrets
internal/instance/    application service (create, update, lifecycle)
internal/whatsapp/    whatsmeow wrapper, InstanceManager, pairing, reconnect
internal/message/     phone normalization, validation, media, send service
internal/webhook/     HMAC signer, bounded retry dispatcher
internal/httpapi/     Gin router, middleware, handlers, OpenAPI
internal/metrics/     Prometheus collectors
internal/safehttp/    SSRF-safe HTTP client
web/admin/            React + Vite + TypeScript Arabic RTL dashboard
migrations embedded in internal/database/migrations/
```

## License

See [LICENSE](LICENSE).
