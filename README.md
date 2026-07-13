# WA Gateway Go

Daily development workflow: [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

Port Go dari [github.com/niammuddin/wa-gateway](https://github.com/niammuddin/wa-gateway).

Dokumen desain:

- [PRD](docs/PRD.md)
- [ERD](docs/ERD.md)
- [Porting plan](docs/PORTING_PLAN.md)
- [Porting status](docs/PORTING_STATUS.md)
- [OpenAPI](docs/OPENAPI.yaml)
- [Production deployment](docs/DEPLOYMENT.md)

Status saat ini: core API, PostgreSQL migration, Redis/Asynq worker, auth, API
keys, templates, webhooks, session lifecycle, QR/pairing, text/media sending,
receipts, admin shell, and OpenAPI contract are implemented. E2E image and
document sends have been verified with a linked WhatsApp session. Receipt
`delivered/read` still depends on the recipient device generating the event.

Production deployment guide tersedia di
[`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md), dengan checklist tambahan di
[`docs/PRODUCTION_READINESS.md`](docs/PRODUCTION_READINESS.md).

Jalankan setelah Go tersedia:

```bash
go run ./cmd/gateway
```

Buka `http://localhost:3000/` untuk landing page atau `http://localhost:3000/admin`
untuk admin panel.

Dengan Docker:

```bash
docker compose up -d postgres redis
DATABASE_URL='postgresql://wagateway:wagateway@localhost:5432/wagateway?sslmode=disable' go run ./cmd/gateway
```

Admin shell tersedia di `http://localhost:3000/admin` saat service berjalan.
