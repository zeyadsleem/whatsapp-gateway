# API Guide

Base URL: `https://your-gateway.example.com`

- Admin endpoints use `Authorization: Bearer $ADMIN_API_KEY`.
- Shop endpoints use `Authorization: Bearer $SHOP_API_KEY` (the key selects the tenant).
- Errors use `{"error":{"code":"...","message":"..."}}`.
- Swagger UI: `/docs` — OpenAPI spec: `/api/v1/openapi.yaml`.

## Admin: create an instance

```bash
curl -X POST https://your-gateway.example.com/api/v1/admin/instances \
  -H "Authorization: Bearer $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "shop_id": "shop_125",
    "display_name": "فرع مدينة نصر",
    "webhook_url": "https://example.com/webhooks/whatsapp"
  }'
```

Response `201` (the API key and webhook secret are shown **once**):

```json
{
  "id": "6d1e0f2e-...",
  "shop_id": "shop_125",
  "display_name": "فرع مدينة نصر",
  "status": "disconnected",
  "webhook_url": "https://example.com/webhooks/whatsapp",
  "webhook_enabled": true,
  "created_at": "2026-09-18T12:00:00Z",
  "api_key": "wgs_...",
  "webhook_secret": "whsec_..."
}
```

## Admin: list and inspect instances

```bash
curl https://your-gateway.example.com/api/v1/admin/instances \
  -H "Authorization: Bearer $ADMIN_API_KEY"

curl https://your-gateway.example.com/api/v1/admin/instances/$INSTANCE_ID \
  -H "Authorization: Bearer $ADMIN_API_KEY"
```

The list never contains messages, contacts or session credentials, and never
returns plaintext API keys or webhook secrets.

## Admin: connect and pair via QR

```bash
# Start pairing (or reconnect an existing session)
curl -X POST .../api/v1/admin/instances/$ID/connect \
  -H "Authorization: Bearer $ADMIN_API_KEY"

# Poll for the QR (202 means "pairing is starting, retry shortly")
curl .../api/v1/admin/instances/$ID/qr \
  -H "Authorization: Bearer $ADMIN_API_KEY"
```

Response `200`:

```json
{
  "status": "qr_required",
  "connected": false,
  "qr_code": "2@abc...",
  "qr_png_base64": "data:image/png;base64,...",
  "expires_at": "2026-09-18T12:00:40Z"
}
```

- `GET .../qr?format=png` returns `image/png` directly.
- QR codes rotate every 20–60 seconds; the endpoint always returns the current one.
- When pairing succeeds, `status` becomes `connected` and `phone_number` is set.

## Admin: connection actions

```bash
# Temporary disconnect (keeps the session, no QR needed to resume)
curl -X POST .../api/v1/admin/instances/$ID/disconnect -H "Authorization: Bearer $ADMIN_API_KEY"

# Restart the socket
curl -X POST .../api/v1/admin/instances/$ID/restart -H "Authorization: Bearer $ADMIN_API_KEY"

# Log out: removes the linked device on WhatsApp (QR required to pair again)
curl -X POST .../api/v1/admin/instances/$ID/logout -H "Authorization: Bearer $ADMIN_API_KEY"

# Delete: removes the instance, its API key and its WhatsApp session
curl -X DELETE .../api/v1/admin/instances/$ID -H "Authorization: Bearer $ADMIN_API_KEY"
```

## Admin: webhook configuration

```bash
curl -X PUT .../api/v1/admin/instances/$ID/webhook \
  -H "Authorization: Bearer $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "webhook_url": "https://example.com/webhooks/whatsapp",
    "webhook_enabled": true,
    "rotate_webhook_secret": true
  }'
```

`webhook_secret` is returned only when it was newly generated. Private,
loopback and `.local`/`.internal` targets are rejected.

## Admin: rotate a shop API key

```bash
curl -X POST .../api/v1/admin/instances/$ID/api-key/rotate \
  -H "Authorization: Bearer $ADMIN_API_KEY"
# → {"api_key":"wgs_..."}  (shown once; the old key stops working immediately)
```

