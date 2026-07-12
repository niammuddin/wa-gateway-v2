package store

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"
)

var ErrNotFound = errors.New("not found")

func queueName(priority int) string {
	switch {
	case priority > 0:
		return "messages_high"
	case priority < 0:
		return "messages_low"
	default:
		return "messages"
	}
}

type Session struct {
	ID           string     `json:"id"`
	SessionID    string     `json:"session_id"`
	Status       string     `json:"status"`
	Method       string     `json:"method,omitempty"`
	PhoneNumber  string     `json:"phone_number,omitempty"`
	QRCode       string     `json:"qr_code,omitempty"`
	PairingCode  string     `json:"pairing_code,omitempty"`
	QRExpiresAt  *time.Time `json:"qr_expires_at,omitempty"`
	WAJID        string     `json:"wa_jid,omitempty"`
	MessageCount int        `json:"message_count"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type Message struct {
	ID                     string    `json:"id"`
	JobID                  string    `json:"job_id,omitempty"`
	SessionID              string    `json:"session_id"`
	To                     string    `json:"to"`
	Type                   string    `json:"type"`
	Content                string    `json:"content,omitempty"`
	Status                 string    `json:"status"`
	WAID                   string    `json:"wa_message_id,omitempty"`
	Error                  string    `json:"error,omitempty"`
	ReferenceID            string    `json:"reference_id,omitempty"`
	SourceType             string    `json:"source_type,omitempty"`
	SourceID               string    `json:"source_id,omitempty"`
	TemplateID             string    `json:"template_id,omitempty"`
	URL                    string    `json:"url,omitempty"`
	Filename               string    `json:"filename,omitempty"`
	MIMEType               string    `json:"mime_type,omitempty"`
	Priority               int       `json:"priority,omitempty"`
	Delay                  int       `json:"delay,omitempty"`
	IdempotencyScope       string    `json:"-"`
	IdempotencyKeyHash     string    `json:"-"`
	IdempotencyFingerprint string    `json:"-"`
	CreatedAt              time.Time `json:"created_at"`
}
type PendingJob struct {
	JobID string
	Data  []byte
}

type Memory struct {
	mu       sync.RWMutex
	sessions map[string]Session
	messages map[string]Message
}

type Store interface {
	ListSessions(context.Context) ([]Session, error)
	GetSession(context.Context, string) (Session, bool, error)
	PutSession(context.Context, Session) error
	PutMessage(context.Context, Message) error
	ListMessages(context.Context) ([]Message, error)
	UpdateSession(context.Context, string, string, string, string, string) error
	SetSessionQRExpiry(context.Context, string, *time.Time) error
	UpdateMessageStatus(context.Context, string, string, string, string) error
	SetSessionIdentity(context.Context, string, string, string) error
	UpdateMessageReceipt(context.Context, string, string, string, time.Time) error
	UpdateQueueJob(context.Context, string, string, string) error
	ListPendingQueueJobs(context.Context) ([]PendingJob, error)
	GetMessageByWAID(context.Context, string, string) (Message, bool, error)
}

func NewMemory() *Memory {
	return &Memory{sessions: map[string]Session{}, messages: map[string]Message{}}
}

func (m *Memory) ListSessions(_ context.Context) ([]Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Session, 0, len(m.sessions))
	for _, item := range m.sessions {
		item.ID = item.SessionID
		item.MessageCount = 0
		for _, message := range m.messages {
			if message.SessionID == item.SessionID {
				item.MessageCount++
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func (m *Memory) GetSession(_ context.Context, id string) (Session, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.sessions[id]
	v.ID = v.SessionID
	return v, ok, nil
}

func (m *Memory) PutSession(_ context.Context, v Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[v.SessionID] = v
	return nil
}

func (m *Memory) PutMessage(_ context.Context, v Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[v.ID] = v
	return nil
}

func (m *Memory) ListMessages(_ context.Context) ([]Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Message, 0, len(m.messages))
	for _, item := range m.messages {
		out = append(out, item)
	}
	return out, nil
}

func (m *Memory) UpdateSession(_ context.Context, id, status, qrCode, pairingCode, phone string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	v.Status = status
	v.QRCode = qrCode
	v.PairingCode = pairingCode
	v.PhoneNumber = phone
	v.UpdatedAt = time.Now().UTC()
	if qrCode == "" {
		v.QRExpiresAt = nil
	}
	m.sessions[id] = v
	return nil
}
func (m *Memory) SetSessionQRExpiry(_ context.Context, id string, expiry *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	v.QRExpiresAt = expiry
	m.sessions[id] = v
	return nil
}

func (m *Memory) UpdateMessageStatus(_ context.Context, id, status, waID, messageError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.messages[id]
	if !ok {
		return ErrNotFound
	}
	v.Status = status
	v.WAID = waID
	v.Error = messageError
	m.messages[id] = v
	return nil
}

func (m *Memory) SetSessionIdentity(_ context.Context, id, jid, phone string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	v.WAJID = jid
	v.PhoneNumber = phone
	v.UpdatedAt = time.Now().UTC()
	m.sessions[id] = v
	return nil
}

func (m *Memory) UpdateMessageReceipt(_ context.Context, sessionID, waID, status string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, v := range m.messages {
		if v.SessionID == sessionID && v.WAID == waID {
			v.Status = status
			m.messages[id] = v
			return nil
		}
	}
	return ErrNotFound
}
func (m *Memory) UpdateQueueJob(context.Context, string, string, string) error { return nil }
func (m *Memory) ListPendingQueueJobs(context.Context) ([]PendingJob, error)   { return nil, nil }
func (m *Memory) GetMessageByWAID(_ context.Context, sessionID, waID string) (Message, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, v := range m.messages {
		if v.SessionID == sessionID && v.WAID == waID {
			return v, true, nil
		}
	}
	return Message{}, false, nil
}

type Postgres struct{ db *sql.DB }

func NewPostgres(db *sql.DB) *Postgres { return &Postgres{db: db} }

func (p *Postgres) ListSessions(ctx context.Context) ([]Session, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id::text, session_id, status, COALESCE(method, ''), COALESCE(phone_number,''), COALESCE(qr_code,''), COALESCE(pairing_code,''), COALESCE(wa_jid,''), message_count, qr_expires_at, created_at, updated_at FROM (SELECT sessions.*, (SELECT COUNT(*) FROM messages m WHERE m.session_id=sessions.session_id) AS message_count FROM sessions) sessions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var v Session
		if err := rows.Scan(&v.ID, &v.SessionID, &v.Status, &v.Method, &v.PhoneNumber, &v.QRCode, &v.PairingCode, &v.WAJID, &v.MessageCount, &v.QRExpiresAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (p *Postgres) GetSession(ctx context.Context, id string) (Session, bool, error) {
	var v Session
	err := p.db.QueryRowContext(ctx, `SELECT s.id::text, s.session_id, s.status, COALESCE(s.method, ''), COALESCE(s.phone_number,''), COALESCE(s.qr_code,''), COALESCE(s.pairing_code,''), COALESCE(s.wa_jid,''), (SELECT COUNT(*) FROM messages m WHERE m.session_id=s.session_id), s.qr_expires_at, s.created_at, s.updated_at FROM sessions s WHERE s.session_id = $1`, id).Scan(&v.ID, &v.SessionID, &v.Status, &v.Method, &v.PhoneNumber, &v.QRCode, &v.PairingCode, &v.WAJID, &v.MessageCount, &v.QRExpiresAt, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, false, nil
	}
	return v, err == nil, err
}

func (p *Postgres) PutSession(ctx context.Context, v Session) error {
	_, err := p.db.ExecContext(ctx, `INSERT INTO sessions(session_id, status, method, phone_number, wa_jid, created_at, updated_at) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7) ON CONFLICT(session_id) DO UPDATE SET status=EXCLUDED.status, method=EXCLUDED.method, phone_number=EXCLUDED.phone_number, updated_at=EXCLUDED.updated_at`, v.SessionID, v.Status, v.Method, v.PhoneNumber, v.WAJID, v.CreatedAt, v.UpdatedAt)
	return err
}

func (p *Postgres) UpdateSession(ctx context.Context, id, status, qrCode, pairingCode, phone string) error {
	_, err := p.db.ExecContext(ctx, `UPDATE sessions SET status=$2, qr_code=NULLIF($3,''), pairing_code=NULLIF($4,''), qr_expires_at=CASE WHEN $3='' THEN NULL ELSE qr_expires_at END, phone_number=NULLIF($5,''), updated_at=now() WHERE session_id=$1`, id, status, qrCode, pairingCode, phone)
	return err
}
func (p *Postgres) SetSessionQRExpiry(ctx context.Context, id string, expiry *time.Time) error {
	_, err := p.db.ExecContext(ctx, `UPDATE sessions SET qr_expires_at=$2, updated_at=now() WHERE session_id=$1`, id, expiry)
	return err
}

func (p *Postgres) UpdateMessageStatus(ctx context.Context, id, status, waID, messageError string) error {
	_, err := p.db.ExecContext(ctx, `UPDATE messages SET status=$2, wa_message_id=NULLIF($3,''), error=NULLIF($4,''), sent_at=CASE WHEN $2='sent' THEN now() ELSE sent_at END, updated_at=now() WHERE id=$1`, id, status, waID, messageError)
	return err
}

func (p *Postgres) SetSessionIdentity(ctx context.Context, id, jid, phone string) error {
	_, err := p.db.ExecContext(ctx, `UPDATE sessions SET wa_jid=$2,phone_number=NULLIF($3,''),updated_at=now() WHERE session_id=$1`, id, jid, phone)
	return err
}

func (p *Postgres) UpdateMessageReceipt(ctx context.Context, sessionID, waID, status string, occurredAt time.Time) error {
	_, err := p.db.ExecContext(ctx, `UPDATE messages SET status=$3,delivered_at=CASE WHEN $3='delivered' THEN $4 ELSE delivered_at END,read_at=CASE WHEN $3='read' THEN $4 ELSE read_at END,updated_at=$4 WHERE session_id=$1 AND wa_message_id=$2 AND status IN ('sent','delivered')`, sessionID, waID, status, occurredAt)
	return err
}
func (p *Postgres) UpdateQueueJob(ctx context.Context, jobID, status, messageError string) error {
	_, err := p.db.ExecContext(ctx, `UPDATE queue_jobs SET status=$2,error=NULLIF($3,''),attempts=attempts+1,processed_at=CASE WHEN $2 IN ('active','completed','failed') THEN now() ELSE processed_at END,completed_at=CASE WHEN $2='completed' THEN now() ELSE completed_at END WHERE job_id=$1`, jobID, status, messageError)
	return err
}
func (p *Postgres) ListPendingQueueJobs(ctx context.Context) ([]PendingJob, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT job_id,data::text FROM queue_jobs WHERE status IN ('waiting','retrying') ORDER BY created_at LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingJob
	for rows.Next() {
		var j PendingJob
		var raw string
		if err := rows.Scan(&j.JobID, &raw); err != nil {
			return nil, err
		}
		j.Data = []byte(raw)
		out = append(out, j)
	}
	return out, rows.Err()
}
func (p *Postgres) GetMessageByWAID(ctx context.Context, sessionID, waID string) (Message, bool, error) {
	var v Message
	err := p.db.QueryRowContext(ctx, `SELECT id,session_id,"to",type,COALESCE(content,''),status,COALESCE(reference_id,''),COALESCE(source_type,''),COALESCE(source_id,'') FROM messages WHERE session_id=$1 AND wa_message_id=$2`, sessionID, waID).Scan(&v.ID, &v.SessionID, &v.To, &v.Type, &v.Content, &v.Status, &v.ReferenceID, &v.SourceType, &v.SourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, false, nil
	}
	return v, err == nil, err
}

func (p *Postgres) PutMessage(ctx context.Context, v Message) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO messages(id, job_id, session_id, "to", type, content, url, filename, mime_type, template_id, reference_id, source_type, source_id, idempotency_scope, idempotency_key_hash, idempotency_fingerprint, status, created_at, updated_at) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,'')::uuid,NULLIF($11,''),NULLIF($12,''),NULLIF($13,''),NULLIF($14,''),NULLIF($15,''),NULLIF($16,''),$17,$18,$18)`, v.ID, v.JobID, v.SessionID, v.To, v.Type, v.Content, v.URL, v.Filename, v.MIMEType, v.TemplateID, v.ReferenceID, v.SourceType, v.SourceID, v.IdempotencyScope, v.IdempotencyKeyHash, v.IdempotencyFingerprint, v.Status, v.CreatedAt); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO queue_jobs(queue_name, job_id, name, data, status, priority, max_attempts) VALUES($1,$2,'send-message',jsonb_build_object('messageId',$3::text,'sessionId',$4::text,'to',$5::text,'type',$6::text,'content',$7::text,'url',$8::text,'filename',$9::text,'mimeType',$10::text,'priority',$11::int,'delay',$12::int),'waiting',$11,3)`, queueName(v.Priority), v.JobID, v.ID, v.SessionID, v.To, v.Type, v.Content, v.URL, v.Filename, v.MIMEType, v.Priority, v.Delay); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (p *Postgres) ListMessages(ctx context.Context) ([]Message, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id, COALESCE(job_id,''), session_id, "to", type, COALESCE(content, ''), status, COALESCE(wa_message_id,''), COALESCE(error,''), created_at FROM messages ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var v Message
		if err := rows.Scan(&v.ID, &v.JobID, &v.SessionID, &v.To, &v.Type, &v.Content, &v.Status, &v.WAID, &v.Error, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
