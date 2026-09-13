package whatsapp

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"main/http/models"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mdp/qrterminal"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type MultimediaType string

const (
	MultimediaNone     MultimediaType = ""
	MultimediaImage    MultimediaType = "image"
	MultimediaVideo    MultimediaType = "video"
	MultimediaAudio    MultimediaType = "audio"
	MultimediaDocument MultimediaType = "document"
)

type ReceivedMessage struct {
	ID                string         `json:"id"`
	From              string         `json:"from"`
	FromPN            string         `json:"from_pn,omitempty"`
	Chat              string         `json:"chat,omitempty"`
	Text              string         `json:"text"`
	IsFromMe          bool           `json:"is_from_me"`
	MultimediaType    MultimediaType `json:"multimedia_type,omitempty"`
	MultimediaCaption string         `json:"multimedia_caption,omitempty"`
}

type forwardableMessage struct {
	text               string
	imageURL           string
	imageDirectPath    string
	imageMediaKey      []byte
	imageFileEncSHA256 []byte
	imageFileSHA256    []byte
	imageFileLength    uint64
	imageMimeType      string
	imageCaption       string
	imageJPEGThumbnail []byte
	imageHeight        uint32
	imageWidth         uint32
}

func getMessageFields(v *events.Message) (text string, fm *forwardableMessage) {
	text = v.Message.GetConversation()
	if text == "" && v.Message.GetExtendedTextMessage() != nil {
		text = v.Message.GetExtendedTextMessage().GetText()
	}

	if img := v.Message.GetImageMessage(); img != nil {
		fm = &forwardableMessage{
			imageURL:           img.GetURL(),
			imageDirectPath:    img.GetDirectPath(),
			imageMediaKey:      img.GetMediaKey(),
			imageFileEncSHA256: img.GetFileEncSHA256(),
			imageFileSHA256:    img.GetFileSHA256(),
			imageFileLength:    img.GetFileLength(),
			imageMimeType:      img.GetMimetype(),
			imageCaption:       img.GetCaption(),
			imageJPEGThumbnail: img.GetJPEGThumbnail(),
			imageHeight:        img.GetHeight(),
			imageWidth:         img.GetWidth(),
		}
		if text == "" {
			text = img.GetCaption()
		}
	}

	return text, fm
}

func (fm *forwardableMessage) buildMessage() *waE2E.Message {
	msgText := fm.text
	if msgText == "" {
		msgText = fm.imageCaption
	} else if fm.imageCaption != "" {
		msgText = fm.text + "\n" + fm.imageCaption
	}

	if fm.imageURL != "" {
		return &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				URL:           &fm.imageURL,
				DirectPath:    &fm.imageDirectPath,
				MediaKey:      fm.imageMediaKey,
				FileEncSHA256: fm.imageFileEncSHA256,
				FileSHA256:    fm.imageFileSHA256,
				FileLength:    &fm.imageFileLength,
				Mimetype:      proto.String(fm.imageMimeType),
				Caption:       proto.String(msgText),
				JPEGThumbnail: fm.imageJPEGThumbnail,
				Height:        &fm.imageHeight,
				Width:         &fm.imageWidth,
				ContextInfo:   &waE2E.ContextInfo{},
			},
		}
	} else if fm.text != "" {
		return &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:        proto.String(fm.text),
				ContextInfo: &waE2E.ContextInfo{},
			},
		}
	}
	return nil
}

func (fm *forwardableMessage) buildNewsletterMessage() *waE2E.Message {
	msgText := fm.text
	if msgText == "" {
		msgText = fm.imageCaption
	} else if fm.imageCaption != "" {
		msgText = fm.text + "\n" + fm.imageCaption
	}

	if fm.imageURL != "" {
		return &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				URL:           &fm.imageURL,
				DirectPath:    &fm.imageDirectPath,
				MediaKey:      fm.imageMediaKey,
				FileEncSHA256: fm.imageFileEncSHA256,
				FileSHA256:    fm.imageFileSHA256,
				FileLength:    &fm.imageFileLength,
				Mimetype:      proto.String(fm.imageMimeType),
				Caption:       proto.String(msgText),
				JPEGThumbnail: fm.imageJPEGThumbnail,
				Height:        &fm.imageHeight,
				Width:         &fm.imageWidth,
			},
		}
	} else if fm.text != "" {
		return &waE2E.Message{
			Conversation: proto.String(fm.text),
		}
	}
	return nil
}

func (w *WhatsAppClient) getSenderPN(v *events.Message) (string, string) {
	sender := v.Info.Sender
	fromStr := sender.ToNonAD().String()
	fromPN := ""

	if v.Info.AddressingMode == types.AddressingModeLID {
		if !v.Info.SenderAlt.IsEmpty() {
			fromPN = v.Info.SenderAlt.ToNonAD().String()
		}
	}

	if v.Info.IsGroup && !sender.IsEmpty() {
		return fromStr, fromPN
	}

	target := v.Info.Chat
	return target.ToNonAD().String(), fromPN
}

