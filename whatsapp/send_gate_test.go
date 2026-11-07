package whatsapp

import (
	"context"
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func TestSendGateSpacing(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}
	gate := NewSendGate(20, 0)
	gate.now = func() time.Time { return clock.now }
	gate.sleep = func(ctx context.Context, d time.Duration) error {
		clock.now = clock.now.Add(d)
		return nil
	}
	start := clock.now
	if err := gate.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := gate.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := gate.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := clock.now.Sub(start); got != 6*time.Second {
		t.Fatalf("elapsed = %s, want 6s (20/min)", got)
	}
}

func TestSendGateHonorsCancellation(t *testing.T) {
	gate := NewSendGate(20, 0)
	gate.next = time.Now().Add(time.Hour)
	gate.sleep = func(ctx context.Context, d time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gate.Wait(ctx); err == nil {
		t.Fatal("expected cancellation error")
	}
}
