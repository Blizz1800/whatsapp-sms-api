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
	http.Handle("/qr", http.HandlerFunc(handlers.QRHandler))
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
