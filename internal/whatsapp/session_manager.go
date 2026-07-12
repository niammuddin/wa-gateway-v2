package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
	"github.com/niammuddin/wa-gateway-v2/internal/store"
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
	clients    map[string]*WhatsMeowClient
	dispatcher EventDispatcher
}

func (m *SessionManager) SetDispatcher(d EventDispatcher) { m.dispatcher = d }

func NewSessionManager(ctx context.Context, databaseURL string, dataStore store.Store) (*SessionManager, error) {
	container, err := sqlstore.New(ctx, "postgres", databaseURL, waLog.Stdout("WhatsMeowStore", "WARN", false))
	if err != nil {
		return nil, err
	}
	return &SessionManager{container: container, store: dataStore, clients: map[string]*WhatsMeowClient{}}, nil
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
		case *events.Connected:
			_ = m.store.UpdateSession(context.Background(), sessionID, "connected", "", "", phone)
			if m.dispatcher != nil {
				_ = m.dispatcher.Dispatch(context.Background(), "session.connected", map[string]any{"sessionId": sessionID, "phoneNumber": phone})
			}
		case *events.Disconnected:
			_ = m.store.UpdateSession(context.Background(), sessionID, "disconnected", "", "", phone)
			if m.dispatcher != nil {
				_ = m.dispatcher.Dispatch(context.Background(), "session.disconnected", map[string]any{"sessionId": sessionID, "phoneNumber": phone})
			}
		case *events.LoggedOut:
			_ = m.store.UpdateSession(context.Background(), sessionID, "logged_out", "", "", phone)
		case *events.Receipt:
			status := ""
			switch v.Type {
			case events.ReceiptTypeDelivered:
				status = "delivered"
			case events.ReceiptTypeRead:
				status = "read"
			}
			if status != "" {
				for _, id := range v.MessageIDs {
					waID := string(id)
					if msg, ok, _ := m.store.GetMessageByWAID(context.Background(), sessionID, waID); ok && m.dispatcher != nil {
						_ = m.dispatcher.Dispatch(context.Background(), "message."+status, map[string]any{"messageId": msg.ID, "sessionId": sessionID, "to": msg.To, "waMessageId": waID, "referenceId": msg.ReferenceID, "sourceType": msg.SourceType, "sourceId": msg.SourceID, "timestamp": v.Timestamp.UTC().Format(time.RFC3339)})
					}
					_ = m.store.UpdateMessageReceipt(context.Background(), sessionID, waID, status, v.Timestamp)
				}
			}
		}
	})
	if !client.IsLoggedIn() {
		qr, err := client.QRChannel(connectionCtx)
		if err != nil {
			return err
		}
		go func() {
			for item := range qr {
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
							time.Sleep(time.Second)
						}
						if pairErr == nil {
							_ = m.store.UpdateSession(context.Background(), sessionID, "connecting", "", pairingCode, phone)
						} else {
							_ = m.store.UpdateSession(context.Background(), sessionID, "failed", "", "", phone)
						}
					} else {
						qrValue := item.Code
						if png, err := qrcode.Encode(item.Code, qrcode.Medium, 256); err == nil {
							qrValue = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
						}
						_ = m.store.UpdateSession(context.Background(), sessionID, "connecting", qrValue, "", phone)
						expires := time.Now().UTC().Add(item.Timeout)
						_ = m.store.SetSessionQRExpiry(context.Background(), sessionID, &expires)
					}
				}
			}
		}()
	}
	go func() {
		if err := client.Connect(connectionCtx); err != nil {
			_ = m.store.UpdateSession(context.Background(), sessionID, "failed", "", "", phone)
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
		m.mu.Lock()
		m.clients[session.SessionID] = client
		m.mu.Unlock()
		go func(c *WhatsMeowClient, id string) {
			if err := c.Connect(context.Background()); err != nil {
				_ = m.store.UpdateSession(context.Background(), id, "failed", "", "", session.PhoneNumber)
			}
		}(client, session.SessionID)
	}
	return nil
}

func (m *SessionManager) Reconnect(ctx context.Context, sessionID string) error {
	c, ok := m.get(sessionID)
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
			_ = m.store.UpdateSession(context.Background(), sessionID, "failed", "", "", "")
		}
	}()
	return nil
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
func (m *SessionManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.clients {
		_ = c.Disconnect()
	}
	_ = m.container.Close()
}
