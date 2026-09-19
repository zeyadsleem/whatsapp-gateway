# Updating whatsmeow

The WhatsApp Web multi-device protocol changes over time. `go.mau.fi/whatsmeow`
is the only component that speaks it, and it is isolated behind
`internal/whatsapp`. Updating it is a deliberate, staged operation.

## Current pin

```
go.mau.fi/whatsmeow v0.0.0-20260917111002-2e338d0ee73d
```

Pinned by pseudo-version to a known-good commit, not `@latest`.

## Procedure

1. **Read the upstream changes.** Check whatsmeow's `CHANGELOG.md` and recent
   commits for API changes, protocol updates and breaking changes.

2. **Update the dependency.**

   ```bash
   go get go.mau.fi/whatsmeow@<new-version>
   go mod tidy
   go mod verify
   ```

3. **Compile and vet.**

   ```bash
   go build ./...
   go vet ./...
   ```

4. **Run the full test suite, including the race detector.**

   ```bash
   TEST_DATABASE_URL=postgres://... go test -race ./...
   ```

5. **Build the container and smoke-test one instance.**

   - `docker compose up -d --build`
   - Pair a dedicated test shop number (not a production number).
   - Verify: QR pairing, `connected` status, send text, receive a reply,
     webhook delivery, `restart` without a QR, `disconnect` + `connect`.
   - Restart the container and confirm automatic session restore.

6. **Roll out.** Update one production instance first, watch the logs
   (`event=state_change`, `event=reconnect`, `event=send_message`) for 24 hours,
   then update the rest.

7. **Record it.** Add an entry to `docs/decisions.md` with the version, date and
   any observed protocol issues.

## If something breaks

```bash
go get go.mau.fi/whatsmeow@v0.0.0-20260917111002-2e338d0ee73d
go mod tidy
go build ./... && TEST_DATABASE_URL=... go test ./...
docker compose up -d --build
```

Sessions are unaffected by library updates as long as the protocol version
stays accepted by WhatsApp. A QR re-pair is only needed if WhatsApp invalidates
the linked device itself.

## What to watch in the code

If whatsmeow changes these APIs, the changes are localized:

| API | Used in |
|---|---|
| `whatsmeow.NewClient`, `Connect/Disconnect/Logout` | `internal/whatsapp/client.go` |
| `GetQRChannel` and `QRChannelItem` events | `internal/whatsapp/client.go` |
| `AddEventHandler` event types | `internal/whatsapp/events.go` |
| `SendMessage`, `Upload`, `waE2E` message structs | `internal/whatsapp/send.go` |
| `sqlstore.NewWithDB`, `Container.Upgrade`, `GetDevice` | `cmd/server/main.go`, `internal/whatsapp/client.go` |
| Store interfaces (`msgsecret`, contacts, app state) | untouched by application code |
