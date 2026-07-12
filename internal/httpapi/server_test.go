package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/niammuddin/wa-gateway-v2/internal/store"
	"log/slog"
)

func TestSessionAndMessageContract(t *testing.T) {
	app := New(store.NewMemory(), slog.Default()).Handler()

	request := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/", bytes.NewBufferString(`{"sessionId":"isp-main","method":"qr"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create session status = %d, want %d", response.Code, http.StatusCreated)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/messages/send", bytes.NewBufferString(`{"sessionId":"isp-main","to":"628123456789","type":"text","message":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("send status = %d, want %d", response.Code, http.StatusAccepted)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "queued" {
		t.Fatalf("status = %v, want queued", payload["status"])
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/messages/send", bytes.NewBufferString(`{"sessionId":"isp-main","to":"628123456789","type":"text","message":"second"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("second send status = %d, want %d", response.Code, http.StatusAccepted)
	}
	list := httptest.NewRecorder()
	app.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/messages/", nil))
	var listed struct {
		Data []store.Message `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 2 {
		t.Fatalf("messages = %d, want 2 without idempotency headers", len(listed.Data))
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/messages/send", bytes.NewBufferString(`{"sessionId":"isp-main","to":"628123456789","type":"text","message":"delayed","priority":7,"delay":1500}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("priority/delay send status = %d, want %d", response.Code, http.StatusAccepted)
	}
	list = httptest.NewRecorder()
	app.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/messages/", nil))
	var priorityPayload struct {
		Data []store.Message `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &priorityPayload); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range priorityPayload.Data {
		if message.Priority == 7 && message.Delay == 1500 && message.Content == "delayed" {
			found = true
			break
		}
	}
	if len(priorityPayload.Data) != 3 || !found {
		t.Fatalf("priority/delay message not persisted: %+v", priorityPayload.Data)
	}
}

func TestMessageRequiresExistingSession(t *testing.T) {
	app := New(store.NewMemory(), slog.Default()).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/messages/send", bytes.NewBufferString(`{"sessionId":"missing","to":"628123456789","type":"text"}`))
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestAuthSessionWithoutCookieReturnsNoContent(t *testing.T) {
	app := New(store.NewMemory(), slog.Default()).Handler()
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("auth session status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestMessageRejectsInvalidType(t *testing.T) {
	app := New(store.NewMemory(), slog.Default()).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/", bytes.NewBufferString(`{"sessionId":"isp-main","method":"qr"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	request = httptest.NewRequest(http.MethodPost, "/api/v1/messages/send", bytes.NewBufferString(`{"sessionId":"isp-main","to":"628123456789","type":"video"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid type status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestMessageRejectsLongReferenceID(t *testing.T) {
	app := New(store.NewMemory(), slog.Default()).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/", bytes.NewBufferString(`{"sessionId":"isp-main","method":"qr"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)

	request = httptest.NewRequest(http.MethodPost, "/api/v1/messages/send", bytes.NewBufferString(`{"sessionId":"isp-main","to":"628123456789","type":"text","referenceId":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("long referenceId status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
