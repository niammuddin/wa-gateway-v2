package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/niammuddin/wa-gateway-v2/internal/auth"
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

func TestPublicContractRoutes(t *testing.T) {
	app := New(store.NewMemory(), slog.Default()).Handler()
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "landing page", method: http.MethodGet, path: "/", wantStatus: http.StatusOK},
		{name: "health", method: http.MethodGet, path: "/health", wantStatus: http.StatusOK},
		{name: "admin shell", method: http.MethodGet, path: "/admin", wantStatus: http.StatusOK},
		{name: "auth session without cookie", method: http.MethodGet, path: "/api/v1/auth/session", wantStatus: http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			app.ServeHTTP(response, httptest.NewRequest(tt.method, tt.path, nil))
			if response.Code != tt.wantStatus {
				t.Fatalf("%s %s status = %d, want %d", tt.method, tt.path, response.Code, tt.wantStatus)
			}
		})
	}
}

func TestProtectedRoutesRequireAuthentication(t *testing.T) {
	app := NewWithDependencies(store.NewMemory(), nil, auth.New(nil, "access-secret", "refresh-secret"), slog.Default()).Handler()
	paths := []string{
		"/api/v1/sessions/",
		"/api/v1/messages/",
		"/api/v1/api-keys/",
		"/api/v1/templates/",
		"/api/v1/webhooks/",
		"/api/v1/stats",
		"/api/v1/queue",
		"/api/v1/monitoring",
		"/api/v1/dashboard",
		"/api/v1/events",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("GET %s status = %d, want %d", path, response.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestBearerAuthenticationContract(t *testing.T) {
	service := auth.New(nil, "access-secret", "refresh-secret")
	app := NewWithDependencies(store.NewMemory(), nil, service, slog.Default()).Handler()

	for _, token := range []string{"not-a-jwt", "Bearer", "Bearer not-a-jwt"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/", nil)
		request.Header.Set("Authorization", token)
		app.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("authorization %q status = %d, want %d", token, response.Code, http.StatusUnauthorized)
		}
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":      "user-1",
		"username": "admin",
		"exp":      now.Add(time.Minute).Unix(),
		"iat":      now.Unix(),
		"iss":      "wa-gateway",
	})
	signed, err := token.SignedString([]byte("access-secret"))
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/", nil)
	request.Header.Set("Authorization", "Bearer "+signed)
	app.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("valid bearer status = %d, want %d", response.Code, http.StatusOK)
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
