# Security

## Secrets and key handling

| Secret | Storage | Exposure |
|---|---|---|
| `ADMIN_API_KEY` | Environment only | Never logged, compared in constant time |
| `APP_ENCRYPTION_KEY` | Environment only | Derives AES-256-GCM key for webhook secrets |
| Shop API keys | SHA-256 hash in `app.api_keys` | Plaintext shown once at creation/rotation |
| Webhook secrets | AES-256-GCM encrypted (`app.instances.webhook_secret_enc`) | Plaintext shown once at creation/rotation |
| WhatsApp session keys | `whatsapp` schema (whatsmeow) | Never exposed through any endpoint |

- API keys are high-entropy random values (`wgs_` + 32 random bytes). SHA-256
  is appropriate for hashing them; they are not passwords.
- Bearer extraction is strict and comparisons use `crypto/subtle`.
- Rotation invalidates the previous shop key immediately.

## Authentication and authorization

- Admin endpoints require `Authorization: Bearer <ADMIN_API_KEY>`.
- Shop endpoints require a per-shop key. The key resolves the tenant; the body
  cannot select another instance (unknown fields are rejected).
- Shop keys cannot access admin endpoints and the admin key cannot access shop
  endpoints.
- Tenant isolation is covered by `TestShopAPIKeyRotationAndIsolation`.

## Input handling

- Strict JSON decoding with `DisallowUnknownFields` on every endpoint.
- Request body limits: 64 KB JSON, 256 KB admin, media capped by `MAX_UPLOAD_MB`.
- Phone numbers are normalized to E.164 digits and validated (7–15 digits).
- Media: images are content-sniffed; documents are restricted to a safe MIME
  allowlist (executables, shell scripts and archives of that kind are rejected).
- Multipart uploads are streamed into memory with hard limits and never written
  to temporary files.

## SSRF protection

Webhook URLs and media URLs are validated twice:

1. At configuration time: only `http`/`https`, no `localhost`, `*.local`,
   `*.internal`, no literal private/loopback/link-local/CGNAT addresses.
2. At dial time: DNS is resolved and every candidate IP is checked again before
   connecting; redirects are re-validated and limited to 3 hops.

This blocks DNS-rebinding tricks and cloud metadata endpoints
(`169.254.169.254`).

## Webhooks

- Every delivery is signed with `HMAC-SHA256(secret, timestamp + "." + body)`.
- Consumers should compare signatures in constant time and reject stale
  timestamps.
- Delivery failures are retried in memory only; payloads are never persisted.
- Redirects during delivery are re-validated against the SSRF rules.

## Rate limiting

- Per shop API key and for the admin key, using token buckets
  (`SHOP_RATE_LIMIT_RPS` / `SHOP_RATE_LIMIT_BURST`, admin equivalent).
- Rejections return `429` and increment `whatsapp_gateway_rate_limited_total`.
- No bulk/broadcast features exist; the gateway is designed for transactional
  notifications tied to shop systems.

## HTTP hardening

- Security headers on every response: `X-Content-Type-Options`, `X-Frame-Options:
  DENY`, `Referrer-Policy`, `Permissions-Policy`, restrictive CSP, and HSTS in
  production.
- CORS is same-origin unless `ALLOWED_ORIGINS` is explicitly configured.
- Request IDs (`X-Request-ID`) on every request for correlation.
- Server timeouts (read header 10s, read 30s, write 60s, idle 120s).

## Logging hygiene

- Never logged: API keys, webhook secrets, QR payloads, message content,
  session credentials.
- Customer phone numbers are masked (`2010******67`) by the log adapter; the
  application logs do not include them at all.
- The whatsmeow logger is wrapped by a sanitizing adapter
  (`internal/logger/logger.go`), including at debug level.

## Dependencies

- The Go module pins `go.mau.fi/whatsmeow` to a specific pseudo-version.
  Update deliberately (see [UPDATING-WHATSMEOW.md](UPDATING-WHATSMEOW.md)).
- `go.sum` is committed; run `go mod verify` in CI.
- The container runs as a non-root user and contains only the static binary,
  CA certificates and a minimal Alpine base.

## Deployment checklist

- [ ] `ADMIN_API_KEY` and `APP_ENCRYPTION_KEY` generated with `openssl rand -base64 48`
- [ ] PostgreSQL not exposed publicly; strong password
- [ ] TLS terminated by a reverse proxy; `APP_BIND=127.0.0.1`
- [ ] `/metrics` and `/docs` restricted if not needed publicly
- [ ] Regular encrypted backups of the PostgreSQL volume (contains session keys)
- [ ] `LOG_LEVEL=info` in production (debug is verbose but still redacted)

## Reporting a vulnerability

Open a private security advisory or contact the maintainer directly. Please do
not file public issues for security problems.
