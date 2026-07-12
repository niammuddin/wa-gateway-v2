# Porting Plan: Node.js to Go

## Mapping teknologi

| Node.js | Go | Catatan |
|---|---|---|
| Fastify | `net/http` + `chi` | Pertahankan route dan response JSON |
| Baileys | `go.mau.fi/whatsmeow` | Adapter wajib, jangan bocorkan tipe library ke domain |
| BullMQ | `asynq` + Redis | PostgreSQL tetap source of truth |
| Knex | `pgx` + `sqlc` | Query typed dan parameterized |
| bcrypt | Argon2id/bcrypt | Jangan downgrade password security |
| Pino | `log/slog` | JSON structured logging |
| Swagger | OpenAPI | Salin contract `docs/OPENAPI.yaml` lalu validasi |
| Vitest | Go `testing` | Unit, integration, contract test |

## Porting sequence

1. Copy and verify public API contract.
2. Create PostgreSQL migration and repository interfaces.
3. Implement auth and API key middleware.
4. Implement session manager and `WhatsAppClient` interface.
5. Integrate whatsmeow in a separate adapter package.
6. Implement message outbox and Redis worker.
7. Implement webhook delivery and retry.
8. Port templates, stats, dashboard, queue, and monitoring.
9. Port admin UI only after API contract is stable.
10. Run Node-vs-Go contract tests against the same PostgreSQL fixtures.

## Boundary rule

The domain layer may know `Session`, `Message`, and `Webhook`, but must not know
`whatsmeow.Client`, QR internals, or WhatsApp protocol nodes. This prevents a
future move to WhatsApp Cloud API from requiring a rewrite of the whole gateway.
