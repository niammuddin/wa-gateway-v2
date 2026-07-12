# Production readiness

## Required environment

- Set a long random `JWT_SECRET` and a different `JWT_REFRESH_SECRET`.
- Set a long random `WEBHOOK_ENCRYPTION_KEY`.
- Set non-default `ADMIN_USERNAME` and `ADMIN_PASSWORD`.
- Use a persistent PostgreSQL database and Redis volume.
- Mount `/app/auth_sessions` persistently; WhatsApp sessions depend on it.
- Put the gateway behind TLS and restrict PostgreSQL/Redis ports from public access.

## Operational checks

- `GET /health` returns `200`.
- Session status is `connected` before sending.
- Contacts that have never chatted with the account may trigger WhatsApp error
  `463`; the recipient must initiate or establish the conversation first.
- Monitor `messages`, `queue_jobs`, and `webhook_deliveries` for failures.
- Configure webhook secrets and verify `X-WA-Signature` on the receiver.
- Back up PostgreSQL and the WhatsApp session volume together.

## Current verification boundary

- Text, image, and document sending were verified with a linked session.
- `delivered` and `read` receipts require the recipient to open/read the
  message; the gateway now persists and forwards the original receipt timestamp.
- The admin page is operationally complete for dashboard, sessions, messages,
  media, webhooks, API keys, and templates, but is not pixel-identical to the
  Node.js SPA.
