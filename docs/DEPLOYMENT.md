# Production deployment

This guide deploys the gateway, PostgreSQL, Redis, and persistent WhatsApp
session storage with Docker Compose. Put HTTPS in front of the gateway with a
reverse proxy or a managed load balancer.

## 1. Prepare the server

Install Docker Engine and the Compose plugin. Create a dedicated directory and
copy the repository into it. Do not expose PostgreSQL or Redis ports publicly.

```bash
mkdir -p /opt/wa-gateway-v2
cd /opt/wa-gateway-v2
```

## 2. Configure secrets

```bash
cp .env.production.example .env.production
chmod 600 .env.production
```

Replace every placeholder. Generate secrets with a password manager or:

```bash
openssl rand -hex 32
```

Use different values for `JWT_SECRET`, `JWT_REFRESH_SECRET`, and
`WEBHOOK_ENCRYPTION_KEY`. Never commit `.env.production`.

## 3. Start production

```bash
docker compose --env-file .env.production -f docker-compose.production.yml up -d --build
docker compose --env-file .env.production -f docker-compose.production.yml ps
curl -fsS http://127.0.0.1:3000/health
```

The gateway runs database migrations on startup. The first WhatsApp QR or
pairing login must be completed through `/admin`; the `auth_sessions` volume
must remain attached across upgrades and restarts.

## 4. HTTPS and proxy

Expose only the gateway port through the reverse proxy. Forward HTTPS traffic
to `127.0.0.1:3000`, preserve the `Host` header, and allow long-lived
connections for `/api/v1/events` (SSE). Do not expose ports 5432 or 6379 to the
internet.

## 5. Upgrade and rollback

```bash
git pull --ff-only
docker compose --env-file .env.production -f docker-compose.production.yml up -d --build
docker compose --env-file .env.production -f docker-compose.production.yml ps
```

Before an upgrade, keep a known-good image tag or Git commit. To roll back,
checkout that commit and run the same `up -d --build` command. Do not delete
volumes during an upgrade.

## 6. Backups

Back up PostgreSQL and the WhatsApp session volume together. A database dump
without the session volume can leave the database and linked devices out of
sync.

```bash
docker compose --env-file .env.production -f docker-compose.production.yml exec -T postgres \
  sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' > backup.sql
docker run --rm -v wa-gateway-v2_auth_sessions:/data -v "$PWD":/backup alpine \
  tar czf /backup/auth_sessions.tar.gz -C /data .
```

Store backups outside the server and periodically test restoration.

## 7. Operations checklist

- Confirm `/health` returns `{"status":"ok"}`.
- Confirm `/api/v1/monitoring` reports PostgreSQL and Redis as `up`.
- Complete a QR/pairing login and verify it survives a gateway restart.
- Send a test message and verify sent, delivered, read, and failed paths.
- Verify webhook signatures using `X-WA-Signature`.
- Monitor `queue_jobs`, `messages`, `webhook_deliveries`, and container logs.
- Rotate admin and JWT secrets through a planned maintenance window.
- Restrict SSH, Docker, and reverse-proxy administration to trusted operators.

## Logs and shutdown

```bash
docker compose --env-file .env.production -f docker-compose.production.yml logs -f gateway
docker compose --env-file .env.production -f docker-compose.production.yml down
```

Do not use `down -v` unless you intentionally want to destroy PostgreSQL,
Redis, and WhatsApp session data.
