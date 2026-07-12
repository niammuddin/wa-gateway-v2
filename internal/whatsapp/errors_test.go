package whatsapp

import (
	"errors"
	"fmt"
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestReachoutTimelockError(t *testing.T) {
	raw := fmt.Errorf("%w 463", whatsmeow.ErrServerReturnedError)
	if !IsReachoutTimelockError(raw) {
		t.Fatal("raw whatsmeow 463 should be detected")
	}
	normalized := normalizeWhatsAppError(raw)
	if !errors.Is(normalized, ErrReachoutTimelock) {
		t.Fatalf("normalized error does not wrap ErrReachoutTimelock: %v", normalized)
	}
	if IsReachoutTimelockError(fmt.Errorf("%w 4631", whatsmeow.ErrServerReturnedError)) {
		t.Fatal("4631 must not be treated as 463")
	}
}
