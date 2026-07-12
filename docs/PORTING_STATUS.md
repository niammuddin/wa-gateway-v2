# Porting status

## Implemented in this initial pass

- PRD derived from the Node.js routes, services, migrations, and deployment model.
- ERD and PostgreSQL baseline migration.
- Go module and single-binary HTTP entrypoint.
- Graceful HTTP shutdown.
- Health endpoint.
- Session list/create/detail and lifecycle status endpoints.
- Message send/list contract with queued response.
- Protocol boundary interface for a one-client-per-session WhatsApp manager.
- Compiled `whatsmeow` adapter for connect, disconnect, logout, QR channel, and
  text sending behind the protocol boundary.
- PostgreSQL repository and automatic migration runner for the initial
  session/message slice.
- Docker Compose stack for PostgreSQL, Redis, and the gateway.
- Authenticated CRUD for templates, API keys, and webhooks.
- HMAC webhook delivery records and HTTP dispatch for `message.sent`.
- Message idempotency key, template variable replacement, resend/delete/detail,
  stats, queue, dashboard, and password-change endpoints.
- WhatsApp receipt mapping for sent message delivery/read status.
- QR-to-PNG data URL conversion, pairing-code path, media upload/send adapter,
  session history protection, throttle configuration, and minimal admin shell.
- Worker-side enforcement for per-session minimum interval and per-minute cap.
- Startup republish of durable `queue_jobs` waiting/retrying rows into Redis.
- Idempotency safety fix: requests without `Idempotency-Key` no longer share the
  empty-key hash, and concurrent duplicate-key races return the existing job.
- Message queue parity: send requests now accept `priority` and `delay` in
  milliseconds; priority maps to weighted Asynq queues and delay survives queue
  recovery through the persisted job payload.
- WhatsApp error handling: reach-out timelock `463` is normalized to an
  actionable failed-message error and is not retried by the queue worker.
- Media routing parity: worker dispatch is now driven by the declared message
  type; image/document/pdf jobs require a media URL and cannot silently fall
  through to text sending.
- Receipt parity: delivered/read webhook payloads and persisted timestamps now
  use the original WhatsApp receipt event timestamp instead of gateway time.

## Remaining verification limits

- A real WhatsApp account must be linked to verify end-to-end send/media and
  real receipt events; the adapter and lifecycle are wired, but no account is
  available for an automated test.
- The Admin SPA assets are now byte-identical to the Node.js source (`index`,
  CSS, Alpine runtime, app JS, and favicon) and are embedded in the Go binary;
  `/admin` also serves the required asset wildcard routes.
- Receipt delivered/read E2E verification still depends on the recipient device
  opening and reading the test message.
