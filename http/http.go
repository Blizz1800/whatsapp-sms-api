package http

import (
	"fmt"
	"log"
	"main/http/handlers"
	"main/whatsapp"
	"net/http"
	"os"
)

func SetupHandlers(client *whatsapp.WhatsAppClient) {
	handlers.SetClient(client)
	http.Handle("/sms", http.HandlerFunc(handlers.SmsHandler))
	http.Handle("/forward", http.HandlerFunc(handlers.ForwardHandler))
	http.Handle("/reaction", http.HandlerFunc(handlers.ReactionHandler))
	http.Handle("/qr", http.HandlerFunc(handlers.QRHandler))
	http.Handle("/inbox", http.HandlerFunc(handlers.InboxHandler))
	http.Handle("/message", http.HandlerFunc(handlers.MessageHandler))
	http.Handle("/message/protect", http.HandlerFunc(handlers.ProtectMessageHandler))
	http.Handle("/message/unprotect", http.HandlerFunc(handlers.UnprotectMessageHandler))
	http.Handle("/forward_received", http.HandlerFunc(handlers.ForwardReceivedHandler))
	http.Handle("/groups", http.HandlerFunc(handlers.GroupsHandler))
}

func Serve() {
	port := os.Getenv("PORT")
	if len(port) == 0 {
		port = "9050"
	}
	address := fmt.Sprintf("0.0.0.0:%v", port)
	log.Default().Printf("Starting server on %s", address)
	log.Fatal(http.ListenAndServe(address, nil))
}