## Admin: stats and health

```bash
curl .../api/v1/admin/stats -H "Authorization: Bearer $ADMIN_API_KEY"
# → {"total":12,"by_status":{"connected":10,...},"sent_ok":1523,"sent_failed":4,"received":88}

curl https://your-gateway.example.com/health
# → {"status":"ok","uptime_seconds":3600,"database":"up","instances":{...},"version":"1.0.0"}
```

## Shop: send text

```bash
curl -X POST https://your-gateway.example.com/api/v1/messages/text \
  -H "Authorization: Bearer $SHOP_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"to":"201001234567","text":"تم تجهيز طلبك رقم 551"}'
```

```json
{"success": true, "message_id": "3EB0A1B2C3", "timestamp": "2026-09-18T12:00:00Z"}
```

Validation: `to` accepts `+`, spaces, dashes and parentheses and is normalized;
`text` must be non-empty and at most 20,000 characters. If the instance is not
connected the API returns `409 instance_not_connected`.

## Shop: send an image

```bash
# Multipart upload (max 16 MB by default)
curl -X POST .../api/v1/messages/image \
  -H "Authorization: Bearer $SHOP_API_KEY" \
  -F "to=201001234567" \
  -F "caption=صورة المنتج" \
  -F "file=@product.jpg"

# Or fetch from a public URL
curl -X POST .../api/v1/messages/image \
  -H "Authorization: Bearer $SHOP_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"to":"201001234567","url":"https://cdn.example.com/product.jpg","caption":"صورة المنتج"}'
```

Supported image types: JPEG, PNG, WebP, GIF (content-sniffed, not trusted from
the filename). Media is streamed, sent, and discarded; no temporary files.

## Shop: send a document

```bash
curl -X POST .../api/v1/messages/document \
  -H "Authorization: Bearer $SHOP_API_KEY" \
  -F "to=201001234567" \
  -F "filename=invoice.pdf" \
  -F "caption=فاتورة الطلب 551" \
  -F "file=@invoice.pdf"
```

Allowed document types include PDF, Office formats, plain text, CSV, JSON, XML
and ZIP. Executable and script types are rejected with `415`.

## Webhooks: events and signature verification

Events: `message.received`, `instance.connected`, `instance.disconnected`,
`instance.logged_out`.

`message.received` payload:

```json
{
  "event": "message.received",
  "event_id": "b0f2c1d4-...",
  "instance_id": "6d1e0f2e-...",
  "shop_id": "shop_125",
  "from": "201001234567",
  "message_id": "3EB0A1B2C3",
  "type": "text",
  "text": "1",
  "timestamp": "2026-09-18T12:00:05Z"
}
```

Headers:

```
X-Webhook-Event: message.received
X-Webhook-Timestamp: 2026-09-18T12:00:05Z
X-Webhook-Signature: sha256=<hex>
X-Instance-ID: 6d1e0f2e-...
X-Webhook-Event-ID: b0f2c1d4-...
```

Signature input: `X-Webhook-Timestamp + "." + rawBody`.

Idempotency: deduplicate on `message_id` (stable WhatsApp ID) or `event_id`
(stable across retries of the same event). The gateway itself keeps no
permanent dedupe store.

### Verify in Node.js

```js
import crypto from "node:crypto";

export function verifyWebhook(secret, timestamp, rawBody, signatureHeader) {
  const expected = "sha256=" + crypto
    .createHmac("sha256", secret)
    .update(`${timestamp}.${rawBody}`)
    .digest("hex");
  return crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(signatureHeader));
}
```

### Verify in Go

```go
func verify(secret []byte, timestamp string, body []byte, header string) bool {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(header))
}
```

### Verify in Python

```python
import hashlib, hmac

def verify(secret: bytes, timestamp: str, body: bytes, header: str) -> bool:
    expected = "sha256=" + hmac.new(secret, f"{timestamp}.".encode() + body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, header)
```

Always use the raw request body (before JSON parsing) for verification.
