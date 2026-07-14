# Dokumentasi Integrasi Billing

Dokumen ini adalah panduan integrasi antara sistem billing/CRM dan WA Gateway
Go. Gateway mengelola session WhatsApp, queue, pengiriman pesan, dan receipt.
Billing tetap menjadi sumber data pelanggan, invoice, pembayaran, dan aturan
bisnis.

## 1. Batas tanggung jawab

### Sistem billing

- Menentukan kapan notifikasi harus dikirim.
- Menyediakan nomor tujuan, isi pesan, template, dan metadata transaksi.
- Mengirim `Idempotency-Key` yang stabil untuk setiap notifikasi bisnis.
- Menerima webhook dan memperbarui status notifikasi/invoice.
- Menyimpan relasi internal antara invoice/customer dan `referenceId`.

### WA Gateway

- Mengautentikasi pemanggil melalui API key.
- Memvalidasi session dan payload.
- Menyimpan pesan secara durable.
- Memproses pengiriman melalui queue dan whatsmeow.
- Menerima receipt WhatsApp: sent, delivered, dan read.
- Mengirim webhook yang ditandatangani ke billing.
- Menyimpan histori delivery dan error operasional.

Gateway bukan sistem billing dan tidak menghitung tagihan, saldo pelanggan,
jatuh tempo, denda, atau status pembayaran.

## 2. Alur integrasi

```text
Billing
  │ POST /api/v1/messages/send + X-Api-Key
  ▼
WA Gateway API
  │ validasi → idempotency → simpan queued → Redis/Asynq
  ▼
WhatsApp session
  ├── message.sent
  ├── message.delivered
  ├── message.read
  └── message.failed
          │
          ▼
       Webhook billing
```

Response `202 Accepted` berarti pesan sudah diterima gateway dan masuk queue;
itu belum berarti pesan sudah terkirim ke WhatsApp.

## 3. Persiapan keamanan

Buat API key khusus untuk aplikasi billing. Jangan gunakan access token admin
untuk integrasi server-to-server.

Rekomendasi:

- Satu API key untuk satu aplikasi atau tenant.
- Batasi API key ke session melalui `sessionId` jika hanya boleh memakai satu
  nomor WhatsApp.
- Gunakan `allowedIps` dan `rateLimit` sesuai kebutuhan deployment.
- Simpan API key hanya di secret manager atau environment variable billing.
- API key plaintext hanya muncul sekali ketika dibuat.
- Gunakan HTTPS untuk API dan endpoint webhook.
- Jangan menaruh API key, webhook secret, atau isi pesan sensitif di log.

Header integrasi:

```http
X-Api-Key: wg_live_xxxxxxxxxxxxxxxxx
Content-Type: application/json
```

## 4. Membuat API key

Endpoint ini membutuhkan autentikasi admin, bukan API key billing.

```bash
curl -X POST https://wa.example.com/api/v1/api-keys \
  -H "Authorization: Bearer $ADMIN_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "billing-production",
    "sessionId": "billing-main",
    "rateLimit": 60,
    "allowedIps": ["10.20.0.15"],
    "isActive": true
  }'
```

Simpan nilai key dari response saat itu juga. Database hanya menyimpan hash dan
prefix key sehingga key lama tidak dapat dilihat kembali dari admin.

## 5. Mengirim pesan text

```bash
curl -X POST https://wa.example.com/api/v1/messages/send \
  -H "X-Api-Key: $WA_GATEWAY_API_KEY" \
  -H "Idempotency-Key: invoice-INV-20260714-000123-reminder-1" \
  -H "Content-Type: application/json" \
  -d '{
    "sessionId": "billing-main",
    "to": "628123456789",
    "type": "text",
    "message": "Tagihan internet Anda sebesar Rp150.000 jatuh tempo pada 20 Juli 2026.",
    "referenceId": "INV-20260714-000123",
    "sourceType": "invoice",
    "sourceId": "000123",
    "priority": 5,
    "delay": 0
  }'
```

Response normal:

```json
{
  "messageId": "uuid-message",
  "jobId": "uuid-job",
  "status": "queued",
  "message": "Message queued for delivery"
}
```

