package whatsapp

import (
	"context"
	"log"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"main/http/models"
)

// recipientSendTimeout limita cuánto puede tardar el envío a un solo
// destinatario. whatsmeow espera hasta 75s por defecto; con 20 grupos eso
// hace que un grupo colgado haga superar el timeout de 120s del bot.
const recipientSendTimeout = 15 * time.Second

// sendRecipient ejecuta send con un timeout propio y devuelve el id, la
// duración real y el error. El timeout cubre también la preparación/cifrado,
// no solo la espera de la respuesta.
func sendRecipient(ctx context.Context, timeout time.Duration, send func(context.Context) (types.MessageID, error)) (types.MessageID, time.Duration, error) {
	start := time.Now()
	sendCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	id, err := send(sendCtx)
	return id, time.Since(start), err
}

// sendToRecipient envía a un destinatario con timeout y deja un log por
// destinatario para poder identificar grupos que fallan o tardan demasiado.
func (w *WhatsAppClient) sendToRecipient(recipient string, jid types.JID, msg *waE2E.Message) models.ForwardResult {
	id, elapsed, err := sendRecipient(w.Ctx, recipientSendTimeout, func(ctx context.Context) (types.MessageID, error) {
		resp, err := w.Client.SendMessage(ctx, jid, msg)
		return resp.ID, err
	})
	if err != nil {
		log.Printf("forward_received: recipient=%s ok=false duration=%s error=%v", recipient, elapsed, err)
		return models.ForwardResult{Recipient: recipient, Success: false, Error: err.Error()}
	}

	log.Printf("forward_received: recipient=%s ok=true duration=%s", recipient, elapsed)
	w.recordSentMessage(id)
	return models.ForwardResult{Recipient: recipient, Success: true}
}
