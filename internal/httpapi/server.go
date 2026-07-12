package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
	"github.com/niammuddin/wa-gateway-v2/internal/admin"
	"github.com/niammuddin/wa-gateway-v2/internal/auth"
	"github.com/niammuddin/wa-gateway-v2/internal/events"
	"github.com/niammuddin/wa-gateway-v2/internal/queue"
	secretbox "github.com/niammuddin/wa-gateway-v2/internal/secret"
	"github.com/niammuddin/wa-gateway-v2/internal/store"
	"github.com/niammuddin/wa-gateway-v2/internal/webhook"
	"github.com/niammuddin/wa-gateway-v2/internal/whatsapp"
)

type Server struct {
	store      store.Store
	queue      queue.Queue
	auth       *auth.Service
	whatsapp   *whatsapp.SessionManager
	db         *sql.DB
	dispatcher *webhook.Dispatcher
	events     *events.Broker
	logger     *slog.Logger
}

func New(s store.Store, logger *slog.Logger) *Server { return &Server{store: s, logger: logger} }

func NewWithQueue(s store.Store, q queue.Queue, logger *slog.Logger) *Server {
	return &Server{store: s, queue: q, logger: logger}
}

func NewWithDependencies(s store.Store, q queue.Queue, a *auth.Service, logger *slog.Logger) *Server {
	return &Server{store: s, queue: q, auth: a, logger: logger}
}

func NewWithSession(s store.Store, q queue.Queue, a *auth.Service, manager *whatsapp.SessionManager, logger *slog.Logger) *Server {
	return &Server{store: s, queue: q, auth: a, whatsapp: manager, logger: logger}
}

func NewWithAll(s store.Store, q queue.Queue, a *auth.Service, manager *whatsapp.SessionManager, db *sql.DB, dispatcher *webhook.Dispatcher, logger *slog.Logger) *Server {
	return &Server{store: s, queue: q, auth: a, whatsapp: manager, db: db, dispatcher: dispatcher, events: events.NewBroker(), logger: logger}
}
func (s *Server) EventBroker() *events.Broker { return s.events }
func (s *Server) SetEventBroker(b *events.Broker) {
	if b != nil {
		s.events = b
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		write(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/admin", admin.Handler)
	r.Get("/admin/*", admin.Handler)
	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", s.login)
			r.Post("/refresh", s.refresh)
			r.Get("/session", s.refreshFromCookie)
			r.With(s.requireAuth).Post("/logout", s.logout)
			r.With(s.requireAuth).Put("/password", s.changePassword)
		})
		r.With(s.requireAuth).Get("/monitoring", func(w http.ResponseWriter, _ *http.Request) {
			databaseStatus, redisStatus := "not_configured", "not_configured"
			if os.Getenv("DATABASE_URL") != "" {
				databaseStatus = "configured"
			}
			if os.Getenv("REDIS_URL") != "" {
				redisStatus = "configured"
			}
			write(w, http.StatusOK, map[string]string{"status": "ok", "database": databaseStatus, "redis": redisStatus})
		})
		r.With(s.requireAuth).Route("/sessions", func(r chi.Router) {
			r.Get("/", s.listSessions)
			r.Post("/", s.createSession)
			r.Get("/{sessionID}", s.getSession)
			r.Post("/{sessionID}/reconnect", s.sessionAction("reconnecting"))
			r.Post("/{sessionID}/disconnect", s.sessionAction("disconnected"))
			r.Post("/{sessionID}/logout", s.sessionAction("logged_out"))
			r.Delete("/{sessionID}", s.deleteSession)
			r.Put("/{sessionID}/throttle", s.updateThrottle)
		})
		r.With(s.requireAuth).Route("/api-keys", func(r chi.Router) {
			r.Get("/", s.listAPIKeys)
			r.Post("/", s.createAPIKey)
			r.Put("/{id}", s.updateAPIKey)
			r.Delete("/{id}", s.deleteAPIKey)
		})
		r.With(s.requireAuth).Route("/templates", func(r chi.Router) {
			r.Get("/", s.listTemplates)
			r.Post("/", s.createTemplate)
			r.Get("/{id}", s.getTemplate)
			r.Put("/{id}", s.updateTemplate)
			r.Delete("/{id}", s.deleteTemplate)
		})
		r.With(s.requireAuth).Route("/webhooks", func(r chi.Router) {
			r.Get("/", s.listWebhooks)
			r.Post("/", s.createWebhook)
			r.Put("/{id}", s.updateWebhook)
			r.Delete("/{id}", s.deleteWebhook)
			r.Post("/{id}/test", s.testWebhook)
			r.Post("/deliveries/{id}/retry", s.retryWebhook)
			r.Get("/stats", s.webhookStats)
			r.Get("/deliveries", s.webhookDeliveries)
		})
		r.With(s.requireAuth).Get("/stats", s.stats)
		r.With(s.requireAuth).Get("/queue", s.queueStats)
		r.With(s.requireAuth).Get("/events", s.sseEvents)
		r.With(s.requireAuth).Get("/dashboard", s.dashboard)
		r.With(s.requireAuth).Route("/messages", func(r chi.Router) {
			r.Post("/send", s.sendMessage)
			r.Get("/", s.listMessages)
			r.Get("/{id}", s.getMessage)
			r.Post("/{id}/resend", s.resendMessage)
			r.Delete("/{id}", s.deleteMessage)
		})
	})
	return r
}

