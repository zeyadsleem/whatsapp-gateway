# Privacy Policy and Data Audit

This gateway is built for shops to connect their own WhatsApp number and send
transactional messages. It is **not** a CRM, helpdesk, inbox or chat viewer.

## What we store (application schema `app`)

| Data | Table / field | Why |
|---|---|---|
| Shop identifier and display name | `app.instances.shop_id`, `display_name` | Identify the tenant |
| Shop WhatsApp number and JID | `app.instances.phone_number`, `wa_jid` | Show the connected number; reconnect after restart |
| Connection status and timestamps | `app.instances.status`, `created_at`, `updated_at`, `last_connected_at` | Operational state |
| Webhook configuration | `app.instances.webhook_url`, `webhook_enabled` | Deliver events |
| Webhook secret (encrypted) | `app.instances.webhook_secret_enc` | HMAC-sign outgoing webhooks; AES-256-GCM at rest |
| API key (SHA-256 hash only) | `app.api_keys.key_hash`, `key_prefix` | Authenticate shop requests; plaintext shown once |
| Aggregate counters in memory | none (process memory) | Sent / failed / received totals for the dashboard |

There is deliberately **no** table for messages, chats, conversations, contacts,
media or message history, and there never will be. `scripts/privacy-audit.sh`
and `internal/database/database_test.go` (`TestPrivacySchemaAudit`) verify this
against `information_schema`, and
`internal/webhook/privacy_test.go` (`TestMessageEventDoesNotPersist`) proves
that dispatching an incoming message writes no rows at all.

## What we never store

- Message text, captions, media, or any message body — incoming or outgoing.
- Customer phone numbers in any log, table or metric label.
- Contact books, chat lists, conversation history.
- Message media: uploads/fetches are streamed into memory, sent, and discarded.
  No temporary files are written (multipart bodies are read through
  `MultipartReader`, never `ParseMultipartForm`).
- Webhook payloads: deliveries are in-memory only. If the process stops during
  a retry, that delivery is lost. This is intentional: losing a webhook is
  preferable to persisting message content.

## Incoming message path

```
WhatsApp servers → whatsmeow (decrypt in memory) → extract minimal fields
→ in-memory webhook queue → HMAC-signed POST to the shop → discard
```

The gateway never issues a database write on the incoming path.

## Outgoing message path

```
REST API → validate → whatsmeow SendMessage → response (message ID) → discard
```

The response to the shop contains the WhatsApp message ID and timestamp. The
message body is never logged and never written to disk.

## What whatsmeow stores internally (schema `whatsapp`)

WhatsApp Multi-Device requires persistent cryptographic state. Deleting it
would force a new QR pairing and can break Signal session reliability. The
following is stored *by the library*, in an isolated PostgreSQL schema that is:

1. never copied into `app.*`,
2. never exposed through any endpoint,
3. never used to build product features,
4. documented here instead of being hidden.

| whatsmeow table | Contents | Why it is required |
|---|---|---|
| `whatsmeow_device` | Noise key, identity key, signed prekey, registration ID, ADV secret, account signature, platform, push name | The device identity itself; without it the session cannot be restored |
| `whatsmeow_identity_keys` | Signal identity keys of peers | End-to-end encryption trust state |
| `whatsmeow_sessions` | Signal session records | End-to-end encryption |
| `whatsmeow_pre_keys`, `whatsmeow_sender_keys` | Prekeys and group sender keys | Session establishment and group decryption |
| `whatsmeow_app_state_sync_keys`, `whatsmeow_app_state_versions`, `whatsmeow_app_state_mutation_macs` | App-state sync keys, versions and mutation MACs | Required to process and validate app-state sync |
| `whatsmeow_lid_map` | LID ↔ phone-number mappings | Required to address users on WhatsApp's LID rollout |
| `whatsmeow_message_secrets` | Message secrets for recently received messages | Required to decrypt your own device echoes and respond to retries/receipts |
| `whatsmeow_retry_buffer` | Encrypted payloads awaiting retry receipts | Protocol retry handling for undecryptable messages |
| `whatsmeow_contacts`, `whatsmeow_chat_settings`, `whatsmeow_privacy_tokens`, `whatsmeow_nct_salt`, `whatsmeow_event_buffer` | Push/contact names, chat settings, privacy tokens, buffers | Protocol-required app-state and retry handling |

Notes:

- App-state sync (contact names, chat settings, muting, pinning) is part of the
  WhatsApp Multi-Device protocol. whatsmeow persists the entries required to
  keep the app-state hash consistent. Disabling the store would corrupt app
  state and break the connection, so it is kept **inside `whatsapp.*` only**.
  We do not read it, copy it, log it, or expose it.
- **History sync** (`events.HistorySync`) is received and immediately discarded
  (`internal/whatsapp/events.go`). Nothing from history sync is persisted in
  `app.*`. whatsmeow itself only decrypts/acks the notification.
- The gateway never calls `GetAllContacts`, message-search APIs, or similar.
  There is no code path that enumerates contacts for display or export.

## Logging

- Structured logs use fields such as `instance`, `event`, `result`,
  `duration_ms`; message bodies and full customer numbers are forbidden fields.
- The whatsmeow log adapter redacts QR payloads entirely and masks digit runs
  longer than six characters (`internal/logger/logger.go`).
- API keys, webhook secrets and session credentials are never logged.
- `LOG_LEVEL=debug` does not change these rules: the redaction applies at the
  adapter level for every level.

## Webhook signing

Shops can verify authenticity themselves:

```
X-Webhook-Signature: sha256=<hex( HMAC-SHA256( secret, timestamp + "." + rawBody ) )>
X-Webhook-Timestamp: <RFC3339>
X-Webhook-Event-ID:  <unique per event, stable across retries>
```

## Verifying this document

```bash
# Static + integration checks
TEST_DATABASE_URL=postgres://... go test ./...

# Live database inspection
./scripts/privacy-audit.sh
```

Both must pass before any release.
