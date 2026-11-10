package whatsapp

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"main/config"
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
	UpdateID           int64            `json:"update_id,omitempty"`
	ID                 string           `json:"id"`
	From               string           `json:"from"`
	FromPN             string           `json:"from_pn,omitempty"`
	Chat               string           `json:"chat,omitempty"`
	Text               string           `json:"text"`
	IsFromMe           bool             `json:"is_from_me"`
	MultimediaType     MultimediaType   `json:"multimedia_type,omitempty"`
	MultimediaCaption  string           `json:"multimedia_caption,omitempty"`
}

type forwardableMessage struct {
	senderLID          string
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
	SendGate *SendGate
	groupCache GroupCache

	mu                  sync.RWMutex
	receivedMessages    []ReceivedMessage
	nextUpdateID        int64
	updatesBufferSize   int
	forwardableMessages map[string]*forwardableMessage
	sentMessageIDs      map[string]bool
	recentSelf          map[string]time.Time

	Connected chan struct{}

	clientMu       sync.RWMutex
	loginReady     chan struct{}
	loginReadyOnce sync.Once
	connectedOnce  sync.Once
	pairMu         sync.Mutex
}

const selfDedupWindow = 30 * time.Second

func NewWhatsAppClient() *WhatsAppClient {
	cfg := config.Load()
	buf := cfg.UpdatesBufferSize
	if buf < 100 {
		buf = 100
	}
	return &WhatsAppClient{
		Connected:           make(chan struct{}),
		loginReady:          make(chan struct{}),
		SendGate:            NewSendGate(cfg.SendRatePerMinute, 250*time.Millisecond),
		groupCache:          NewMemoryGroupCache(time.Hour),
		updatesBufferSize:   buf,
		forwardableMessages: make(map[string]*forwardableMessage),
		sentMessageIDs:      make(map[string]bool),
		recentSelf:          make(map[string]time.Time),
	}
}

// waitSendTurn espera el turno del rate limiter compartido. Nunca bloquea
// más allá del contexto; si la puerta no está inicializada, pasa directo.
func (w *WhatsAppClient) waitSendTurn(ctx context.Context) error {
	if w.SendGate == nil {
		return nil
	}
	return w.SendGate.Wait(ctx)
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

// appendReceivedMessage assigns a monotonic update_id, appends the message to
// the received buffer, trims the oldest entries to the buffer size, and
// returns the message with its update_id populated. It must be called with
// w.mu held by the caller? No: it locks internally.
func (w *WhatsAppClient) appendReceivedMessage(rm ReceivedMessage) ReceivedMessage {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.nextUpdateID++
	rm.UpdateID = w.nextUpdateID
	w.receivedMessages = append(w.receivedMessages, rm)
	if w.updatesBufferSize > 0 && len(w.receivedMessages) > w.updatesBufferSize {
		trim := len(w.receivedMessages) - w.updatesBufferSize
		w.receivedMessages = w.receivedMessages[trim:]
	}
	return rm
}

// GetUpdates returns received messages with update_id > offset, capped at
// limit (clamped to 1..500). If timeout > 0 it polls every 100 ms until new
// messages appear or timeout/ctx cancellation. timeout 0 returns immediately.
func (w *WhatsAppClient) GetUpdates(ctx context.Context, offset int64, limit int, timeout time.Duration) []Update {
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	poll := 100 * time.Millisecond
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		for {
			got := w.updatesSnapshot(offset, limit)
			if len(got) > 0 {
				return got
			}
			select {
			case <-ctx.Done():
				return nil
			case <-timer.C:
				return nil
			case <-time.After(poll):
			}
		}
	}
	return w.updatesSnapshot(offset, limit)
}

