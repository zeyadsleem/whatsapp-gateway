# Architecture — WhatsApp Gateway

A small, privacy-first, multi-tenant WhatsApp gateway. Shops link their own WhatsApp
number as a Linked Device via QR, then their internal systems (POS, attendance,
booking, notifications) send messages through a per-shop REST API and receive
incoming messages through signed webhooks.

The gateway is **not** a CRM, inbox, or chat manager. It stores connection state,
never conversation data.

## Components

```
                    +----------------------------+
  Admin Browser --->|  Go HTTP server (Gin)      |
  (Arabic RTL SPA)  |                            |
                    |  /api/v1/admin/*  (admin)  |
  Shop systems ---->|  /api/v1/messages/* (shop) |----> InstanceManager
  (API key)         |  /health /metrics /docs    |          |
                    +----------------------------+          |
                                    |                       v
                                    |              +---------------------+
                                    |              | WhatsAppClient      |
                                    |              | (one per instance)  |
                                    |              | go.mau.fi/whatsmeow |
                                    |              +----------+----------+
                                    |                         |
                                    v                         v
                          +------------------+       WhatsApp Web
                          | WebhookDispatcher|       Multi-Device
                          | bounded pool +   |
                          | HMAC + retries   |
                          +---------+--------+
                                    |
                                    v
                          Shop webhook endpoint
```

### Packages

| Package | Responsibility |
|---|---|
| `internal/config` | Environment loading, validation, safe defaults |
| `internal/logger` | `log/slog` setup + sanitizing `waLog.Logger` adapter |
| `internal/database` | pgx pool, embedded migrations, repositories |
| `internal/auth` | Key generation, SHA-256 hashing, constant-time verify |
| `internal/instance` | Application service: create/update/delete instances, API keys, webhook config |
| `internal/whatsapp` | whatsmeow wrapper, `InstanceManager`: registry, state machine, QR pairing, reconnect, send/receive |
| `internal/message` | Incoming/outgoing message translation, phone normalization |
| `internal/webhook` | HMAC signing, bounded retry dispatcher |
| `internal/httpapi` | Gin router, middleware, handlers, QR image rendering |
| `internal/metrics` | Prometheus collectors (no personal labels) |
| `internal/ratelimit` | Per-API-key token buckets |

## Database layout

One PostgreSQL database, two schemas:

### `app` — application data (owned by us)

```sql
app.instances (
  id                uuid primary key,
  shop_id           text unique not null,
  display_name      text not null,
  wa_jid            text,              -- e.g. 201001234567@s.whatsapp.net, set after pairing
  phone_number      text,              -- display only, never a primary key
  status            text not null,     -- see state machine
  webhook_url       text,
  webhook_secret_enc bytea,            -- AES-256-GCM encrypted with APP_ENCRYPTION_KEY
  webhook_enabled   boolean not null default true,
  created_at        timestamptz not null default now(),
  updated_at        timestamptz not null default now(),
  last_connected_at timestamptz
)

app.api_keys (
  id           uuid primary key,
  instance_id  uuid not null references app.instances(id) on delete cascade,
  key_hash     text unique not null,   -- SHA-256 hex of the raw key
  key_prefix   text not null,          -- first 8 chars, for identification only
  created_at   timestamptz not null default now(),
  last_used_at timestamptz
)

app.schema_migrations (version int primary key, applied_at timestamptz)
```

There is deliberately **no** `messages`, `chats`, `contacts`, or `conversations`
table. Automation tests and `scripts/privacy-audit.sh` assert this against
`information_schema`.

### `whatsapp` — whatsmeow protocol state (owned by the library)

Created and upgraded by `sqlstore.Container.Upgrade()` with
`search_path=whatsapp,public`. It contains only what the WhatsApp Multi-Device
protocol requires to survive restarts: Noise/Signal keys, prekeys, sender keys,
app-state sync keys and versions, LID↔PN mappings, message secrets (needed to
decrypt your own device-echoes and receipts), and app-state derived entries
(push names/contact entries that arrive as part of app state sync).

Rules:

1. We never copy anything from `whatsapp.*` into `app.*`.
2. We never expose `whatsapp.*` through the API.
3. We never build features on top of it (no contacts UI, no message search).
4. See `PRIVACY.md` for the full audit.

The application pool and the whatsmeow pool are two separate `*sql.DB`/pgx pools
with different `search_path`, so unqualified whatsmeow queries can never touch
`app` and vice versa.

## WhatsApp lifecycle

### States

```
disconnected ──connect()──> connecting ──QR event──> qr_required
     ^                          |                        |
     |                          | Connected              | pair success
     |                          v                        v
     |<── manual disconnect ── connected <──────────────┘
     |                          |
     |                unexpected Disconnected/StreamError
     |                          v
     |                     reconnecting ──backoff──> connecting
     |                          |
     |                LoggedOut / permanent failure
     v                          v
 (manual)                  logged_out / error
```

