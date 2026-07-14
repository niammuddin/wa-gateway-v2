package webhook

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestNewHTTPClientDefaultsToIPv4(t *testing.T) {
	t.Setenv("WEBHOOK_HTTP_IP_VERSION", "")
	client := newHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}

	conn, err := transport.DialContext(context.Background(), "tcp", "127.0.0.1:1")
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected connection to fail")
	}
	if _, ok := err.(*net.OpError); !ok {
		t.Fatalf("expected net.OpError, got %T", err)
	}
}

func TestNewHTTPClientHonorsAddressFamily(t *testing.T) {
	t.Setenv("WEBHOOK_HTTP_IP_VERSION", "6")
	client := newHTTPClient()
	transport := client.Transport.(*http.Transport)
	_, err := transport.DialContext(context.Background(), "tcp", "127.0.0.1:1")
	if err == nil {
		t.Fatal("expected connection to fail")
	}
	if !strings.Contains(err.Error(), "network is unreachable") && !strings.Contains(err.Error(), "cannot assign requested address") && !strings.Contains(err.Error(), "connection refused") && !strings.Contains(err.Error(), "no suitable address") {
		t.Fatalf("unexpected dial error: %v", err)
	}
}
