# Product Requirements Document — WA Gateway Go

**Status:** Draft v1.0  
**Source of truth:** `https://github.com/niammuddin/wa-gateway`  
**Target implementation:** Go + whatsmeow

## 1. Ringkasan

WA Gateway Go adalah REST API multi-session WhatsApp gateway untuk sistem lain,
terutama billing/CRM, yang membutuhkan pengiriman pesan melalui akun WhatsApp
yang login dengan QR atau pairing code. Sistem menyimpan status pesan, retry,
delivery receipt, webhook, template, API key, dan audit trail.

Versi Go harus mempertahankan perilaku publik project Node.js, tetapi memisahkan
domain gateway dari adapter protokol WhatsApp sehingga library WhatsApp dapat
diganti tanpa mengubah API bisnis.

## 2. Tujuan

- Menyediakan API kompatibel dengan endpoint `/api/v1` pada project Node.js.
- Mendukung banyak session WhatsApp dalam satu deployment.
- Menjamin satu session hanya dimiliki satu client/worker pada satu waktu.
- Menyimpan pesan dan webhook secara durable.
- Mendukung retry, delay, priority, idempotency, dan throttling per session.
- Menyediakan observability untuk session, queue, database, Redis, dan webhook.
- Menghasilkan single binary Go yang mudah dideploy melalui Docker.

## 3. Non-goals

- Mengimplementasikan WhatsApp protocol sendiri.
- Menjamin bebas blokir dari WhatsApp.
- Menggantikan WhatsApp Business Cloud API untuk use case resmi.
- Menyediakan voice/video call atau broadcast list.
- Menjamin semua fitur Baileys tersedia pada release pertama.

`whatsmeow` berbicara dengan protokol WhatsApp Web Multi-Device dan bukan API
resmi Meta. Perubahan protokol, kebijakan, atau pembatasan akun harus dianggap
sebagai risiko produk.

## 4. Persona dan use case

### Admin

- Login ke admin API.
- Membuat, melihat, reconnect, disconnect, dan logout session.
- Melihat queue, message history, webhook delivery, dan monitoring.
- Mengelola API key, template, dan webhook.

### Sistem billing/CRM

- Mengirim pesan text/media melalui API key.
- Menggunakan template dan variable.
- Menyertakan `referenceId`, `sourceType`, dan `sourceId`.
- Menerima webhook sent/delivered/read/failed.
- Melakukan retry dengan aman menggunakan idempotency key.

## 5. Functional requirements

### Authentication and authorization

- Login username/password menghasilkan access JWT dan refresh cookie.
- Refresh token disimpan dalam bentuk hash atau token rotation yang aman.
- Endpoint integrasi menerima `X-Api-Key`.
- API key hanya ditampilkan penuh satu kali saat dibuat; database menyimpan
  SHA-256 hash dan prefix.
- API key dapat dibatasi ke satu session, IP exact/CIDR, rate limit, dan status aktif.
- Semua endpoint admin membutuhkan autentikasi.

### Session management

- Membuat session dengan method `qr` atau `pairing`.
- Menyimpan QR data URL dan expiry.
- Menyediakan status `disconnected`, `connecting`, `connected`,
  `reconnecting`, `logged_out`, dan `failed`.
- Reconnect otomatis dengan exponential backoff dan batas percobaan.
- Disconnect mempertahankan credential.
- Logout menghapus/menonaktifkan credential WhatsApp.
- Session yang sudah memiliki message history tidak boleh dihapus secara
  diam-diam; gunakan disconnect/logout atau prosedur archive.

### Message delivery

- Mendukung `text`, `image`, `document`, dan `pdf`.
- Semua message masuk ke durable queue.
- Mendukung `delay`, `priority`, `attempts`, dan exponential backoff.
- Mendukung template variable replacement.
- Mendukung idempotency per caller scope.
- Menyimpan status `queued`, `processing`, `sent`, `delivered`, `read`, `failed`.
- Menyimpan WhatsApp message ID dan timestamp receipt.
- Mengembalikan QR yang diterima dari adapter sebagai data yang dapat dirender
  oleh admin/API layer.
- Menyediakan list, detail, resend, dan delete dengan aturan status.

### Session throttling

Per session dapat dikonfigurasi:

- `minIntervalMs`
- `jitterMs`
- `maxMessagesPerMinute`
- `failureThreshold`
- `pauseDurationMs`

