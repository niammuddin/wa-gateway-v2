package whatsapp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// Client is the protocol boundary. The API and domain packages must depend on
// this interface, not on whatsmeow types. This keeps the gateway replaceable
// with another WhatsApp adapter or Cloud API adapter later.
type Client interface {
	Connect(context.Context) error
	Disconnect() error
	Logout(context.Context) error
	SendText(context.Context, string, string) (SendResult, error)
	Status() string
}

type SendResult struct {
	WhatsAppMessageID string
}

// Manager owns exactly one client per session_id in a single process. A future
// multi-worker deployment must add a lease/lock before moving ownership out of
// process memory.
type Manager struct {
	mu      sync.RWMutex
	clients map[string]Client
}

func NewManager() *Manager { return &Manager{clients: make(map[string]Client)} }

func (m *Manager) Put(sessionID string, client Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[sessionID] = client
}
func (m *Manager) Get(sessionID string) (Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.clients[sessionID]
	return v, ok
}
func (m *Manager) Delete(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clients, sessionID)
}

// WhatsMeowClient is the first protocol adapter. It intentionally exposes only
// the small Client interface used by the gateway domain.
type WhatsMeowClient struct {
	client *whatsmeow.Client
	mu     sync.RWMutex
	status string
}

func (c *WhatsMeowClient) AddEventHandler(handler whatsmeow.EventHandler) uint32 {
	return c.client.AddEventHandler(handler)
}
func (c *WhatsMeowClient) IsLoggedIn() bool { return c.client.IsLoggedIn() }
func (c *WhatsMeowClient) JID() string {
	if c.client.Store.ID == nil {
		return ""
	}
	return c.client.Store.ID.String()
}

func NewWhatsMeowClient(device *store.Device, module string) *WhatsMeowClient {
	// Use the supported Chrome companion profile for the linked-device label.
	// This is metadata only; it does not change the session's cryptographic
	// identity or credential storage.
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
	store.SetOSInfo("Linux", [3]uint32{1, 0, 0})
	logger := waLog.Stdout(module, "WARN", false)
	return &WhatsMeowClient{client: whatsmeow.NewClient(device, logger), status: "disconnected"}
}

func (c *WhatsMeowClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	c.status = "connecting"
	c.mu.Unlock()
	err := c.client.ConnectContext(ctx)
	c.mu.Lock()
	if err != nil {
		c.status = "failed"
	} else {
		c.status = "connected"
	}
	c.mu.Unlock()
	return err
}

func (c *WhatsMeowClient) Disconnect() error {
	c.client.Disconnect()
	c.mu.Lock()
	c.status = "disconnected"
	c.mu.Unlock()
	return nil
}
func (c *WhatsMeowClient) PairPhone(ctx context.Context, phone string) (string, error) {
	// Ask WhatsApp to notify the phone as soon as the linking code is issued.
	return c.client.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
}

func (c *WhatsMeowClient) Logout(ctx context.Context) error {
	err := c.client.Logout(ctx)
	c.mu.Lock()
	c.status = "logged_out"
	c.mu.Unlock()
	return err
}

func (c *WhatsMeowClient) SendText(ctx context.Context, to string, message string) (SendResult, error) {
	phone := regexp.MustCompile(`\D`).ReplaceAllString(to, "")
	if phone == "" {
		return SendResult{}, fmt.Errorf("invalid WhatsApp destination")
	}
	jid := types.NewJID(phone, types.DefaultUserServer)
	resp, err := c.client.SendMessage(ctx, jid, &waE2E.Message{Conversation: proto.String(message)})
	if err != nil {
		return SendResult{}, normalizeWhatsAppError(err)
	}
	return SendResult{WhatsAppMessageID: string(resp.ID)}, nil
}

func (c *WhatsMeowClient) SendMedia(ctx context.Context, to, kind, rawURL, filename, caption string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("media download returned %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 25<<20))
	if err != nil {
		return "", err
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}
	appInfo := whatsmeow.MediaDocument
	if kind == "image" {
		appInfo = whatsmeow.MediaImage
	}
	upload, err := c.client.Upload(ctx, data, appInfo)
	if err != nil {
		return "", err
	}
	phone := regexp.MustCompile(`\D`).ReplaceAllString(to, "")
	jid := types.NewJID(phone, types.DefaultUserServer)
	var msg *waE2E.Message
	if kind == "image" {
		msg = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{URL: proto.String(upload.URL), Mimetype: proto.String(mime), Caption: proto.String(caption), FileSHA256: upload.FileSHA256, FileLength: proto.Uint64(upload.FileLength), MediaKey: upload.MediaKey, FileEncSHA256: upload.FileEncSHA256, DirectPath: proto.String(upload.DirectPath)}}
	} else {
		msg = &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{URL: proto.String(upload.URL), Mimetype: proto.String(mime), Title: proto.String(caption), FileName: proto.String(filename), FileSHA256: upload.FileSHA256, FileLength: proto.Uint64(upload.FileLength), MediaKey: upload.MediaKey, FileEncSHA256: upload.FileEncSHA256, DirectPath: proto.String(upload.DirectPath)}}
	}
	result, err := c.client.SendMessage(ctx, jid, msg)
	return string(result.ID), normalizeWhatsAppError(err)
}

func (c *WhatsMeowClient) Status() string { c.mu.RLock(); defer c.mu.RUnlock(); return c.status }

// QRChannel exposes the raw QR data used by the API layer to render a data URL.
// It must be called before Connect for a new device.
func (c *WhatsMeowClient) QRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return c.client.GetQRChannel(ctx)
}

func NormalizePhone(value string) string {
	return strings.TrimSpace(regexp.MustCompile(`\D`).ReplaceAllString(value, ""))
}
