package whatsapp

import (
	"context"
	"errors"
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestClassifySendError(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		retryable bool
		ambiguous bool
	}{
		{"timeout", context.DeadlineExceeded, true, true},
		{"not connected", whatsmeow.ErrNotConnected, true, true},
		{"unknown", errors.New("network reset"), true, true},
		{"permanent", ErrPermanent, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifySendError(c.err)
			if got.Retryable != c.retryable || got.Ambiguous != c.ambiguous {
				t.Fatalf("got retryable=%v ambiguous=%v, want %v/%v", got.Retryable, got.Ambiguous, c.retryable, c.ambiguous)
			}
		})
	}
}

func TestBuildForwardMessageMissingSourceIsPermanent(t *testing.T) {
	w := NewWhatsAppClient()
	job := &Job{
		ChatID:  "120363012345678901@g.us",
		Payload: JobPayload{Type: "forward", ForwardFromMessageID: "missing"},
	}
	_, _, err := w.buildJobMessage(job)
	if !errors.Is(err, ErrPermanent) {
		t.Fatalf("err = %v, want ErrPermanent", err)
	}
}

func TestBuildForwardMessageAppliesCaptionOverride(t *testing.T) {
	w := NewWhatsAppClient()
	w.forwardableMessages["m1"] = &forwardableMessage{
		imageURL:     "https://example.com/a.jpg",
		imageCaption: "old",
		text:         "old",
	}
	caption := "new"
	msg, _, err := w.buildJobMessage(&Job{
		ChatID:  "120363012345678901@g.us",
		Payload: JobPayload{Type: "forward", ForwardFromMessageID: "m1", CaptionOverride: &caption},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := msg.GetImageMessage().GetCaption(); got != "new" {
		t.Fatalf("caption = %q, want new", got)
	}
}

func TestParseJIDStrictRejectsEmpty(t *testing.T) {
	if _, err := parseJIDStrict(""); err == nil {
		t.Fatal("expected error for empty JID")
	}
	if _, err := parseJIDStrict("120363012345678901@g.us"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsConnectedRequiresClientAndConnectedSignal(t *testing.T) {
	w := NewWhatsAppClient()
	if w.IsConnected() {
		t.Fatal("client without a connection should not be connected")
	}
	w.Client = &whatsmeow.Client{}
	close(w.Connected)
	if !w.IsConnected() {
		t.Fatal("connected client should be reported as connected")
	}
}

func TestSendJobWithoutClientIsAmbiguous(t *testing.T) {
	w := NewWhatsAppClient()
	_, err := w.SendJob(context.Background(), &Job{
		ChatID:  "a@s.whatsapp.net",
		Payload: JobPayload{Type: "text", Text: "hello"},
	})
	if !errors.Is(err, whatsmeow.ErrNotConnected) {
		t.Fatalf("err = %v, want ErrNotConnected", err)
	}
	var jobErr *JobError
	if !errors.As(err, &jobErr) || !jobErr.Retryable || !jobErr.Ambiguous {
		t.Fatalf("error classification = %+v, want retryable ambiguous JobError", jobErr)
	}
}
