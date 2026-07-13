package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/niammuddin/wa-gateway-v2/internal/store"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type EventDispatcher interface {
	Dispatch(context.Context, string, map[string]any) error
}
type SessionManager struct {
	container  *sqlstore.Container
	store      store.Store
	mu         sync.RWMutex
	receiptMu  sync.Mutex
	clients    map[string]*WhatsMeowClient
	dispatcher EventDispatcher
	logger     *slog.Logger
}

func (m *SessionManager) SetDispatcher(d EventDispatcher) { m.dispatcher = d }

func NewSessionManager(ctx context.Context, databaseURL string, dataStore store.Store) (*SessionManager, error) {
	container, err := sqlstore.New(ctx, "postgres", databaseURL, waLog.Stdout("WhatsMeowStore", "WARN", false))
	if err != nil {
		return nil, err
	}
	return &SessionManager{container: container, store: dataStore, clients: map[string]*WhatsMeowClient{}, logger: slog.Default()}, nil
}

func (m *SessionManager) Create(ctx context.Context, sessionID, method, phone string) error {
	// The HTTP request context ends as soon as the create response is returned.
	// WhatsApp connection ownership must outlive that request.
	connectionCtx := context.Background()
	m.mu.Lock()
	if _, ok := m.clients[sessionID]; ok {
		m.mu.Unlock()
		return nil
	}
	device := m.container.NewDevice()
	client := NewWhatsMeowClient(device, "WhatsApp/"+sessionID)
	m.clients[sessionID] = client
	m.mu.Unlock()
	client.AddEventHandler(func(evt any) {
		switch v := evt.(type) {
		case *events.PairSuccess:
			_ = m.store.SetSessionIdentity(context.Background(), sessionID, v.ID.String(), v.ID.User)
			_ = m.store.UpdateSession(context.Background(), sessionID, "connecting", "", "", v.ID.User)
			m.dispatchSessionStatus(sessionID, "connecting", v.ID.User)
		case *events.Connected:
			connectedPhone := phone
			if connectedPhone == "" {
				if current, ok, _ := m.store.GetSession(context.Background(), sessionID); ok {
					connectedPhone = current.PhoneNumber
				}
			}
			_ = m.store.UpdateSession(context.Background(), sessionID, "connected", "", "", connectedPhone)
			m.dispatchSessionStatus(sessionID, "connected", connectedPhone)
		case *events.Disconnected:
			_ = m.store.UpdateSession(context.Background(), sessionID, "disconnected", "", "", phone)
			m.dispatchSessionStatus(sessionID, "disconnected", phone)
		case *events.LoggedOut:
			_ = m.store.UpdateSession(context.Background(), sessionID, "logged_out", "", "", phone)
			m.dispatchSessionStatus(sessionID, "logged_out", phone)
		case *events.Receipt:
			m.handleReceipt(sessionID, v)
		}
	})
	if !client.IsLoggedIn() {
		qr, err := client.QRChannel(connectionCtx)
		if err != nil {
			return err
		}
		go func() {
			for item := range qr {
				if !m.isCurrentClient(sessionID, client) {
					return
				}
				if item.Event == "timeout" {
					_ = m.store.UpdateSession(context.Background(), sessionID, "failed", "", "", phone)
					m.dispatchSessionStatus(sessionID, "failed", phone)
					continue
				}
				if item.Event == "code" {
					if method == "pairing" && phone != "" {
						// The first QR item signals that the websocket is ready, but
						// WhatsApp can still reject the pairing request for a short
						// window. Match the Node implementation with bounded retries.
						var pairingCode string
						var pairErr error
						for attempt := 0; attempt < 5; attempt++ {
							pairingCode, pairErr = client.PairPhone(connectionCtx, phone)
							if pairErr == nil {
								break
							}
							m.logger.Warn("whatsapp pairing code request failed", "session_id", sessionID, "attempt", attempt+1, "error", pairErr)
							if isPairingRateLimited(pairErr) {
								break
							}
							time.Sleep(time.Duration(attempt+1) * time.Second)
						}
						if pairErr == nil {
							if !m.isCurrentClient(sessionID, client) {
								return
							}
							_ = m.store.UpdateSession(context.Background(), sessionID, "connecting", "", pairingCode, phone)
							expires := time.Now().UTC().Add(160 * time.Second)
							_ = m.store.SetSessionQRExpiry(context.Background(), sessionID, &expires)
							m.dispatchSessionStatus(sessionID, "connecting", phone)
						} else {
							if !m.isCurrentClient(sessionID, client) {
								return
							}
							_ = m.store.UpdateSession(context.Background(), sessionID, "failed", "", "", phone)
							m.logger.Error("whatsapp pairing failed", "session_id", sessionID, "error", pairErr)
							m.dispatchSessionStatus(sessionID, "failed", phone)
						}
					} else {
						qrValue := item.Code
						if png, err := qrcode.Encode(item.Code, qrcode.Medium, 256); err == nil {
							qrValue = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
						}
						if !m.isCurrentClient(sessionID, client) {
							return
						}
						_ = m.store.UpdateSession(context.Background(), sessionID, "connecting", qrValue, "", phone)
						expires := time.Now().UTC().Add(item.Timeout)
						_ = m.store.SetSessionQRExpiry(context.Background(), sessionID, &expires)
						m.dispatchSessionStatus(sessionID, "connecting", phone)
					}
				}
			}
		}()
	}
	go func() {
		if err := client.Connect(connectionCtx); err != nil {
			if !m.isCurrentClient(sessionID, client) {
				return
			}
			_ = m.store.UpdateSession(context.Background(), sessionID, "failed", "", "", phone)
			m.logger.Error("whatsapp session connect failed", "session_id", sessionID, "error", err)
			m.dispatchSessionStatus(sessionID, "failed", phone)
		}
	}()
	return nil
}

