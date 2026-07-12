ALTER TABLE messages ADD COLUMN IF NOT EXISTS idempotency_key_hash varchar(64);
ALTER TABLE messages ADD COLUMN IF NOT EXISTS idempotency_scope varchar(128);
ALTER TABLE messages ADD COLUMN IF NOT EXISTS idempotency_fingerprint varchar(64);
CREATE UNIQUE INDEX IF NOT EXISTS messages_idempotency_scope_key_unique ON messages(idempotency_scope,idempotency_key_hash) WHERE idempotency_scope IS NOT NULL AND idempotency_key_hash IS NOT NULL;