type WhatsAppClient struct {
	Client *whatsmeow.Client
	Ctx    context.Context

	mu                  sync.RWMutex
	receivedMessages    []ReceivedMessage
	forwardableMessages map[string]*forwardableMessage
	sentMessageIDs      map[string]bool
	recentSelf          map[string]time.Time

	Connected chan struct{}
}

const selfDedupWindow = 30 * time.Second

func NewWhatsAppClient() *WhatsAppClient {
	return &WhatsAppClient{
		Connected:           make(chan struct{}),
		forwardableMessages: make(map[string]*forwardableMessage),
		sentMessageIDs:      make(map[string]bool),
		recentSelf:          make(map[string]time.Time),
	}
}

func (w *WhatsAppClient) recordSentMessage(id string) {
	if id == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.sentMessageIDs) > 5000 {
		w.sentMessageIDs = make(map[string]bool)
	}
	w.sentMessageIDs[id] = true
}

func (w *WhatsAppClient) wasSentByBot(id string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.sentMessageIDs[id]
}

func (w *WhatsAppClient) GetReceivedMessages() []ReceivedMessage {
	w.mu.RLock()
	defer w.mu.RUnlock()
	result := make([]ReceivedMessage, len(w.receivedMessages))
	copy(result, w.receivedMessages)
	return result
}

func (w *WhatsAppClient) EventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		// Ignorar SOLO los mensajes que el propio bot envió (sus respuestas).
		// Los mensajes que el usuario escribe manualmente desde su número
		// (incluido el chat consigo mismo) sí deben procesarse.
		// if v.Info.IsFromMe && w.wasSentByBot(v.Info.ID) {
		// 	return
		// }

		sender, senderPN := w.getSenderPN(v)
		text, fm := getMessageFields(v)

		multimediaType := MultimediaNone
		if fm != nil && fm.imageURL != "" {
			multimediaType = MultimediaImage
		}

		// Store in received message list
		chatJID := v.Info.Chat.ToNonAD().String()
		if v.Info.IsFromMe {
			now := time.Now()
			key := sender + "|" + chatJID + "|" + text
			w.mu.Lock()
			if last, ok := w.recentSelf[key]; ok && now.Sub(last) < selfDedupWindow {
				w.mu.Unlock()
				return
			}
			w.recentSelf[key] = now
			w.mu.Unlock()
		}
		rm := ReceivedMessage{
			ID:             v.Info.ID,
			From:           sender,
			FromPN:         senderPN,
			Chat:           chatJID,
			Text:           text,
			IsFromMe:       v.Info.IsFromMe,
			MultimediaType: multimediaType,
		}
		if fm != nil {
			rm.MultimediaCaption = fm.imageCaption
		}

		w.mu.Lock()
		w.receivedMessages = append(w.receivedMessages, rm)
		// Always store forwardable data keyed by ID
		if w.forwardableMessages == nil {
			w.forwardableMessages = make(map[string]*forwardableMessage)
		}
		if fm == nil {
			fm = &forwardableMessage{text: text}
		}
		w.forwardableMessages[v.Info.ID] = fm
		SaveForwardableMessage(v.Info.ID, sender, fm)
		w.mu.Unlock()

		fmt.Printf("📩 Message from %s: %s\n", sender, text)
	}
}

func isNewsletter(jid types.JID) bool {
	return jid.Server == types.NewsletterServer
}

func (w *WhatsAppClient) ForwardReceivedMessage(id string, recipients []string, captionOverride *string) []models.ForwardResult {
	results := make([]models.ForwardResult, 0, len(recipients))

	w.mu.RLock()
	fm, ok := w.forwardableMessages[id]
	w.mu.RUnlock()

	// Try loading from database if not in memory
	if !ok {
		fm = LoadForwardableMessage(id)
		if fm != nil {
			ok = true
		}
	}

	// Apply caption override for image messages if provided
	if ok && captionOverride != nil && fm.imageURL != "" {
		clone := *fm
		clone.imageCaption = *captionOverride
		fm = &clone
	}

	// If not found as forwardable, treat as text-only
	if !ok {
		for _, recipient := range recipients {
			text := ""
			for _, rm := range w.receivedMessages {
				if rm.ID == id {
					text = rm.Text
					break
				}
			}
			jid, err := parseJID(recipient)
			if err != nil {
				results = append(results, models.ForwardResult{Recipient: recipient, Success: false, Error: err.Error()})
				continue
			}
			var waMessage *waE2E.Message
			if isNewsletter(jid) {
				waMessage = &waE2E.Message{Conversation: proto.String(text)}
			} else {
				waMessage = &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text:        proto.String(text),
						ContextInfo: &waE2E.ContextInfo{},
					},
				}
			}
			sentMsg, err := w.Client.SendMessage(w.Ctx, jid, waMessage)
			if err != nil {
				results = append(results, models.ForwardResult{Recipient: recipient, Success: false, Error: err.Error()})
			} else {
				w.recordSentMessage(sentMsg.ID)
				results = append(results, models.ForwardResult{Recipient: recipient, Success: true})
			}
		}
		return results
	}

	for _, recipient := range recipients {
		jid, err := parseJID(recipient)
		if err != nil {
			results = append(results, models.ForwardResult{Recipient: recipient, Success: false, Error: err.Error()})
			continue
		}

		var waMessage *waE2E.Message
		if isNewsletter(jid) {
			waMessage = fm.buildNewsletterMessage()
		} else {
			waMessage = fm.buildMessage()
		}
		if waMessage == nil {
			results = append(results, models.ForwardResult{Recipient: recipient, Success: false, Error: "Empty message"})
			continue
		}

		sentMsg, err := w.Client.SendMessage(w.Ctx, jid, waMessage)
		if err != nil {
			results = append(results, models.ForwardResult{Recipient: recipient, Success: false, Error: err.Error()})
		} else {
			w.recordSentMessage(sentMsg.ID)
			results = append(results, models.ForwardResult{Recipient: recipient, Success: true})
		}
	}
	return results
}