type principalKey struct{}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			next.ServeHTTP(w, r)
			return
		}
		var p auth.Principal
		var err error
		if key := r.Header.Get("X-Api-Key"); key != "" {
			p, err = s.auth.VerifyAPIKey(r.Context(), key, clientIP(r))
		} else {
			header := r.Header.Get("Authorization")
			if len(header) < 8 || !strings.EqualFold(header[:7], "Bearer ") {
				err = auth.ErrUnauthorized
			} else {
				p, err = s.auth.VerifyAccess(strings.TrimSpace(header[7:]))
			}
		}
		if err != nil {
			writeError(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

func (s *Server) sseEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	ch, unsubscribe := s.events.Subscribe()
	defer unsubscribe()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(": connected\n\n"))
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			_, _ = w.Write([]byte("event: " + evt.Name + "\ndata: " + string(evt.Data) + "\n\n"))
			flusher.Flush()
		}
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusNotImplemented, "authentication is not configured")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(r, &body) || body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password must be non-empty strings")
		return
	}
	access, refresh, expires, err := s.auth.Login(r.Context(), body.Username, body.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	setRefreshCookie(w, refresh)
	write(w, http.StatusOK, map[string]any{"accessToken": access, "expiresIn": expires})
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) { s.refreshToken(w, r) }
func (s *Server) refreshFromCookie(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie("refreshToken"); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.refreshToken(w, r)
}
func (s *Server) refreshToken(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusNotImplemented, "authentication is not configured")
		return
	}
	c, err := r.Cookie("refreshToken")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Missing refresh token")
		return
	}
	access, refresh, expires, err := s.auth.Refresh(r.Context(), c.Value)
	if err != nil {
		clearRefreshCookie(w)
		writeError(w, http.StatusUnauthorized, "Invalid refresh token")
		return
	}
	setRefreshCookie(w, refresh)
	write(w, http.StatusOK, map[string]any{"accessToken": access, "expiresIn": expires})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if p, ok := r.Context().Value(principalKey{}).(auth.Principal); ok && !p.IsAPIKey {
		_ = s.auth.Logout(r.Context(), p.UserID)
	}
	clearRefreshCookie(w)
	write(w, http.StatusOK, map[string]string{"message": "Logged out"})
}
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	p, ok := r.Context().Value(principalKey{}).(auth.Principal)
	if !ok || p.IsAPIKey {
		writeError(w, 401, "User authentication required")
		return
	}
	var b struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if !decode(r, &b) || b.OldPassword == "" || b.NewPassword == "" {
		writeError(w, 400, "oldPassword and newPassword are required")
		return
	}
	if err := s.auth.ChangePassword(r.Context(), p.UserID, b.OldPassword, b.NewPassword); errors.Is(err, auth.ErrUnauthorized) {
		writeError(w, 401, "Invalid old password")
		return
	} else if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	clearRefreshCookie(w)
	write(w, 200, map[string]string{"message": "Password changed"})
}

