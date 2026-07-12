package whatsapp

import (
	"errors"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
)

var ErrReachoutTimelock = errors.New("WhatsApp reach-out timelock (error 463): establish a conversation with the recipient first, then retry")

func IsReachoutTimelockError(err error) bool {
	return err != nil && (errors.Is(err, ErrReachoutTimelock) ||
		(errors.Is(err, whatsmeow.ErrServerReturnedError) && strings.HasSuffix(err.Error(), " 463")))
}

func normalizeWhatsAppError(err error) error {
	if !IsReachoutTimelockError(err) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrReachoutTimelock, err)
}
