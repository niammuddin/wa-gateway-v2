# WA Gateway Go

Port Go dari `https://github.com/niammuddin/wa-gateway`.

Dokumen desain:

- [PRD](docs/PRD.md)
- [ERD](docs/ERD.md)
- [Porting plan](docs/PORTING_PLAN.md)
- [Porting status](docs/PORTING_STATUS.md)
- [OpenAPI](docs/OPENAPI.yaml)

Status saat ini: core API, PostgreSQL migration, Redis/Asynq worker, auth, API
keys, templates, webhooks, session lifecycle, QR/pairing, text/media sending,
receipts, admin shell, and OpenAPI contract are implemented. E2E image and
document sends have been verified with a linked WhatsApp session. Receipt
`delivered/read` still depends on the recipient device generating the event.

Production deployment checklist tersedia di
[`docs/PRODUCTION_READINESS.md`](docs/PRODUCTION_READINESS.md).

Jalankan setelah Go tersedia:

```bash
go run ./cmd/gateway
curl http://localhost:3000/health
```

Dengan Docker:

```bash
docker compose up -d postgres redis
DATABASE_URL='postgresql://wagateway:wagateway@localhost:5432/wagateway?sslmode=disable' go run ./cmd/gateway
```

Admin shell tersedia di `http://localhost:3000/admin` saat service berjalan.
