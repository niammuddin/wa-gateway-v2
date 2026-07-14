package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	secretbox "github.com/niammuddin/wa-gateway-v2/internal/secret"
)

type Dispatcher struct {
	db     *sql.DB
	client *http.Client
	key    string
}

func (d *Dispatcher) Test(ctx context.Context, webhookID string) (string, error) {
	id := uuid.New()
	payload, _ := json.Marshal(map[string]any{"event": "webhook.test", "timestamp": time.Now().UTC().Format(time.RFC3339), "data": map[string]any{"webhookId": webhookID}})
	if _, err := d.db.ExecContext(ctx, `INSERT INTO webhook_deliveries(id,webhook_id,event_id,event,payload,status) VALUES($1,$2,$3,'webhook.test',$4,'queued')`, id, webhookID, uuid.New(), payload); err != nil {
		return "", err
	}
	return id.String(), d.Retry(ctx, id.String())
}

func New(db *sql.DB) *Dispatcher {
	return &Dispatcher{db: db, client: newHTTPClient(), key: os.Getenv("WEBHOOK_ENCRYPTION_KEY")}
}

func newHTTPClient() *http.Client {
	addressFamily := strings.TrimSpace(os.Getenv("WEBHOOK_HTTP_IP_VERSION"))
	if addressFamily == "" {
		addressFamily = "4"
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if addressFamily == "4" {
				network = "tcp4"
			} else if addressFamily == "6" {
				network = "tcp6"
			}
			return dialer.DialContext(ctx, network, address)
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: 20 * time.Second}
}

func (d *Dispatcher) Dispatch(ctx context.Context, event string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT id,url,events,COALESCE(secret_encrypted,''),COALESCE(session_ids,ARRAY[]::text[]),is_active FROM webhooks WHERE is_active=true`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, url, encrypted string
		var events, scopes []string
		var active bool
		if err := rows.Scan(&id, &url, pq.Array(&events), &encrypted, pq.Array(&scopes), &active); err != nil {
			return err
		}
		if !contains(events, event) || !scopeMatches(scopes, payload["sessionId"]) {
			continue
		}
		deliveryID := uuid.New()
		if _, err := d.db.ExecContext(ctx, `INSERT INTO webhook_deliveries(id,webhook_id,event_id,event,payload,status) VALUES($1,$2,$3,$4,$5,'queued')`, deliveryID, id, uuid.New(), event, body); err != nil {
			return err
		}
		secret, _ := secretbox.Decrypt(encrypted, d.key)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-WA-Event", event)
		req.Header.Set("X-WA-Signature", sign(body, secret))
		resp, sendErr := d.client.Do(req)
		status := "delivered"
		var errorText any
		if sendErr != nil {
			status = "failed"
			errorText = sendErr.Error()
		} else {
			_ = resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				status = "failed"
				errorText = resp.Status
			}
		}
		_, _ = d.db.ExecContext(ctx, `UPDATE webhook_deliveries SET status=$2,attempts=1,response_status=$3,error=$4,delivered_at=CASE WHEN $2='delivered' THEN now() ELSE delivered_at END,updated_at=now() WHERE id=$1`, deliveryID, status, nullableStatus(resp), errorText)
	}
	return rows.Err()
}

func (d *Dispatcher) Retry(ctx context.Context, deliveryID string) error {
	var url, encrypted, event string
	var payload []byte
	err := d.db.QueryRowContext(ctx, `SELECT w.url,w.secret_encrypted,d.event,d.payload FROM webhook_deliveries d JOIN webhooks w ON w.id=d.webhook_id WHERE d.id=$1`, deliveryID).Scan(&url, &encrypted, &event, &payload)
	if err != nil {
		return err
	}
	secret, _ := secretbox.Decrypt(encrypted, d.key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-WA-Event", event)
	req.Header.Set("X-WA-Signature", sign(payload, secret))
	resp, sendErr := d.client.Do(req)
	status := "delivered"
	var errorText any
	if sendErr != nil {
		status = "failed"
		errorText = sendErr.Error()
	} else {
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			status = "failed"
			errorText = resp.Status
		}
	}
	_, err = d.db.ExecContext(ctx, `UPDATE webhook_deliveries SET status=$2,attempts=attempts+1,response_status=$3,error=$4,delivered_at=CASE WHEN $2='delivered' THEN now() ELSE delivered_at END,updated_at=now() WHERE id=$1`, deliveryID, status, nullableStatus(resp), errorText)
	return err
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func scopeMatches(scopes []string, value any) bool {
	if len(scopes) == 0 {
		return true
	}
	session, ok := value.(string)
	if !ok {
		return false
	}
	return contains(scopes, session)
}
func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
func nullableStatus(resp *http.Response) any {
	if resp == nil {
		return nil
	}
	return resp.StatusCode
}
