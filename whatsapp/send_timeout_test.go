package whatsapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestSendRecipientCutsAtTimeout(t *testing.T) {
	const timeout = 50 * time.Millisecond
	start := time.Now()

	_, elapsed, err := sendRecipient(context.Background(), timeout, func(ctx context.Context) (types.MessageID, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed < timeout {
		t.Fatalf("elapsed = %s, want >= %s", elapsed, timeout)
	}
	if total := time.Since(start); total > time.Second {
		t.Fatalf("sendRecipient tardo %s; debio cortar en ~%s", total, timeout)
	}
}

func TestSendRecipientReturnsMessageID(t *testing.T) {
	want := types.MessageID("ABC123")

	got, _, err := sendRecipient(context.Background(), time.Second, func(ctx context.Context) (types.MessageID, error) {
		return want, nil
	})

	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if got != want {
		t.Fatalf("id = %q, want %q", got, want)
	}
}

func TestSendRecipientReturnsSendError(t *testing.T) {
	wantErr := errors.New("fallo de whatsapp")

	_, _, err := sendRecipient(context.Background(), time.Second, func(ctx context.Context) (types.MessageID, error) {
		return "", wantErr
	})

	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}
