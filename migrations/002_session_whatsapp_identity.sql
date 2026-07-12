ALTER TABLE sessions ADD COLUMN IF NOT EXISTS wa_jid text;
CREATE INDEX IF NOT EXISTS sessions_wa_jid_idx ON sessions(wa_jid) WHERE wa_jid IS NOT NULL;