| State | Meaning |
|---|---|
| `disconnected` | No socket, no reconnect scheduled. Session may still exist. |
| `connecting` | `Connect()` in flight or awaiting `Connected`. |
| `qr_required` | Pairing active; QR code available via API. |
| `connected` | Session validated, socket open. |
| `reconnecting` | Unexpected drop; exponential backoff running. |
| `logged_out` | Linked device removed (by phone or admin logout). |
| `error` | Unrecoverable failure (banned, client outdated, …). |

### Startup restore

1. Load all instances with `wa_jid` set and status in (`connected`, `disconnected`, `reconnecting`).
2. `sqlstore.Container.GetDevice(ctx, jid)` loads the persisted device.
3. Build a whatsmeow client and `Connect()` — no QR needed while the session is valid.
4. Instances without `wa_jid` (or with `logged_out`/`error`) stay idle until an admin connects them.

### Reconnect policy

- Triggered on `Disconnected`, `StreamError`, `KeepAliveTimeout`, `ConnectFailure` (recoverable), `TemporaryBan`.
- Exponential backoff: `min(2^n * 5s, 5m)` with ±20% jitter, reset on `Connected`.
- One goroutine per instance, driven by a cancellable context — no busy loops.
- Permanent failures (`LoggedOut`, `ClientOutdated`, certain `ConnectFailure` reasons) stop the loop and set `error`/`logged_out`.
- `TemporaryBan` backs off to the maximum interval rather than hammering.

### QR pairing

- `client.GetQRChannel(ctx)` **before** `Connect()`; the channel is only valid when `Store.ID == nil`.
- QR codes rotate every 20–60s. When the channel closes with `timeout`, the manager re-creates the pairing client and starts a fresh channel (bounded attempts / overall pairing window) so the admin screen always has a live code.
- On `PairSuccess`, the manager persists `wa_jid` + `phone_number`, transitions to `connected`, and fires the `instance.connected` webhook.
- Passkey-based pairing is not supported in the MVP; the API reports a clear error if the phone requests it.

## Request flows

### Send text

```
POST /api/v1/messages/text  (Authorization: Bearer <shop key>)
  → auth middleware: hash key, look up instance, attach to context
  → rate limit per key
  → validate & normalize destination (E.164, no +, ≤15 digits)
  → manager.Get(instance) → require status=connected
  → whatsmeow SendMessage(ctx, jid, text) with timeout
  → 200 {success, message_id, timestamp}   (message body discarded)
```

### Receive message

```
whatsmeow events.Message
  → filter: direct chats only, skip from_me echoes, skip protocol/empty types
  → extract minimal fields (id, sender, type, text if text, timestamp)
  → webhook dispatcher (in-memory queue) → HMAC-signed POST to shop
  → discard payload
```

No application writes happen on the receive path.

### Webhook delivery

- Queue: bounded in-memory channel (default 256), worker pool (default 4).
- Retries: up to `WEBHOOK_MAX_RETRIES` (default 3) with exponential backoff.
- Timeout: `WEBHOOK_TIMEOUT_SECONDS` (default 5s) per attempt.
- Signature: `X-Webhook-Signature: sha256=<hex(HMAC-SHA256(secret, timestamp + "." + raw_body))>`.
- Headers: `X-Webhook-Event`, `X-Webhook-Timestamp`, `X-Webhook-Signature`, `X-Instance-ID`, `X-Webhook-Event-ID`.
- On shutdown the queue is drained for a short window, then dropped. Dropping is the
  designed privacy trade-off: no message content is ever persisted for redelivery.

## Privacy boundaries

| Data | Stored in `app`? | Stored by whatsmeow? | Exposed via API? |
|---|---|---|---|
| Shop / instance metadata | Yes | No | Yes |
| Shop WhatsApp number | Yes | Yes (device JID) | Yes |
| Connection status | Yes | No | Yes |
| API key | Hash only | No | No (shown once at creation) |
| Webhook secret | Encrypted | No | Shown once; used for signing |
| Message text | **Never** | No | No |
| Message history | **Never** | **No** (history sync discarded) | No |
| Customer contacts | **Never** | Protocol-required app-state entries | No |
| Signal/session keys | No | Yes (required) | No |

`events.HistorySync` is received and ignored — the payload is never written to
`app.*`. whatsmeow itself only decrypts/acks the notification.

## Concurrency

- Target: 50 concurrent connected instances without design changes; 100 by config.
- `InstanceManager` holds a `sync.RWMutex`-guarded registry of per-instance `*WhatsAppClient`.
- Each client owns: a mutex-protected state, one event-handler goroutine (whatsmeow internal), at most one reconnect goroutine, at most one QR-pairing goroutine.
- All goroutines are tied to the manager context and cancelled on shutdown.
- Verified with `go test -race ./...`.

## Graceful shutdown

1. `http.Server.Shutdown` stops accepting requests (drain window).
2. Webhook dispatcher stops intake, drains briefly, cancels workers.
3. Manager cancels all reconnect/QR goroutines and calls `Disconnect()` (never `Logout`) on every client.
4. pgx pools and the whatsmeow `*sql.DB` close (flushes pending writes).
5. Process exits. Sessions remain valid → restart reconnects without QR.
