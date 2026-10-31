package whatsapp

import (
	"errors"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
)

var (
	// ErrLoginNotReady is returned when the WhatsApp login session is not ready
	// to generate a pairing code yet.
	ErrLoginNotReady = errors.New("WhatsApp login is not ready yet")
	// ErrAlreadyLinked is returned when the WhatsApp account is already linked.
	ErrAlreadyLinked = errors.New("WhatsApp is already linked")
	// ErrInvalidPhone is returned when the phone number cannot be used for pairing.
	ErrInvalidPhone = errors.New("invalid phone number")
)

const (
	pairLoginReadyTimeout = 20 * time.Second
	pairClientName        = "Chrome (Windows)"
)

// waitForLoginReady waits until the WhatsApp websocket is ready to start a
// pairing flow. It returns ErrLoginNotReady if the session does not become
// ready in time.
func (w *WhatsAppClient) waitForLoginReady(timeout time.Duration) error {
	if w.loginReady == nil {
		return ErrLoginNotReady
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-w.loginReady:
		return nil
	case <-timer.C:
		return ErrLoginNotReady
	}
}

// PairPhone generates an 8-digit pairing code so a phone number can link the
// WhatsApp account without scanning a QR code.
func (w *WhatsAppClient) PairPhone(phone string) (string, error) {
	if err := w.waitForLoginReady(pairLoginReadyTimeout); err != nil {
		return "", err
	}

	client := w.getClient()
	if client == nil {
		return "", ErrLoginNotReady
	}
	if client.Store.ID != nil {
		return "", ErrAlreadyLinked
	}
	if !client.IsConnected() {
		return "", ErrLoginNotReady
	}

	// Only one pairing code should be active at a time: a new code replaces
	// the previous one on WhatsApp's side.
	w.pairMu.Lock()
	defer w.pairMu.Unlock()

	// Re-check after waiting for any other pairing request to finish.
	if client.Store.ID != nil {
		return "", ErrAlreadyLinked
	}
	if !client.IsConnected() {
		return "", ErrLoginNotReady
	}

	code, err := client.PairPhone(w.Ctx, phone, false, whatsmeow.PairClientChrome, pairClientName)
	if err != nil {
		if errors.Is(err, whatsmeow.ErrPhoneNumberTooShort) || errors.Is(err, whatsmeow.ErrPhoneNumberIsNotInternational) {
			return "", fmt.Errorf("%w: %v", ErrInvalidPhone, err)
		}
		if errors.Is(err, whatsmeow.ErrNotConnected) {
			return "", ErrLoginNotReady
		}
		return "", err
	}

	return code, nil
}

func (w *WhatsAppClient) getClient() *whatsmeow.Client {
	w.clientMu.RLock()
	defer w.clientMu.RUnlock()
	return w.Client
}

func (w *WhatsAppClient) setClient(client *whatsmeow.Client) {
	w.clientMu.Lock()
	defer w.clientMu.Unlock()
	w.Client = client
}

func (w *WhatsAppClient) markLoginReady() {
	w.loginReadyOnce.Do(func() {
		if w.loginReady != nil {
			close(w.loginReady)
		}
	})
}

func (w *WhatsAppClient) markConnected() {
	w.connectedOnce.Do(func() {
		if w.Connected != nil {
			close(w.Connected)
		}
	})
}
