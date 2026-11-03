package whatsapp

import (
	"errors"
	"testing"
	"time"
)

func TestWaitForLoginReadyReturnsWhenReady(t *testing.T) {
	ready := make(chan struct{})
	close(ready)
	client := &WhatsAppClient{loginReady: ready}

	if err := client.waitForLoginReady(time.Second); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestWaitForLoginReadyTimesOut(t *testing.T) {
	client := &WhatsAppClient{loginReady: make(chan struct{})}

	err := client.waitForLoginReady(10 * time.Millisecond)
	if !errors.Is(err, ErrLoginNotReady) {
		t.Fatalf("expected ErrLoginNotReady, got %v", err)
	}
}

func TestWaitForLoginReadyWithoutChannelReturnsNotReady(t *testing.T) {
	client := &WhatsAppClient{}

	err := client.waitForLoginReady(10 * time.Millisecond)
	if !errors.Is(err, ErrLoginNotReady) {
		t.Fatalf("expected ErrLoginNotReady, got %v", err)
	}
}