func (w *WhatsAppClient) updatesSnapshot(offset int64, limit int) []Update {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []Update
	for _, rm := range w.receivedMessages {
		if rm.UpdateID > offset {
			out = append(out, Update{UpdateID: rm.UpdateID, Message: rm})
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

// Update wraps a received message with its monotonic update_id.
type Update struct {
	UpdateID int64           `json:"update_id"`
	Message  ReceivedMessage `json:"message"`
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

		rm = w.appendReceivedMessage(rm)
		// Always store forwardable data keyed by ID
		w.mu.Lock()
		if w.forwardableMessages == nil {
			w.forwardableMessages = make(map[string]*forwardableMessage)
		}
		if fm == nil {
			fm = &forwardableMessage{text: text}
		}
		fm.senderLID = sender
		w.forwardableMessages[v.Info.ID] = fm
		w.mu.Unlock()

		fmt.Printf("📩 Message from %s: %s\n", sender, text)
	}
}

// ProtectMessage persists the forwardable data of a message (found in memory
// or already saved) so it survives API restarts and is kept by the cleanup job.
func (w *WhatsAppClient) ProtectMessage(id string) bool {
	w.mu.RLock()
	fm, ok := w.forwardableMessages[id]
	w.mu.RUnlock()

	if !ok {
		fm = LoadForwardableMessage(id)
		if fm == nil {
			return false
		}
		return SaveForwardableMessage(id, fm.senderLID, fm)
	}

	return SaveForwardableMessage(id, fm.senderLID, fm)
}

// UnprotectMessage removes the persisted row of a message whose schedule is done.
func (w *WhatsAppClient) UnprotectMessage(id string) bool {
	DeleteForwardableMessage(id)
	return true
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
			results = append(results, w.sendToRecipient(recipient, jid, waMessage))
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

		results = append(results, w.sendToRecipient(recipient, jid, waMessage))
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
	w.mu.RLock()
	cache := w.groupCache
	w.mu.RUnlock()

	if cache != nil {
		if groups, hit, err := cache.Get(context.Background()); err == nil && hit {
			return groups, nil
		}
	}

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

	w.mu.RLock()
	cache = w.groupCache
	w.mu.RUnlock()
	if cache != nil {
		_ = cache.Set(context.Background(), items)
	}

	return items, nil
}

// SetGroupCache inyecta una implementación de GroupCache (por ejemplo Redis).
// Se llama antes de usar GetGroupsAndNewsletters; si es nil, no se cachea.
func (w *WhatsAppClient) SetGroupCache(cache GroupCache) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.groupCache = cache
}

func (w *WhatsAppClient) Identity() (id, phone, name string, ok bool) {
	if !w.IsConnected() || w.Client == nil || w.Client.Store.ID == nil {
		return "", "", "", false
	}
	jid := w.Client.Store.ID.ToNonAD()
	return jid.String(), jid.User, w.Client.Store.PushName, true
}

func (w *WhatsAppClient) Connect() {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	w.Ctx = context.Background()
	cfg := config.Load()
	container, err := sqlstore.New(w.Ctx, cfg.Driver(), cfg.DSN(), dbLog)
	if err != nil {
		panic(err)
	}
	// If you want multiple sessions, remember their JIDs and use .GetDevice(jid) or .GetAllDevices() instead.
	deviceStore, err := container.GetFirstDevice(w.Ctx)
	if err != nil {
		panic(err)
	}
	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	w.setClient(client)
	client.AddEventHandler(w.EventHandler)

	if client.Store.ID != nil {
		// Already logged in, just connect
		if err := client.Connect(); err != nil {
			panic(err)
		}
		w.markLoginReady()
		w.markConnected()
		return
	}

	// No ID stored, start a login session. If the QR codes expire before the
	// account is linked, start a new session so the QR/pairing code stays
	// available while the service is running.
	for {
		qrChan, err := client.GetQRChannel(context.Background())
		if err != nil {
			panic(err)
		}
		if err := client.Connect(); err != nil {
			panic(err)
		}

		for evt := range qrChan {
			if evt.Event == whatsmeow.QRChannelEventCode {
				w.markLoginReady()
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
				continue
			}

			fmt.Println("Login event:", evt.Event)
			if evt.Event == whatsmeow.QRChannelSuccess.Event {
				w.markLoginReady()
				w.markConnected()
			}
		}

		if client.Store.ID != nil {
			return
		}
		fmt.Println("QR login session ended, generating a new one...")
		time.Sleep(5 * time.Second)
	}
}

func (w *WhatsAppClient) Disconnect() {
	// Disconnect the client when done
	w.Client.Disconnect()
}