func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	values, err := s.auth.ListAPIKeys(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, values)
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeError(w, 501, "database is not configured")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,name,body,created_at,updated_at FROM templates ORDER BY created_at DESC`)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, name, body string
		var created, updated time.Time
		if err := rows.Scan(&id, &name, &body, &created, &updated); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{"id": id, "name": name, "body": body, "created_at": created, "updated_at": updated})
	}
	write(w, 200, out)
}
func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name string `json:"name"`
		Body string `json:"body"`
	}
	if !decode(r, &b) || strings.TrimSpace(b.Name) == "" || strings.TrimSpace(b.Body) == "" {
		writeError(w, 400, "name and body are required")
		return
	}
	var out map[string]any
	var id string
	var created, updated time.Time
	err := s.db.QueryRowContext(r.Context(), `INSERT INTO templates(name,body) VALUES($1,$2) RETURNING id,name,body,created_at,updated_at`, strings.TrimSpace(b.Name), b.Body).Scan(&id, &b.Name, &b.Body, &created, &updated)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	out = map[string]any{"id": id, "name": b.Name, "body": b.Body, "created_at": created, "updated_at": updated}
	write(w, 201, out)
}
func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request) {
	var id, name, body string
	var created, updated time.Time
	err := s.db.QueryRowContext(r.Context(), `SELECT id,name,body,created_at,updated_at FROM templates WHERE id=$1`, chi.URLParam(r, "id")).Scan(&id, &name, &body, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Template not found")
		return
	}
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"id": id, "name": name, "body": body, "created_at": created, "updated_at": updated})
}
func (s *Server) updateTemplate(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name string `json:"name"`
		Body string `json:"body"`
	}
	if !decode(r, &b) || strings.TrimSpace(b.Name) == "" || strings.TrimSpace(b.Body) == "" {
		writeError(w, 400, "name and body are required")
		return
	}
	var id string
	var created, updated time.Time
	err := s.db.QueryRowContext(r.Context(), `UPDATE templates SET name=$2,body=$3,updated_at=now() WHERE id=$1 RETURNING id,created_at,updated_at`, chi.URLParam(r, "id"), b.Name, b.Body).Scan(&id, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Template not found")
		return
	}
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	write(w, 200, map[string]any{"id": id, "name": b.Name, "body": b.Body, "created_at": created, "updated_at": updated})
}
func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM templates WHERE id=$1`, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, "Template not found")
		return
	}
	write(w, 200, map[string]string{"message": "Template deleted"})
}

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,url,events,COALESCE(session_ids,ARRAY[]::text[]),is_active,created_at,updated_at FROM webhooks ORDER BY created_at DESC`)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, url string
		var events, scopes []string
		var active bool
		var created, updated time.Time
		if err := rows.Scan(&id, &url, pq.Array(&events), pq.Array(&scopes), &active, &created, &updated); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{"id": id, "url": url, "events": events, "session_ids": scopes, "is_active": active, "created_at": created, "updated_at": updated})
	}
	write(w, 200, out)
}
func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	var b struct {
		URL        string   `json:"url"`
		Events     []string `json:"events"`
		Secret     string   `json:"secret"`
		SessionIDs []string `json:"sessionIds"`
		IsActive   *bool    `json:"isActive"`
	}
	if !decode(r, &b) || strings.TrimSpace(b.URL) == "" || len(b.Events) == 0 {
		writeError(w, 400, "url and events are required")
		return
	}
	active := true
	if b.IsActive != nil {
		active = *b.IsActive
	}
	encryptedSecret, err := secretbox.Encrypt(strings.TrimSpace(b.Secret), os.Getenv("WEBHOOK_ENCRYPTION_KEY"))
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	var id string
	var created, updated time.Time
	err = s.db.QueryRowContext(r.Context(), `INSERT INTO webhooks(url,events,secret_encrypted,session_ids,is_active) VALUES($1,$2,$3,$4,$5) RETURNING id,created_at,updated_at`, b.URL, b.Events, encryptedSecret, b.SessionIDs, active).Scan(&id, &created, &updated)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	write(w, 201, map[string]any{"id": id, "url": b.URL, "events": b.Events, "session_ids": b.SessionIDs, "is_active": active, "created_at": created, "updated_at": updated})
}
func (s *Server) updateWebhook(w http.ResponseWriter, r *http.Request) {
	var b struct {
		URL        string   `json:"url"`
		Events     []string `json:"events"`
		Secret     string   `json:"secret"`
		SessionIDs []string `json:"sessionIds"`
		IsActive   *bool    `json:"isActive"`
	}
	if !decode(r, &b) || strings.TrimSpace(b.URL) == "" || len(b.Events) == 0 {
		writeError(w, 400, "url and events are required")
		return
	}
	encryptedSecret, err := secretbox.Encrypt(strings.TrimSpace(b.Secret), os.Getenv("WEBHOOK_ENCRYPTION_KEY"))
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	var id string
	var created, updated time.Time
	err = s.db.QueryRowContext(r.Context(), `UPDATE webhooks SET url=$2,events=$3,secret_encrypted=CASE WHEN $4='' THEN secret_encrypted ELSE $4 END,session_ids=$5,is_active=COALESCE($6,is_active),updated_at=now() WHERE id=$1 RETURNING id,created_at,updated_at`, chi.URLParam(r, "id"), b.URL, b.Events, encryptedSecret, b.SessionIDs, b.IsActive).Scan(&id, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Webhook not found")
		return
	}
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	write(w, 200, map[string]any{"id": id, "url": b.URL, "events": b.Events, "session_ids": b.SessionIDs, "is_active": b.IsActive, "created_at": created, "updated_at": updated})
}
func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM webhooks WHERE id=$1`, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, "Webhook not found")
		return
	}
	write(w, 200, map[string]string{"message": "Webhook deleted"})
}
func (s *Server) testWebhook(w http.ResponseWriter, r *http.Request) {
	if s.dispatcher != nil {
		id, err := s.dispatcher.Test(r.Context(), chi.URLParam(r, "id"))
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "Webhook not found")
			return
		}
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		write(w, 202, map[string]any{"deliveryId": id, "status": "queued"})
		return
	}
	var id string
	err := s.db.QueryRowContext(r.Context(), `SELECT id FROM webhooks WHERE id=$1`, chi.URLParam(r, "id")).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Webhook not found")
		return
	}
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	write(w, 202, map[string]any{"deliveryId": "", "status": "queued"})
}
func (s *Server) retryWebhook(w http.ResponseWriter, r *http.Request) {
	if s.dispatcher == nil {
		writeError(w, 501, "webhook dispatcher is not configured")
		return
	}
	if err := s.dispatcher.Retry(r.Context(), chi.URLParam(r, "id")); errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Delivery not found")
		return
	} else if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]any{"id": chi.URLParam(r, "id"), "status": "queued"})
}
func (s *Server) webhookStats(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT status,count(*) FROM webhook_deliveries GROUP BY status`)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer rows.Close()
	by := map[string]int{}
	total := 0
	for rows.Next() {
		var status string
		var n int
		_ = rows.Scan(&status, &n)
		by[status] = n
		total += n
	}
	write(w, 200, map[string]any{"total": total, "byStatus": by})
}
func (s *Server) webhookDeliveries(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,webhook_id,event,status,attempts,response_status,error,created_at,delivered_at FROM webhook_deliveries ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, wid, event, status string
		var attempts int
		var response sql.NullInt64
		var message sql.NullString
		var created time.Time
		var delivered sql.NullTime
		if err := rows.Scan(&id, &wid, &event, &status, &attempts, &response, &message, &created, &delivered); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		out = append(out, map[string]any{"id": id, "webhook_id": wid, "event": event, "status": status, "attempts": attempts, "response_status": response.Int64, "error": message.String, "created_at": created, "delivered_at": delivered.Time})
	}
	write(w, 200, map[string]any{"data": out, "total": len(out), "page": 1, "limit": 100})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	var messages, sessions int
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM messages`).Scan(&messages)
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM sessions WHERE status='connected'`).Scan(&sessions)
	rows, err := s.db.QueryContext(r.Context(), `SELECT status,count(*) FROM messages GROUP BY status`)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer rows.Close()
	by := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		_ = rows.Scan(&status, &n)
		by[status] = n
	}
	write(w, 200, map[string]any{"messages": messages, "activeSessions": sessions, "messagesByStatus": by})
}
func (s *Server) queueStats(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT status,count(*) FROM queue_jobs GROUP BY status`)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		_ = rows.Scan(&status, &n)
		counts[status] = n
	}
	write(w, 200, map[string]any{"counts": counts})
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	var sessions, connected, queued, failed int
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM sessions`).Scan(&sessions)
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM sessions WHERE status='connected'`).Scan(&connected)
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM messages WHERE status='queued'`).Scan(&queued)
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM messages WHERE status='failed'`).Scan(&failed)
	write(w, 200, map[string]any{"sessions": sessions, "activeSessions": connected, "queueSize": queued, "failedMessages": failed})
}
func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string   `json:"name"`
		AllowedIPs []string `json:"allowedIps"`
		RateLimit  *int     `json:"rateLimit"`
		SessionID  string   `json:"sessionId"`
		IsActive   *bool    `json:"isActive"`
	}
	if !decode(r, &body) || strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	active := true
	if body.IsActive != nil {
		active = *body.IsActive
	}
	value, key, err := s.auth.CreateAPIKey(r.Context(), strings.TrimSpace(body.Name), body.AllowedIPs, body.RateLimit, body.SessionID, active)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusCreated, map[string]any{"id": value.ID, "name": value.Name, "key_prefix": value.KeyPrefix, "key": key, "allowed_ips": value.AllowedIPs, "rate_limit": value.RateLimit, "session_id": value.SessionID, "is_active": value.IsActive, "created_at": value.CreatedAt, "updated_at": value.UpdatedAt})
}
func (s *Server) updateAPIKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string   `json:"name"`
		AllowedIPs []string `json:"allowedIps"`
		RateLimit  *int     `json:"rateLimit"`
		SessionID  string   `json:"sessionId"`
		IsActive   *bool    `json:"isActive"`
	}
	if !decode(r, &body) || strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	value, err := s.auth.UpdateAPIKey(r.Context(), chi.URLParam(r, "id"), strings.TrimSpace(body.Name), body.AllowedIPs, body.RateLimit, body.SessionID, body.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "API key not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, value)
}
func (s *Server) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	err := s.auth.DeleteAPIKey(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "API key not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]string{"message": "API key deleted"})
}
func setRefreshCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{Name: "refreshToken", Value: value, Path: "/api/v1/auth", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 7 * 24 * 60 * 60})
}
func clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "refreshToken", Value: "", Path: "/api/v1/auth", HttpOnly: true, MaxAge: -1})
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListSessions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, items)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID   string `json:"sessionId"`
		Method      string `json:"method"`
		PhoneNumber string `json:"phoneNumber"`
	}
	if !decode(r, &body) || body.SessionID == "" {
		writeError(w, http.StatusBadRequest, "sessionId is required")
		return
	}
	if _, ok, err := s.store.GetSession(r.Context(), body.SessionID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if ok {
		writeError(w, http.StatusConflict, "session already exists")
		return
	}
	if body.Method == "" {
		body.Method = "qr"
	}
	if body.Method != "qr" && body.Method != "pairing" {
		writeError(w, 400, "method must be qr or pairing")
		return
	}
	if body.Method == "pairing" {
		phone := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, body.PhoneNumber)
		if len(phone) < 10 || len(phone) > 15 {
			writeError(w, 400, "phoneNumber must contain 10-15 digits for pairing")
			return
		}
		body.PhoneNumber = phone
	}
	now := time.Now().UTC()
	session := store.Session{ID: body.SessionID, SessionID: body.SessionID, Method: body.Method, Status: "connecting", CreatedAt: now, UpdatedAt: now}
	if err := s.store.PutSession(r.Context(), session); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.whatsapp != nil {
		if err := s.whatsapp.Create(r.Context(), body.SessionID, body.Method, body.PhoneNumber); err != nil {
			_ = s.store.UpdateSession(r.Context(), body.SessionID, "failed", "", "", "")
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	write(w, http.StatusCreated, session)
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	v, ok, err := s.store.GetSession(r.Context(), chi.URLParam(r, "sessionID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	write(w, http.StatusOK, v)
}

func (s *Server) sessionAction(status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "sessionID")
		v, ok, err := s.store.GetSession(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		v.Status = status
		v.UpdatedAt = time.Now().UTC()
		if err := s.store.PutSession(r.Context(), v); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if s.whatsapp != nil {
			var err error
			switch status {
			case "reconnecting":
				err = s.whatsapp.Reconnect(r.Context(), id)
			case "disconnected":
				err = s.whatsapp.Disconnect(id)
			case "logged_out":
				err = s.whatsapp.Logout(r.Context(), id)
			}
			if err != nil {
				writeError(w, http.StatusBadGateway, err.Error())
				return
			}
		}
		if status == "logged_out" {
			v.Status = "disconnected"
			v.QRCode, v.PairingCode, v.QRExpiresAt = "", "", nil
			_ = s.store.PutSession(r.Context(), v)
			write(w, http.StatusOK, map[string]string{"message": "Session logged out"})
			return
		}
		if status == "disconnected" {
			write(w, http.StatusOK, map[string]string{"message": "Session disconnected"})
			return
		}
		write(w, http.StatusOK, v)
	}
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "sessionID")
	var count int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM messages WHERE session_id=$1`, id).Scan(&count); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if count > 0 {
		writeError(w, 409, "Session cannot be deleted because it has message history. Disconnect or logout the session instead.")
		return
	}
	if s.whatsapp != nil {
		_ = s.whatsapp.Logout(r.Context(), id)
	}
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE session_id=$1`, id)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, "Session not found")
		return
	}
	write(w, 200, map[string]string{"message": "Session deleted"})
}
func (s *Server) updateThrottle(w http.ResponseWriter, r *http.Request) {
	var b struct {
		MinIntervalMs        *int `json:"minIntervalMs"`
		JitterMs             *int `json:"jitterMs"`
		MaxMessagesPerMinute *int `json:"maxMessagesPerMinute"`
		FailureThreshold     *int `json:"failureThreshold"`
		PauseDurationMs      *int `json:"pauseDurationMs"`
	}
	if !decode(r, &b) {
		writeError(w, 400, "invalid throttle body")
		return
	}
	checks := []struct {
		name     string
		value    *int
		min, max int
	}{{"minIntervalMs", b.MinIntervalMs, 100, 60000}, {"jitterMs", b.JitterMs, 0, 60000}, {"maxMessagesPerMinute", b.MaxMessagesPerMinute, 1, 600}, {"failureThreshold", b.FailureThreshold, 1, 100}, {"pauseDurationMs", b.PauseDurationMs, 10000, 86400000}}
	for _, c := range checks {
		if c.value != nil && (*c.value < c.min || *c.value > c.max) {
			writeError(w, 400, c.name+" is out of range")
			return
		}
	}
	var min, jitter, max, threshold, pause int
	err := s.db.QueryRowContext(r.Context(), `UPDATE sessions SET min_interval_ms=COALESCE($2,min_interval_ms),jitter_ms=COALESCE($3,jitter_ms),max_messages_per_minute=COALESCE($4,max_messages_per_minute),failure_threshold=COALESCE($5,failure_threshold),pause_duration_ms=COALESCE($6,pause_duration_ms),updated_at=now() WHERE session_id=$1 RETURNING min_interval_ms,jitter_ms,max_messages_per_minute,failure_threshold,pause_duration_ms`, chi.URLParam(r, "sessionID"), b.MinIntervalMs, b.JitterMs, b.MaxMessagesPerMinute, b.FailureThreshold, b.PauseDurationMs).Scan(&min, &jitter, &max, &threshold, &pause)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Session not found")
		return
	}
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	write(w, 200, map[string]int{"minIntervalMs": min, "jitterMs": jitter, "maxMessagesPerMinute": max, "failureThreshold": threshold, "pauseDurationMs": pause})
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID   string            `json:"sessionId"`
		To          string            `json:"to"`
		Type        string            `json:"type"`
		Message     string            `json:"message"`
		TemplateID  string            `json:"templateId"`
		Variables   map[string]string `json:"variables"`
		ReferenceID string            `json:"referenceId"`
		SourceType  string            `json:"sourceType"`
		SourceID    string            `json:"sourceId"`
		URL         string            `json:"url"`
		Filename    string            `json:"filename"`
		MIMEType    string            `json:"mimeType"`
		Priority    int               `json:"priority"`
		Delay       int               `json:"delay"`
	}
	if !decode(r, &body) || body.SessionID == "" || body.To == "" || body.Type == "" {
		writeError(w, http.StatusBadRequest, "sessionId, to, and type are required")
		return
	}
	switch body.Type {
	case "text", "image", "document", "pdf":
	default:
		writeError(w, http.StatusBadRequest, "type must be one of: text, image, document, pdf")
		return
	}
	if len(body.ReferenceID) > 255 {
		writeError(w, http.StatusBadRequest, "referenceId must not exceed 255 characters")
		return
	}
	if _, ok, err := s.store.GetSession(r.Context(), body.SessionID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if p, ok := r.Context().Value(principalKey{}).(auth.Principal); ok && p.IsAPIKey && p.SessionID != "" && p.SessionID != body.SessionID {
		writeError(w, http.StatusForbidden, "API key is restricted to another session")
		return
	}
	if body.TemplateID != "" {
		var template string
		if err := s.db.QueryRowContext(r.Context(), `SELECT body FROM templates WHERE id=$1`, body.TemplateID).Scan(&template); err != nil {
			writeError(w, 404, "Template not found")
			return
		}
		body.Message = resolveTemplate(template, body.Variables)
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	scope := ""
	if p, ok := r.Context().Value(principalKey{}).(auth.Principal); ok {
		if p.IsAPIKey {
			scope = "api-key:" + p.APIKeyID
		} else {
			scope = "user:" + p.UserID
		}
	}
	keyHash := hashString(idempotencyKey)
	if idempotencyKey != "" && s.db != nil && scope != "" {
		var existingID, existingJob, status string
		if err := s.db.QueryRowContext(r.Context(), `SELECT id,COALESCE(job_id,''),status FROM messages WHERE idempotency_scope=$1 AND idempotency_key_hash=$2`, scope, keyHash).Scan(&existingID, &existingJob, &status); err == nil {
			write(w, 200, map[string]any{"messageId": existingID, "jobId": existingJob, "status": status, "message": "Message already queued"})
			return
		}
	}
	id := randomID()
	if body.Delay < 0 {
		body.Delay = 0
	}
	message := store.Message{ID: id, SessionID: body.SessionID, To: body.To, Type: body.Type, Content: body.Message, URL: body.URL, Filename: body.Filename, MIMEType: body.MIMEType, TemplateID: body.TemplateID, ReferenceID: body.ReferenceID, SourceType: body.SourceType, SourceID: body.SourceID, Priority: body.Priority, Delay: body.Delay, Status: "queued", CreatedAt: time.Now().UTC()}
	if idempotencyKey != "" && scope != "" {
		message.IdempotencyScope = scope
		message.IdempotencyKeyHash = keyHash
	}
	message.JobID = id
	if err := s.store.PutMessage(r.Context(), message); err != nil {
		// Two concurrent requests with the same idempotency key may race between
		// the lookup above and the unique insert. Return the already-created
		// message instead of exposing a database 500.
		if idempotencyKey != "" && scope != "" {
			var existingID, existingJob, status string
			if lookupErr := s.db.QueryRowContext(r.Context(), `SELECT id,COALESCE(job_id,''),status FROM messages WHERE idempotency_scope=$1 AND idempotency_key_hash=$2`, scope, keyHash).Scan(&existingID, &existingJob, &status); lookupErr == nil {
				write(w, http.StatusOK, map[string]any{"messageId": existingID, "jobId": existingJob, "status": status, "message": "Message already queued"})
				return
			}
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.queue != nil {
		if err := s.queue.Enqueue(r.Context(), queue.MessageJob{MessageID: message.ID, SessionID: message.SessionID, To: message.To, Type: message.Type, Content: message.Content, URL: message.URL, Filename: message.Filename, MIMEType: message.MIMEType, Priority: message.Priority, Delay: message.Delay}, message.JobID); err != nil {
			writeError(w, http.StatusServiceUnavailable, "message queue unavailable")
			return
		}
	}
	write(w, http.StatusAccepted, map[string]any{"messageId": id, "jobId": id, "status": "queued", "message": "Message queued for delivery"})
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	page := 1
	limit := 20
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 0 {
		page = n
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 100 {
		limit = n
	}
	if s.db == nil {
		all, err := s.store.ListMessages(r.Context())
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		search, status, sessionID := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search"))), r.URL.Query().Get("status"), r.URL.Query().Get("sessionId")
		filtered := make([]store.Message, 0, len(all))
		for _, v := range all {
			if status != "" && v.Status != status || sessionID != "" && v.SessionID != sessionID || search != "" && !strings.Contains(strings.ToLower(v.To), search) && !strings.Contains(strings.ToLower(v.Content), search) {
				continue
			}
			filtered = append(filtered, v)
		}
		start := (page - 1) * limit
		if start > len(filtered) {
			start = len(filtered)
		}
		end := start + limit
		if end > len(filtered) {
			end = len(filtered)
		}
		write(w, 200, map[string]any{"data": filtered[start:end], "total": len(filtered), "page": page, "limit": limit})
		return
	}
	where := []string{"1=1"}
	args := []any{}
	add := func(v any, clause string) {
		args = append(args, v)
		where = append(where, clause+"$"+strconv.Itoa(len(args)))
	}
	if v := strings.TrimSpace(r.URL.Query().Get("sessionId")); v != "" {
		add(v, "session_id=")
	}
	if v := strings.TrimSpace(r.URL.Query().Get("status")); v != "" {
		add(v, "status=")
	}
	if v := strings.TrimSpace(r.URL.Query().Get("search")); v != "" {
		args = append(args, "%"+v+"%")
		p := "$" + strconv.Itoa(len(args))
		where = append(where, "(\"to\" ILIKE "+p+" OR COALESCE(content,'') ILIKE "+p+")")
	}
	filter := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(r.Context(), "SELECT count(*) FROM messages WHERE "+filter, args...).Scan(&total); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	args = append(args, limit, (page-1)*limit)
	rows, err := s.db.QueryContext(r.Context(), "SELECT id,COALESCE(job_id,''),session_id,\"to\",type,COALESCE(content,''),status,COALESCE(wa_message_id,''),COALESCE(error,''),created_at FROM messages WHERE "+filter+" ORDER BY created_at DESC LIMIT $"+strconv.Itoa(len(args)-1)+" OFFSET $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []store.Message{}
	for rows.Next() {
		var v store.Message
		if err := rows.Scan(&v.ID, &v.JobID, &v.SessionID, &v.To, &v.Type, &v.Content, &v.Status, &v.WAID, &v.Error, &v.CreatedAt); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{"data": items, "total": total, "page": page, "limit": limit})
}
func (s *Server) getMessage(w http.ResponseWriter, r *http.Request) {
	var id, job, session, to, typ, content, status, waID, errorText, url, filename, mime string
	var created, queued, sent, delivered, read, updated sql.NullTime
	err := s.db.QueryRowContext(r.Context(), `SELECT id,COALESCE(job_id,''),session_id,"to",type,COALESCE(content,''),status,COALESCE(wa_message_id,''),COALESCE(error,''),COALESCE(url,''),COALESCE(filename,''),COALESCE(mime_type,''),created_at,queued_at,sent_at,delivered_at,read_at,updated_at FROM messages WHERE id=$1`, chi.URLParam(r, "id")).Scan(&id, &job, &session, &to, &typ, &content, &status, &waID, &errorText, &url, &filename, &mime, &created, &queued, &sent, &delivered, &read, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Message not found")
		return
	}
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	date := func(v sql.NullTime) any {
		if !v.Valid {
			return nil
		}
		return v.Time
	}
	write(w, 200, map[string]any{"id": id, "job_id": job, "session_id": session, "to": to, "type": typ, "content": content, "status": status, "wa_message_id": waID, "error": errorText, "url": url, "filename": filename, "mime_type": mime, "created_at": date(created), "queued_at": date(queued), "sent_at": date(sent), "delivered_at": date(delivered), "read_at": date(read), "updated_at": date(updated)})
}
func (s *Server) resendMessage(w http.ResponseWriter, r *http.Request) {
	var old store.Message
	err := s.db.QueryRowContext(r.Context(), `SELECT id,session_id,"to",type,COALESCE(content,''),status FROM messages WHERE id=$1`, chi.URLParam(r, "id")).Scan(&old.ID, &old.SessionID, &old.To, &old.Type, &old.Content, &old.Status)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Message not found")
		return
	}
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	old.ID = randomID()
	old.JobID = old.ID
	old.Status = "queued"
	old.CreatedAt = time.Now().UTC()
	if err := s.store.PutMessage(r.Context(), old); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if s.queue != nil {
		if err := s.queue.Enqueue(r.Context(), queue.MessageJob{MessageID: old.ID, SessionID: old.SessionID, To: old.To, Type: old.Type, Content: old.Content}, old.JobID); err != nil {
			writeError(w, 503, "message queue unavailable")
			return
		}
	}
	write(w, 202, map[string]any{"messageId": old.ID, "jobId": old.JobID, "status": "queued", "message": "Message requeued"})
}
func (s *Server) deleteMessage(w http.ResponseWriter, r *http.Request) {
	var status string
	err := s.db.QueryRowContext(r.Context(), `SELECT status FROM messages WHERE id=$1`, chi.URLParam(r, "id")).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Message not found")
		return
	}
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if status == "queued" || status == "processing" {
		writeError(w, 409, "Message cannot be deleted while queued or processing")
		return
	}
	_, err = s.db.ExecContext(r.Context(), `DELETE FROM messages WHERE id=$1`, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	write(w, 200, map[string]string{"message": "Message deleted"})
}

func decode(r *http.Request, dst any) bool {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dst) == nil
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	write(w, status, map[string]any{"statusCode": status, "error": http.StatusText(status), "message": message})
}
func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return strings.ReplaceAll(time.Now().Format(time.RFC3339Nano), ":", "")
	}
	return hex.EncodeToString(b)
}

func resolveTemplate(body string, variables map[string]string) string {
	for key, value := range variables {
		body = strings.ReplaceAll(body, "{{"+key+"}}", value)
	}
	return body
}
func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
