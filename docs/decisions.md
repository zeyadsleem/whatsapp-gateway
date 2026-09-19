# Decisions

Architecture decision records for this project. Newest first.

## ADR-011 — In-memory webhook queue, no persistence

**Status:** accepted

Webhook deliveries live in a bounded in-memory queue with retries. If the
process stops during a retry, the event is lost. Persisting deliveries would
require storing message content in a queue table, which violates the privacy
requirements. Shops can recover critical state by querying the gateway or by
using their own idempotent processing against `message_id`.

## ADR-010 — Two PostgreSQL schemas, two pools

**Status:** accepted

`app.*` is owned by the application; `whatsapp.*` is owned by whatsmeow. Each
has its own connection pool with a different `search_path`, so unqualified
whatsmeow queries can never touch application tables and vice versa. Protocol
state that Multi-Device requires is physically isolated and documents in
`PRIVACY.md` rather than deleted.

## ADR-009 — Per-instance API keys, tenant resolved from the key

**Status:** accepted

Shop API endpoints never accept an instance identifier. The bearer token is
hashed and resolved to exactly one instance. This makes cross-tenant access
impossible by construction, not by validation. Unknown JSON fields are rejected
so a client cannot smuggle `instance_id`.

## ADR-008 — One whatsmeow device per instance, JID stored after pairing

**Status:** accepted

The application instance UUID is the primary key. After pairing we persist the
WhatsApp JID and phone number on the instance row for display and for loading
the device at startup (`sqlstore.Container.GetDevice`). The phone number is
never a primary key.

## ADR-007 — Pairing window with QR regeneration

**Status:** accepted

`GetQRChannel` emits a fixed batch of QR codes and closes with `timeout` when
they expire. The pairing loop detects this, recreates the channel and continues,
bounded to a 5-minute window and 30 rotations. Without this, the admin screen
would show a dead QR after ~60 seconds.

## ADR-006 — Exponential backoff with jitter, no reconnect storms

**Status:** accepted

Reconnects start at 5s, double to a 5-minute cap, with ±20% jitter. Permanent
conditions (logged out, client outdated, stream replaced, temporary ban) stop
the loop instead of hammering WhatsApp servers. 50 instances reconnecting after
an outage spread out instead of synchronizing.

## ADR-005 — History sync discarded

**Status:** accepted

`events.HistorySync` payloads are received (protocol acknowledgement) and
immediately dropped. Nothing from history sync is written to `app.*`. This is
enforced by code review, the privacy integration test, and documented in
`PRIVACY.md`.

## ADR-004 — Media in memory, no temp files

**Status:** accepted

Multipart uploads are read through `MultipartReader` with hard size limits and
never touch disk. URL fetches stream into memory with the same limit. Media is
sent to WhatsApp and discarded. This avoids orphaned temporary files and disk
forensics exposure.

## ADR-003 — SSRF-safe HTTP client for webhooks and media fetches

**Status:** accepted

Both webhook delivery and media URL fetching use a client that validates URLs
at configuration time (scheme, local hostnames, literal private IPs) and again
at dial time after DNS resolution, with redirect re-validation. This blocks
DNS rebinding and cloud metadata endpoints.

## ADR-002 — gin + pgx + log/slog

**Status:** accepted

Gin is a small, stable HTTP framework with a mature middleware ecosystem. pgx is
the reference PostgreSQL driver for Go with native pooling. `log/slog` from the
standard library avoids a logging dependency and integrates with the sanitizing
adapter for whatsmeow logs. No ORM: parameterized SQL keeps queries explicit and
auditable.

## ADR-001 — Modular monolith, no queue infrastructure

**Status:** accepted

One Go binary serves the API, admin dashboard, metrics and WhatsApp clients.
PostgreSQL is the only external dependency. Redis, RabbitMQ, Kafka, NATS and
MinIO are deliberately excluded: the workload (transactional notifications for
tens of shops) does not justify them, and the privacy model forbids persisting
message payloads in any of them.
