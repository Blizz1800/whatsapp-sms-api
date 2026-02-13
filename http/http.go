package http

import (
	"log"
	"main/http/handlers"
	"main/whatsapp"
	"net/http"
)

func SetupHandlers(client *whatsapp.WhatsAppClient) {
	handlers.SetClient(client)
	http.Handle("/sms", http.HandlerFunc(handlers.SmsHandler))
}

func Serve() {
	log.Fatal(http.ListenAndServe("0.0.0.0:9050", nil))
}