func (m *SessionManager) Load(ctx context.Context) error {
	sessions, err := m.store.ListSessions(ctx)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session.WAJID == "" {
			continue
		}
		jid, err := types.ParseJID(session.WAJID)
		if err != nil {
			continue
		}
		device, err := m.container.GetDevice(ctx, jid)
		if err != nil || device == nil {
			continue
		}
		client := NewWhatsMeowClient(device, "WhatsApp/"+session.SessionID)
		// Restored clients need the same lifecycle handlers as newly-created
		// clients; otherwise a successful reconnect remains stuck in the
		// database as "reconnecting".
		client.AddEventHandler(func(evt any) {
			switch v := evt.(type) {
			case *events.Connected:
				_ = m.store.UpdateSession(context.Background(), session.SessionID, "connected", "", "", session.PhoneNumber)
				m.dispatchSessionStatus(session.SessionID, "connected", session.PhoneNumber)
			case *events.Disconnected:
				_ = m.store.UpdateSession(context.Background(), session.SessionID, "disconnected", "", "", session.PhoneNumber)
				m.dispatchSessionStatus(session.SessionID, "disconnected", session.PhoneNumber)
			case *events.LoggedOut:
				_ = m.store.UpdateSession(context.Background(), session.SessionID, "logged_out", "", "", session.PhoneNumber)
				m.dispatchSessionStatus(session.SessionID, "logged_out", session.PhoneNumber)
			case *events.PairSuccess:
				_ = m.store.SetSessionIdentity(context.Background(), session.SessionID, v.ID.String(), v.ID.User)
				m.dispatchSessionStatus(session.SessionID, "connecting", v.ID.User)
			case *events.Receipt:
				m.handleReceipt(session.SessionID, v)
			}
		})
		m.mu.Lock()
		m.clients[session.SessionID] = client
		m.mu.Unlock()
		go func(c *WhatsMeowClient, id string) {
			if err := c.Connect(context.Background()); err != nil {
				if !m.isCurrentClient(id, c) {
					return
				}
				_ = m.store.UpdateSession(context.Background(), id, "failed", "", "", session.PhoneNumber)
				m.dispatchSessionStatus(id, "failed", session.PhoneNumber)
			}
		}(client, session.SessionID)
	}
	return nil
}

