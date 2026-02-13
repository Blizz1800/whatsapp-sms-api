package whatsapp

import (
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func (w *WhatsAppClient) SendMessage(number string, message string) error {
	jid := types.NewJID(number, types.DefaultUserServer)

	waMessage := &waE2E.Message{
		Conversation: proto.String(message),
	}

	_, err := w.Client.SendMessage(w.Ctx, jid, waMessage)
	return err
}
