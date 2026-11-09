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
	http.Handle("/pair", http.HandlerFunc(handlers.PairHandler))
	http.Handle("/inbox", http.HandlerFunc(handlers.InboxHandler))
	http.Handle("/message", http.HandlerFunc(handlers.MessageHandler))
	http.Handle("/message/protect", http.HandlerFunc(handlers.ProtectMessageHandler))
	http.Handle("/message/unprotect", http.HandlerFunc(handlers.UnprotectMessageHandler))
	http.Handle("/forward_received", http.HandlerFunc(handlers.ForwardReceivedHandler))
	http.Handle("/groups", http.HandlerFunc(handlers.GroupsHandler))
	http.HandleFunc("POST /messages", handlers.CreateMessageHandler)
	http.HandleFunc("GET /jobs/{id}", handlers.GetJobHandler)
	http.HandleFunc("GET /jobs", handlers.ListJobsHandler)
	http.HandleFunc("POST /jobs/{id}/retry", handlers.RetryJobHandler)
	http.HandleFunc("POST /sendMessage", handlers.SendMessageSyncHandler)
	http.HandleFunc("GET /getMe", handlers.GetMeHandler)
	http.HandleFunc("GET /getChats", handlers.GetChatsHandler)
	http.HandleFunc("GET /getUpdates", handlers.GetUpdatesHandler)
	http.HandleFunc("GET /stats", handlers.StatsHandler)
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