`referenceId` adalah identifier bisnis yang dibaca kembali oleh billing pada
webhook. `sourceType` dan `sourceId` dapat membedakan invoice, payment,
customer, atau campaign.

## 6. Idempotency dan retry

Billing harus membuat satu key yang stabil untuk satu tindakan bisnis. Jangan
menggunakan UUID baru setiap kali request di-retry.

Contoh: `invoice-INV-20260714-000123-reminder-1`.

Jika request yang sama diterima kembali dengan key dan API key yang sama,
gateway mengembalikan message yang sudah ada dan tidak membuat pesan baru.

- Timeout jaringan: kirim ulang dengan `Idempotency-Key` yang sama.
- Response `200` dengan `Message already queued`: anggap sudah diterima.
- Response `202`: simpan `messageId` dan `jobId`.
- Response `400`: perbaiki payload; jangan retry otomatis.
- Response `401`/`403`: perbaiki credential atau scope API key.
- Response `404`: pastikan session atau template masih ada.
- Response `429`/`503`: retry dengan exponential backoff.
- Jangan retry tanpa batas; gunakan dead-letter atau alert di billing.

Idempotency bersifat per scope principal. API key berbeda tidak dianggap caller
yang sama.

## 7. Template billing

Buat template melalui admin API:

```bash
curl -X POST https://wa.example.com/api/v1/templates \
  -H "Authorization: Bearer $ADMIN_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"invoice-reminder","body":"Halo {{name}}, tagihan {{invoiceNumber}} sebesar {{amount}} jatuh tempo {{dueDate}}."}'
```

Kirim menggunakan `templateId` dan `variables`:

```json
{
  "sessionId": "billing-main",
  "to": "628123456789",
  "type": "text",
  "templateId": "template-uuid",
  "variables": {
    "name": "Budi",
    "invoiceNumber": "INV-20260714-000123",
    "amount": "Rp150.000",
    "dueDate": "20 Juli 2026"
  },
  "referenceId": "INV-20260714-000123",
  "sourceType": "invoice",
  "sourceId": "000123"
}
```

Template diselesaikan saat request masuk. Untuk audit, billing tetap sebaiknya
menyimpan snapshot isi pesan yang dikirim.

## 8. Media dan dokumen

Gateway saat ini menerima `image`, `document`, dan `pdf` melalui URL yang dapat
diakses worker gateway.

```json
{
  "sessionId": "billing-main",
  "to": "628123456789",
  "type": "pdf",
  "message": "Berikut invoice Anda.",
  "url": "https://billing.example.com/private/invoices/INV-000123.pdf",
  "filename": "INV-000123.pdf",
  "mimeType": "application/pdf",
  "referenceId": "INV-000123",
  "sourceType": "invoice",
  "sourceId": "000123"
}
```

URL harus dapat di-fetch oleh gateway, bukan hanya oleh browser user. Gunakan
URL HTTPS signed yang berumur pendek untuk dokumen privat. Jangan menaruh
credential permanen di query string. URL expired akan menghasilkan
`message.failed`.

## 9. Webhook status pesan

Buat webhook dengan secret acak minimal 32 byte:

```bash
curl -X POST https://wa.example.com/api/v1/webhooks \
  -H "Authorization: Bearer $ADMIN_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://billing.example.com/integrations/wa-gateway/webhook",
    "events": ["message.sent", "message.delivered", "message.read", "message.failed"],
    "sessionIds": ["billing-main"],
    "secret": "replace-with-a-random-secret",
    "isActive": true
  }'
```

Header webhook:

```http
X-WA-Event: message.delivered
X-WA-Signature: 64-hex-character-hmac-sha256
Content-Type: application/json
```

Signature: `hex(HMAC-SHA256(raw_request_body, webhook_secret))`.
Validasi menggunakan raw body sebelum JSON diformat ulang dan gunakan
perbandingan constant-time.

Payload `message.sent`:

```json
{"messageId":"uuid-message","sessionId":"billing-main","to":"628123456789","waMessageId":"3EB0F0EA95353257D26593"}
```

Payload `message.delivered` atau `message.read`:

```json
{
  "messageId": "uuid-message",
  "referenceId": "INV-000123",
  "sessionId": "billing-main",
  "sourceId": "000123",
  "sourceType": "invoice",
  "timestamp": "2026-07-14T10:49:10Z",
  "to": "628123456789",
  "waMessageId": "3EB0F0EA95353257D26593"
}
```