Ketika threshold tercapai, session masuk cooldown. Throttling adalah kontrol
operasional, bukan jaminan bahwa akun tidak terkena pembatasan WhatsApp.

### Webhook

- Webhook dapat subscribe ke event tertentu.
- Webhook dapat dibatasi ke session dan API key.
- Secret disimpan terenkripsi.
- Payload ditandatangani HMAC.
- Delivery disimpan sebelum dikirim.
- Delivery memiliki retry, status, response code, dan error.
- Delivery gagal dapat di-retry manual.

### Templates, queue, statistics, monitoring

- CRUD template dengan `{{variable}}`.
- Statistik message per status dan per session.
- Queue counts: waiting, active, completed, failed, delayed.
- Monitoring PostgreSQL, Redis, dan WhatsApp session.
- Dashboard summary untuk active sessions, queue size, message counts, dan
  recent failures.

## 6. Public API compatibility

Endpoint harus mempertahankan route Node.js berikut:

```text
POST   /api/v1/auth/login
GET    /api/v1/auth/session
POST   /api/v1/auth/refresh
POST   /api/v1/auth/logout
PUT    /api/v1/auth/password
GET    /api/v1/sessions
POST   /api/v1/sessions
GET    /api/v1/sessions/:id
DELETE /api/v1/sessions/:id
POST   /api/v1/sessions/:id/reconnect
POST   /api/v1/sessions/:id/disconnect
POST   /api/v1/sessions/:id/logout
PUT    /api/v1/sessions/:id/throttle
POST   /api/v1/messages/send
GET    /api/v1/messages
GET    /api/v1/messages/:id
POST   /api/v1/messages/:id/resend
DELETE /api/v1/messages/:id
GET    /api/v1/api-keys
POST   /api/v1/api-keys
PUT    /api/v1/api-keys/:id
DELETE /api/v1/api-keys/:id
GET    /api/v1/templates
POST   /api/v1/templates
GET    /api/v1/templates/:id
PUT    /api/v1/templates/:id
DELETE /api/v1/templates/:id
GET    /api/v1/webhooks
POST   /api/v1/webhooks
PUT    /api/v1/webhooks/:id
DELETE /api/v1/webhooks/:id
POST   /api/v1/webhooks/:id/test
GET    /api/v1/webhooks/deliveries
GET    /api/v1/webhooks/stats
POST   /api/v1/webhooks/deliveries/:id/retry
GET    /api/v1/queue
GET    /api/v1/stats
GET    /api/v1/monitoring
GET    /api/v1/dashboard
```

## 7. Non-functional requirements

- Graceful shutdown harus menghentikan HTTP, queue worker, WhatsApp clients,
  database, dan Redis secara berurutan.
- Semua log memiliki request ID dan session ID jika tersedia.
- Secret, API key, access token, dan WhatsApp credential tidak boleh masuk log.
- Database query memakai parameter binding.
- Semua state transition penting memiliki test unit/integration.
- Health endpoint membedakan liveness dan readiness.
- Deployment awal mendukung satu process owner untuk seluruh session.
- Deployment multi-worker hanya boleh diaktifkan setelah session lease/lock diuji.

## 8. Acceptance criteria MVP

- Satu session dapat login via QR, restart process, lalu reconnect tanpa login ulang.
- `POST /messages/send` menghasilkan message queued dan job durable.
- Worker mengirim text melalui adapter WhatsApp dan menyimpan WA message ID.
- Receipt sent/delivered/read mengubah status secara monotonic.
- Pesan dengan idempotency key yang sama tidak membuat duplikasi.
- Webhook signed terkirim dan delivery dapat di-retry.
- API key tidak pernah dikembalikan lagi setelah response pembuatan awal.
- Session berisi history tidak dapat dihapus oleh endpoint delete biasa.
- Docker deployment memiliki PostgreSQL, Redis, persistence, healthcheck, dan backup.

## 9. Risiko dan keputusan terbuka

- Stabilitas `whatsmeow` mengikuti perubahan WhatsApp Web.
- Perlu validasi kompatibilitas semua tipe media yang dipakai billing.
- Perlu keputusan apakah auth store whatsmeow dipusatkan di PostgreSQL atau
  database terpisah per session.
- Perlu benchmark jumlah session maksimum per process dan memory baseline.
- Perlu menentukan kapan arsitektur API/worker dipisah secara fisik.
