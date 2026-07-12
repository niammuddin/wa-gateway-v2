# Entity Relationship Diagram — WA Gateway Go

ERD ini mempertahankan schema final project Node.js, termasuk migration untuk
hash API key, idempotency, webhook delivery, scope, encrypted secret,
throttling, dan preservasi history session.

```mermaid
erDiagram
    USERS ||--o{ AUDIT_LOGS : creates
    API_KEYS ||--o{ WEBHOOKS : scopes
    SESSIONS ||--o{ MESSAGES : owns
    TEMPLATES ||--o{ MESSAGES : resolves
    WEBHOOKS ||--o{ WEBHOOK_DELIVERIES : produces
    MESSAGES ||--o{ WEBHOOK_DELIVERIES : references
    SESSIONS ||--o{ QUEUE_JOBS : processes

    USERS {
      uuid id PK
      string username UK
      string password_hash
      string refresh_token_hash
      timestamp created_at
      timestamp updated_at
    }
    SESSIONS {
      uuid id PK
      string session_id UK
      string status
      string method
      string phone_number
      text qr_code
      timestamp qr_expires_at
      string pairing_code
      string wa_jid
      int min_interval_ms
      int jitter_ms
      int max_messages_per_minute
      int failure_threshold
      int pause_duration_ms
      int failure_count
      timestamp failure_window_started_at
      timestamp throttled_until
      timestamp created_at
      timestamp updated_at
    }
    API_KEYS {
      uuid id PK
      string key_hash UK
      string key_prefix
      string name
      text[] allowed_ips
      int rate_limit
      string session_id FK
      boolean is_active
      timestamp created_at
      timestamp updated_at
    }
    MESSAGES {
      uuid id PK
      string job_id
      string session_id FK
      string to
      string type
      text content
      text url
      string filename
      string mime_type
      uuid template_id FK
      jsonb variables
      string reference_id
      string source_type
      string source_id
      string idempotency_scope
      string idempotency_key_hash
      string idempotency_fingerprint
      string status
      text error
      string wa_message_id
      timestamp queued_at
      timestamp sent_at
      timestamp delivered_at
      timestamp read_at
      timestamp created_at
      timestamp updated_at
    }
    TEMPLATES {
      uuid id PK
      string name UK
      text body
      timestamp created_at
      timestamp updated_at
    }
    WEBHOOKS {
      uuid id PK
      string url
      text[] events
      text secret_encrypted
      text[] session_ids
      uuid api_key_id FK
      boolean is_active
      timestamp created_at
      timestamp updated_at
    }
    WEBHOOK_DELIVERIES {
      uuid id PK
      uuid webhook_id FK
      uuid event_id
      string event
      jsonb payload
      string status
      int attempts
      int max_attempts
      int response_status
      text error
      timestamp queued_at
      timestamp delivered_at
      timestamp updated_at
      timestamp created_at
    }
    QUEUE_JOBS {
      uuid id PK
      string queue_name
      string job_id UK
      string name
      jsonb data
      string status
      int priority
      int attempts
      int max_attempts
      text error
      timestamp processed_at
      timestamp completed_at
      timestamp created_at
    }
    AUDIT_LOGS {
      uuid id PK
      string action
      string entity_type
      string entity_id
      uuid user_id FK
      jsonb metadata
      string ip_address
      timestamp created_at
    }
```

## Integrity rules

1. `messages.session_id` references `sessions.session_id` with `ON DELETE
   RESTRICT`; message history tidak boleh hilang karena delete session.
2. `messages(idempotency_scope, idempotency_key_hash)` unique hanya jika kedua
   kolom tidak null.
3. `api_keys.key_hash` unique; kolom plaintext key tidak boleh ada.
4. Secret webhook disimpan encrypted; response API tidak mengembalikan secret.
5. `messages.status` hanya boleh bergerak maju secara monotonic, kecuali
   resend membuat job baru atau mengembalikan message yang gagal ke queued.
6. `webhook_deliveries` immutable pada payload/event; retry hanya mengubah status,
   attempts, response, dan error.
7. `queue_jobs.data` menyimpan snapshot job untuk audit, sementara Redis/asynq
   menjadi executor.

## WhatsApp auth store

`whatsmeow` memiliki device/auth store sendiri. Store ini adalah bagian dari
credential session dan tidak boleh dicampur dengan tabel `messages`. Untuk MVP,
gunakan PostgreSQL store yang dikelola `whatsmeow`; jika versi library yang
dipilih membutuhkan schema khusus, letakkan tabelnya dalam schema/database
terpisah dan jangan melakukan migration manual terhadap tabel internalnya.
