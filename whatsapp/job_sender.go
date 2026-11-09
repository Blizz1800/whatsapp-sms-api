package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"main/config"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

var ErrPermanent = errors.New("permanent job error")

type JobError struct {
	Err                  error
	Retryable, Ambiguous bool
}

func (e *JobError) Error() string {
	if e.Err == nil {
		return ErrPermanent.Error()
	}
	return e.Err.Error()
}

func (e *JobError) Unwrap() error {
	return e.Err
}

type JobSender interface {
	IsConnected() bool
	SendJob(ctx context.Context, job *Job) (JobResult, error)
}

func classifySendError(err error) *JobError {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrPermanent) {
		return &JobError{Err: err, Retryable: false, Ambiguous: false}
	}
	return &JobError{Err: err, Retryable: true, Ambiguous: true}
}

func (w *WhatsAppClient) IsConnected() bool {
	if w.Client == nil {
		return false
	}
	select {
	case <-w.Connected:
		return true
	default:
		return false
	}
}

func (w *WhatsAppClient) SendJob(ctx context.Context, job *Job) (JobResult, error) {
	if w.Client == nil {
		return JobResult{}, classifySendError(whatsmeow.ErrNotConnected)
	}
	message, jid, err := w.buildJobMessageWithContext(ctx, job)
	if err != nil {
		return JobResult{}, classifySendError(err)
	}
	sent, err := w.Client.SendMessage(ctx, jid, message)
	if err != nil {
		return JobResult{}, classifySendError(err)
	}
	w.recordSentMessage(sent.ID)
	return JobResult{
		MessageID: sent.ID,
		ChatID:    job.ChatID,
		SentAt:    nowTS(),
	}, nil
}

func (w *WhatsAppClient) buildJobMessage(job *Job) (*waE2E.Message, types.JID, error) {
	ctx := w.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return w.buildJobMessageWithContext(ctx, job)
}

func (w *WhatsAppClient) buildJobMessageWithContext(ctx context.Context, job *Job) (*waE2E.Message, types.JID, error) {
	if job == nil {
		return nil, types.JID{}, fmt.Errorf("%w: job is nil", ErrPermanent)
	}
	jid, err := parseJIDStrict(job.ChatID)
	if err != nil {
		return nil, types.JID{}, fmt.Errorf("%w: invalid chat id: %v", ErrPermanent, err)
	}

	switch job.Payload.Type {
	case "text":
		if job.Payload.Text == "" {
			return nil, types.JID{}, fmt.Errorf("%w: text payload is empty", ErrPermanent)
		}
		if isNewsletter(jid) {
			return &waE2E.Message{Conversation: proto.String(job.Payload.Text)}, jid, nil
		}
		return &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:        proto.String(job.Payload.Text),
				ContextInfo: &waE2E.ContextInfo{},
			},
		}, jid, nil
	case "forward":
		return w.buildForwardJobMessage(job, jid)
	case "image":
		return w.buildImageJobMessage(ctx, job, jid)
	default:
		return nil, types.JID{}, fmt.Errorf("%w: unsupported message type %q", ErrPermanent, job.Payload.Type)
	}
}

func (w *WhatsAppClient) buildForwardJobMessage(job *Job, jid types.JID) (*waE2E.Message, types.JID, error) {
	id := job.Payload.ForwardFromMessageID
	if id == "" {
		return nil, types.JID{}, fmt.Errorf("%w: source message id is empty", ErrPermanent)
	}
	w.mu.RLock()
	fm, ok := w.forwardableMessages[id]
	if ok {
		copy := *fm
		fm = &copy
	}
	w.mu.RUnlock()
	if !ok {
		fm = LoadForwardableMessage(id)
	}
	if fm == nil {
		return nil, types.JID{}, fmt.Errorf("%w: source message not found", ErrPermanent)
	}

	if fm.imageURL != "" && job.Payload.CaptionOverride != nil {
		if fm.text == fm.imageCaption {
			fm.text = ""
		}
		fm.imageCaption = *job.Payload.CaptionOverride
	}
	var message *waE2E.Message
	if isNewsletter(jid) {
		message = fm.buildNewsletterMessage()
	} else {
		message = fm.buildMessage()
	}
	if message == nil {
		return nil, types.JID{}, fmt.Errorf("%w: source message is empty", ErrPermanent)
	}
	return message, jid, nil
}

func (w *WhatsAppClient) buildImageJobMessage(ctx context.Context, job *Job, jid types.JID) (*waE2E.Message, types.JID, error) {
	media := job.Payload.Media
	if media == nil {
		return nil, types.JID{}, fmt.Errorf("%w: image payload is missing media", ErrPermanent)
	}
	parsed, err := url.ParseRequestURI(media.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, types.JID{}, fmt.Errorf("%w: invalid image URL", ErrPermanent)
	}
	cfg := config.Load()
	client := &http.Client{Timeout: time.Duration(cfg.SendTimeoutSeconds) * time.Second}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, types.JID{}, fmt.Errorf("%w: create image request: %v", ErrPermanent, err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, types.JID{}, fmt.Errorf("%w: download image: %v", ErrPermanent, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, types.JID{}, fmt.Errorf("%w: image server returned %s", ErrPermanent, response.Status)
	}
	imageBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, types.JID{}, fmt.Errorf("%w: read image: %v", ErrPermanent, err)
	}
	mimeType := media.MimeType
	if mimeType == "" {
		mimeType = response.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = "image/png"
		}
	}
	uploadCtx := ctx
	if uploadCtx == nil {
		uploadCtx = context.Background()
	}
	upload, err := w.Client.Upload(uploadCtx, imageBytes, whatsmeow.MediaImage)
	if err != nil {
		return nil, types.JID{}, fmt.Errorf("%w: upload image: %v", ErrPermanent, err)
	}
	image := &waE2E.ImageMessage{
		URL:           &upload.URL,
		DirectPath:    &upload.DirectPath,
		MediaKey:      upload.MediaKey,
		FileEncSHA256: upload.FileEncSHA256,
		FileSHA256:    upload.FileSHA256,
		FileLength:    &upload.FileLength,
		Mimetype:      proto.String(strings.TrimSpace(mimeType)),
		Caption:       proto.String(media.Caption),
	}
	return &waE2E.Message{ImageMessage: image}, jid, nil
}

func parseJIDStrict(raw string) (types.JID, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "+"))
	if raw == "" {
		return types.JID{}, errors.New("JID is empty")
	}
	var jid types.JID
	switch {
	case strings.HasSuffix(raw, "@g.us"):
		jid = types.NewJID(strings.TrimSuffix(raw, "@g.us"), types.GroupServer)
	case strings.HasSuffix(raw, "@newsletter"):
		jid = types.NewJID(strings.TrimSuffix(raw, "@newsletter"), types.NewsletterServer)
	case strings.HasSuffix(raw, "@lid"):
		jid = types.NewJID(strings.TrimSuffix(raw, "@lid"), "lid")
	default:
		jid = types.NewJID(strings.TrimSuffix(raw, "@s.whatsapp.net"), types.DefaultUserServer)
	}
	if jid.User == "" || jid.Server == "" {
		return types.JID{}, errors.New("JID user or server is empty")
	}
	return jid, nil
}

func ValidateChatID(raw string) error {
	if raw == "" || len(raw) > 128 {
		return errors.New("chat_id must be non-empty and at most 128 bytes")
	}
	_, err := parseJIDStrict(raw)
	return err
}