Payload `message.failed`:

```json
{"messageId":"uuid-message","sessionId":"billing-main","to":"628123456789","error":"session client unavailable"}
```

Status di billing harus monotonic: `queued → sent → delivered → read`, atau
`queued → failed`. Event yang sama dapat diterima lebih dari sekali; handler
wajib idempotent menggunakan `messageId` atau `waMessageId` sebagai key unik.

## 10. Delivery history dan response webhook

Endpoint webhook billing harus mengembalikan HTTP `2xx` setelah payload
berhasil diverifikasi dan disimpan. Non-2xx atau timeout membuat delivery gagal
dan dapat di-retry melalui:

```text
POST /api/v1/webhooks/deliveries/:id/retry
GET  /api/v1/webhooks/deliveries
GET  /api/v1/webhooks/stats
```

Simpan `deliveryId`, attempts, response status, dan error untuk operasional.
Delivery history adalah histori webhook, bukan histori pesan WhatsApp.

## 11. Model data di billing

Billing sebaiknya memiliki tabel notifikasi sendiri:

```text
wa_notifications
- id
- invoice_id / customer_id
- gateway_message_id UNIQUE
- gateway_wa_message_id
- idempotency_key UNIQUE
- event_type
- status
- last_error
- sent_at / delivered_at / read_at
- created_at / updated_at
```

Alur transaksi:

1. Buat notification record dan idempotency key.
2. Kirim request ke gateway.
3. Simpan `messageId` dan status `queued`.
4. Verifikasi webhook lalu update dalam transaksi database.
5. Abaikan event duplikat atau event yang lebih rendah dari status sekarang.
6. Lakukan rekonsiliasi berkala dari endpoint message history.

## 12. Kegagalan umum

| Kondisi | Arti | Tindakan |
|---|---|---|
| `queued` lama | Worker/session belum mengirim | Periksa queue dan session |
| `failed` | Pengiriman gagal setelah proses | Simpan error dan alert |
| Media gagal | URL tidak dapat diakses/expired | Buat URL baru lalu resend |
| `429` | Rate limit API/WhatsApp | Backoff, jangan spam retry |
| Session disconnected | Nomor tidak siap mengirim | Tunda notifikasi |
| Webhook timeout | Billing tidak merespons | Perbaiki endpoint dan retry |
| Duplicate event | Retry jaringan/delivery | Handler idempotent |

## 13. Checklist production

- [ ] API key billing dibatasi ke session yang benar.
- [ ] API key dan webhook secret disimpan di secret manager.
- [ ] API dan webhook menggunakan HTTPS.
- [ ] Billing memverifikasi `X-WA-Signature`.
- [ ] Billing memakai `Idempotency-Key` stabil.
- [ ] Handler webhook idempotent dan status monotonic.
- [ ] Billing menyimpan `messageId`, `waMessageId`, dan `referenceId`.
- [ ] Retry menggunakan backoff dan batas percobaan.
- [ ] Ada alert untuk message failed, queue menumpuk, dan webhook failed.
- [ ] Backup PostgreSQL dan credential WhatsApp sudah diuji.
- [ ] Tidak ada secret di source code, Docker image, atau log.

## 14. Status implementasi

Sudah tersedia di gateway:

- API key authentication dan session restriction.
- Text, image, document, dan PDF URL delivery.
- Template variable replacement.
- Durable queue dengan priority, delay, retry, dan throttling.
- Idempotency berdasarkan principal dan `Idempotency-Key`.
- Message status dan WhatsApp receipt.
- Webhook HMAC SHA-256, filter event/session, delivery history, dan retry.

Di luar scope gateway dan harus dibuat di aplikasi billing:

- Invoice, customer, payment, subscription, dan accounting.
- Tenant isolation billing.
- Quota/saldo pesan berdasarkan paket.
- Approval dan jadwal reminder bisnis.
- Rekonsiliasi invoice dengan payment gateway.

Untuk endpoint lengkap, lihat [docs/OPENAPI.yaml](OPENAPI.yaml), dan untuk
model database lihat [docs/ERD.md](ERD.md).