// handleReceipt makes WhatsApp receipts idempotent before publishing them to
// SSE/webhook consumers. WhatsApp may deliver the same receipt more than once.
func (m *SessionManager) handleReceipt(sessionID string, receipt *events.Receipt) {
	status := ""
	switch receipt.Type {
	case events.ReceiptTypeDelivered:
		status = "delivered"
	case events.ReceiptTypeRead:
		status = "read"
	default:
		return
	}

	m.receiptMu.Lock()
	defer m.receiptMu.Unlock()
	for _, id := range receipt.MessageIDs {
		waID := string(id)
		msg, ok, err := m.store.GetMessageByWAID(context.Background(), sessionID, waID)
		if err != nil || !ok || (status == "delivered" && msg.Status != "sent") || (status == "read" && msg.Status != "sent" && msg.Status != "delivered") {
			continue
		}
		if err := m.store.UpdateMessageReceipt(context.Background(), sessionID, waID, status, receipt.Timestamp); err != nil {
			continue
		}
		if m.dispatcher != nil {
			_ = m.dispatcher.Dispatch(context.Background(), "message."+status, map[string]any{
				"messageId": msg.ID, "sessionId": sessionID, "to": msg.To, "waMessageId": waID,
				"referenceId": msg.ReferenceID, "sourceType": msg.SourceType, "sourceId": msg.SourceID,
				"timestamp": receipt.Timestamp.UTC().Format(time.RFC3339),
			})
		}
	}
}

func (m *SessionManager) Reconnect(ctx context.Context, sessionID string) error {
	c, ok := m.get(sessionID)
	if ok && !c.IsLoggedIn() {
		_ = c.Disconnect()
		m.mu.Lock()
		delete(m.clients, sessionID)
		m.mu.Unlock()
		c = nil
		ok = false
	}
	if !ok {
		// Logout removes the authenticated device. Recreate a fresh client so
		// reconnect starts a new QR/pairing flow instead of failing on a stale
		// client that can no longer authenticate.
		session, found, err := m.store.GetSession(ctx, sessionID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("session %s not found", sessionID)
		}
		return m.Create(context.Background(), sessionID, session.Method, session.PhoneNumber)
	}
	// The HTTP request context is cancelled as soon as the response is sent.
	// Connection ownership belongs to the session manager, so reconnect must
	// outlive that request just like initial session creation.
	go func() {
		if err := c.Connect(context.Background()); err != nil {
			if !m.isCurrentClient(sessionID, c) {
				return
			}
			_ = m.store.UpdateSession(context.Background(), sessionID, "failed", "", "", "")
			m.dispatchSessionStatus(sessionID, "failed", "")
		}
	}()
	return nil
}

func isPairingRateLimited(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "rate-overlimit") || strings.Contains(message, "status 429")
}

func (m *SessionManager) dispatchSessionStatus(sessionID, status, phone string) {
	if m.dispatcher == nil {
		return
	}
	payload := map[string]any{"sessionId": sessionID, "status": status, "phoneNumber": phone}
	if session, ok, err := m.store.GetSession(context.Background(), sessionID); err == nil && ok {
		payload["qrCode"] = session.QRCode
		payload["pairingCode"] = session.PairingCode
		payload["qrExpiresAt"] = session.QRExpiresAt
		if session.PhoneNumber != "" {
			payload["phoneNumber"] = session.PhoneNumber
		}
	}
	_ = m.dispatcher.Dispatch(context.Background(), "session."+status, payload)
}
func (m *SessionManager) Disconnect(sessionID string) error {
	c, ok := m.get(sessionID)
	if !ok {
		return fmt.Errorf("session %s not loaded", sessionID)
	}
	return c.Disconnect()
}
func (m *SessionManager) Logout(ctx context.Context, sessionID string) error {
	c, ok := m.get(sessionID)
	if !ok {
		return fmt.Errorf("session %s not loaded", sessionID)
	}
	err := c.Logout(ctx)
	// A logged-out whatsmeow client cannot be reused for a new login.
	m.mu.Lock()
	delete(m.clients, sessionID)
	m.mu.Unlock()
	return err
}
func (m *SessionManager) Client(sessionID string) (*WhatsMeowClient, bool) { return m.get(sessionID) }

type QueueSender struct{ Client *WhatsMeowClient }

func (s QueueSender) SendText(ctx context.Context, to, content string) (string, error) {
	result, err := s.Client.SendText(ctx, to, content)
	return result.WhatsAppMessageID, err
}
func (s QueueSender) SendMedia(ctx context.Context, to, kind, rawURL, filename, caption string) (string, error) {
	return s.Client.SendMedia(ctx, to, kind, rawURL, filename, caption)
}
func (m *SessionManager) get(id string) (*WhatsMeowClient, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clients[id]
	return c, ok
}

func (m *SessionManager) isCurrentClient(id string, candidate *WhatsMeowClient) bool {
	current, ok := m.get(id)
	return ok && current == candidate
}
func (m *SessionManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.clients {
		_ = c.Disconnect()
	}
	_ = m.container.Close()
}
