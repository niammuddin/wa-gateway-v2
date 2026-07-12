# Development workflow

## Recommended daily development

Run only PostgreSQL and Redis in Docker, and run the Go gateway locally for
fast code changes and direct logs.

```bash
cp .env.example .env
# Edit .env and replace all placeholders.
docker compose up -d postgres redis
set -a; source .env; set +a
go run ./cmd/gateway
```

When the gateway runs on the host, use `localhost` for database and Redis
hosts in `.env`. Open `http://localhost:3000/admin`.

## Full Docker validation

```bash
docker compose up -d --build
docker compose logs -f gateway
```

Inside Docker, use service names `postgres` and `redis`, not `localhost`.
Do not run the local and Docker gateway at the same time: both bind port 3000
and may compete for the same WhatsApp sessions.
