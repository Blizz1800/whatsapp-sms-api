package whatsapp

import (
	"context"
	"io"
	"log"
	"net/http"
	"strings"

	"main/http/models"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

var lastMessageId types.MessageID

func (w *WhatsAppClient) SendMessage(number string, message string) error {
	jid := types.NewJID(number, types.DefaultUserServer)

	waMessage := &waE2E.Message{
		Conversation: proto.String(message),
	}

	if err := w.waitSendTurn(w.Ctx); err != nil {
		return err
	}
	msg, _, err := sendRecipient(w.Ctx, recipientSendTimeout, func(ctx context.Context) (types.MessageID, error) {
		resp, err := w.Client.SendMessage(ctx, jid, waMessage)
		return resp.ID, err
	})
	if err != nil {
		return err
	}
	lastMessageId = msg
	w.recordSentMessage(msg)
	return nil
}

func (w *WhatsAppClient) SendMessageToJID(jidStr string, message string) error {
	jid, err := parseJID(jidStr)
	if err != nil {
		return err
	}

	waMessage := &waE2E.Message{
		Conversation: proto.String(message),
	}

	if err := w.waitSendTurn(w.Ctx); err != nil {
		return err
	}
	msg, _, err := sendRecipient(w.Ctx, recipientSendTimeout, func(ctx context.Context) (types.MessageID, error) {
		resp, err := w.Client.SendMessage(ctx, jid, waMessage)
		return resp.ID, err
	})
	if err != nil {
		return err
	}
	lastMessageId = msg
	w.recordSentMessage(msg)
	return nil
}

func (w *WhatsAppClient) SendReaction(number string, reaction string) error {
	target := types.NewJID(number, types.DefaultUserServer)
	message := w.Client.BuildReaction(target, w.Client.Store.GetJID(), lastMessageId, reaction)
	if err := w.waitSendTurn(w.Ctx); err != nil {
		return err
	}
	msg, _, err := sendRecipient(w.Ctx, recipientSendTimeout, func(ctx context.Context) (types.MessageID, error) {
		resp, err := w.Client.SendMessage(ctx, target, message)
		return resp.ID, err
	})
	if err != nil {
		return err
	}
	lastMessageId = msg
	w.recordSentMessage(msg)
	return nil
}

func parseJID(raw string) (types.JID, error) {
	raw = strings.TrimPrefix(raw, "+")
	if strings.HasSuffix(raw, "@g.us") {
		return types.NewJID(strings.TrimSuffix(raw, "@g.us"), types.GroupServer), nil
	}
	if strings.HasSuffix(raw, "@newsletter") {
		return types.NewJID(strings.TrimSuffix(raw, "@newsletter"), types.NewsletterServer), nil
	}
	if strings.HasSuffix(raw, "@lid") {
		return types.NewJID(strings.TrimSuffix(raw, "@lid"), "lid"), nil
	}
	number := strings.TrimSuffix(raw, "@s.whatsapp.net")
	return types.NewJID(number, types.DefaultUserServer), nil
}

func (w *WhatsAppClient) ForwardMessage(req models.ForwardRequest) []models.ForwardResult {
	results := make([]models.ForwardResult, 0, len(req.Recipients))

	var imageBytes []byte
	var imageMimeType string
	var imageCaption string

	if req.Image != nil && req.Image.URL != "" {
		resp, err := http.Get(req.Image.URL)
		if err != nil {
			for _, recipient := range req.Recipients {
				results = append(results, models.ForwardResult{
					Recipient: recipient,
					Success:   false,
					Error:     "Failed to download image: " + err.Error(),
				})
			}
			return results
		}
		defer resp.Body.Close()

		imageBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			for _, recipient := range req.Recipients {
				results = append(results, models.ForwardResult{
					Recipient: recipient,
					Success:   false,
					Error:     "Failed to read image: " + err.Error(),
				})
			}
			return results
		}

		imageMimeType = req.Image.MimeType
		if imageMimeType == "" {
			imageMimeType = resp.Header.Get("Content-Type")
			if imageMimeType == "" {
				imageMimeType = "image/png"
			}
		}

		imageCaption = req.Image.Caption
	}

	for _, recipient := range req.Recipients {
		result := models.ForwardResult{Recipient: recipient}

		jid, err := parseJID(recipient)
		if err != nil {
			result.Success = false
			result.Error = "Invalid recipient JID: " + err.Error()
			results = append(results, result)
			continue
		}

		if imageBytes != nil {
			uploadResp, err := w.Client.Upload(w.Ctx, imageBytes, whatsmeow.MediaImage)
			if err != nil {
				result.Success = false
				result.Error = "Failed to upload image: " + err.Error()
				results = append(results, result)
				continue
			}

			msgText := req.Message
			if msgText == "" {
				msgText = imageCaption
			} else if imageCaption != "" {
				msgText = req.Message + "\n" + imageCaption
			}

			var waMessage *waE2E.Message
			if isNewsletter(jid) {
				waMessage = &waE2E.Message{
					ImageMessage: &waE2E.ImageMessage{
						URL:           &uploadResp.URL,
						DirectPath:    &uploadResp.DirectPath,
						MediaKey:      uploadResp.MediaKey,
						FileEncSHA256: uploadResp.FileEncSHA256,
						FileSHA256:    uploadResp.FileSHA256,
						FileLength:    &uploadResp.FileLength,
						Mimetype:      proto.String(imageMimeType),
						Caption:       proto.String(msgText),
					},
				}
			} else {
				waMessage = &waE2E.Message{
					ImageMessage: &waE2E.ImageMessage{
						URL:           &uploadResp.URL,
						DirectPath:    &uploadResp.DirectPath,
						MediaKey:      uploadResp.MediaKey,
						FileEncSHA256: uploadResp.FileEncSHA256,
						FileSHA256:    uploadResp.FileSHA256,
						FileLength:    &uploadResp.FileLength,
						Mimetype:      proto.String(imageMimeType),
						Caption:       proto.String(msgText),
						ContextInfo: &waE2E.ContextInfo{
							IsForwarded:     proto.Bool(true),
							ForwardingScore: proto.Uint32(1),
						},
					},
				}
			}

			if err := w.waitSendTurn(w.Ctx); err != nil {
				result.Success = false
				result.Error = err.Error()
				results = append(results, result)
				continue
			}
			sentMsg, err := w.Client.SendMessage(w.Ctx, jid, waMessage)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
			} else {
				w.recordSentMessage(sentMsg.ID)
				result.Success = true
			}
		} else {
			var waMessage *waE2E.Message
			if isNewsletter(jid) {
				waMessage = &waE2E.Message{
					Conversation: proto.String(req.Message),
				}
			} else {
				waMessage = &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: proto.String(req.Message),
						ContextInfo: &waE2E.ContextInfo{
							IsForwarded:     proto.Bool(true),
							ForwardingScore: proto.Uint32(1),
						},
					},
				}
			}

			if err := w.waitSendTurn(w.Ctx); err != nil {
				result.Success = false
				result.Error = err.Error()
				results = append(results, result)
				continue
			}
			sentMsg, err := w.Client.SendMessage(w.Ctx, jid, waMessage)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
			} else {
				w.recordSentMessage(sentMsg.ID)
				result.Success = true
			}
		}

		results = append(results, result)
	}

	return results
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