// canWriteGroup reports whether the bot is allowed to send messages to the group.
func canWriteGroup(g *types.GroupInfo, botJID types.JID) bool {
	if !g.IsAnnounce {
		return true
	}
	for _, p := range g.Participants {
		if p.JID.ToNonAD() == botJID && (p.IsAdmin || p.IsSuperAdmin) {
			return true
		}
	}
	return false
}

func (w *WhatsAppClient) GetGroupsAndNewsletters() ([]models.GroupItem, error) {
	var items []models.GroupItem
	index := 1

	groups, err := w.Client.GetJoinedGroups(w.Ctx)
	if err == nil {
		botJID := w.Client.Store.ID.ToNonAD()
		// Sort by group creation date (immutable) so the index order never changes.
		sort.SliceStable(groups, func(i, j int) bool {
			return groups[i].GroupCreated.Before(groups[j].GroupCreated)
		})
		for _, g := range groups {
			if !canWriteGroup(g, botJID) {
				continue
			}
			items = append(items, models.GroupItem{
				Index: index,
				Name:  g.Name,
				JID:   g.JID.String(),
				Type:  "group",
			})
			index++
		}
	}

	newsletters, err := w.Client.GetSubscribedNewsletters(w.Ctx)
	if err == nil {
		// Sort by JID (stable) so channel order also never changes.
		sort.Slice(newsletters, func(i, j int) bool {
			return newsletters[i].ID.String() < newsletters[j].ID.String()
		})
		for _, n := range newsletters {
			if n.ViewerMeta == nil || (n.ViewerMeta.Role != types.NewsletterRoleAdmin && n.ViewerMeta.Role != types.NewsletterRoleOwner) {
				continue
			}
			name := n.ThreadMeta.Name.Text
			if name == "" {
				name = "Canal sin nombre"
			}
			items = append(items, models.GroupItem{
				Index: index,
				Name:  name,
				JID:   n.ID.String(),
				Type:  "channel",
			})
			index++
		}
	}

	return items, nil
}

func (w *WhatsAppClient) Connect() {

	dbLog := waLog.Stdout("Database", "DEBUG", true)
	w.Ctx = context.Background()
	container, err := sqlstore.New(w.Ctx, "pgx", os.Getenv("DATABASE_URL"), dbLog)
	if err != nil {
		panic(err)
	}
	// If you want multiple sessions, remember their JIDs and use .GetDevice(jid) or .GetAllDevices() instead.
	deviceStore, err := container.GetFirstDevice(w.Ctx)
	if err != nil {
		panic(err)
	}
	clientLog := waLog.Stdout("Client", "INFO", true)
	w.Client = whatsmeow.NewClient(deviceStore, clientLog)
	w.Client.AddEventHandler(w.EventHandler)

	if w.Client.Store.ID == nil {
		// No ID stored, new login
		qrChan, _ := w.Client.GetQRChannel(context.Background())
		err = w.Client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// Render the QR code here
				// e.g. qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				// or just manually `echo 2@... | qrencode -t ansiutf8` in a terminal
				fmt.Println("QR code:", evt.Code)
				fmt.Println("QR code recibido, generando imagen...")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				err := qrcode.WriteFile(evt.Code, qrcode.Medium, 256, "whatsapp-qr.png")
				if err != nil {
					fmt.Println("Error generando QR:", err)
				} else {
					fmt.Println("QR guardado como whatsapp-qr.png")
				}
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
		close(w.Connected)
	} else {
		// Already logged in, just connect
		err = w.Client.Connect()
		if err != nil {
			panic(err)
		}
		close(w.Connected)
	}
}

func (w *WhatsAppClient) Disconnect() {
	// Disconnect the client when done
	w.Client.Disconnect()
}
