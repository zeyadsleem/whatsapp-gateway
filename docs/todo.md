# Todo

Known gaps and planned work. Nothing here is required for the current MVP.

## Near term

- [ ] Verify passkey-based pairing. WhatsApp recently began requesting passkeys
      during companion pairing; the MVP surfaces a clear error instead. Add
      WebAuthn support behind `PairPasskeyRequest` if WhatsApp enforces it.
- [ ] `message.delivery_update` webhook event from `events.Receipt` (opt-in per
      instance, aggregate status only, no content).
- [ ] Per-instance webhook event filtering (e.g. disable `instance.disconnected`).
- [ ] Admin UI: webhook delivery health summary (last success/failure timestamp
      per instance, no payloads).

## Later

- [ ] Optional `TRUSTED_PROXIES` configuration for `X-Forwarded-For` handling
      behind a reverse proxy.
- [ ] Audit log for admin actions (instance create/delete/logout) with actor IP.
- [ ] Instance export/import of non-secret configuration for disaster recovery.
- [ ] Benchmarks for 100 concurrent connected instances (memory per client,
      send throughput).

## Explicitly out of scope

CRM, helpdesk, inbox, chat viewer, contact manager, campaign manager, broadcast
marketing, AI/chatbots, group management UI, calls, statuses, microservices,
Kubernetes manifests.
