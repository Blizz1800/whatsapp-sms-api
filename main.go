package main

import (
	"fmt"
	"main/http"
	"main/whatsapp"
)

func main() {
	fmt.Println("Starting connect the WhatsappApi")
	whatsAppClient := &whatsapp.WhatsAppClient{}
	go whatsAppClient.Connect()
	fmt.Println("WhatsappApi connected successfully")

	http.SetupHandlers(whatsAppClient)
	fmt.Println("HTTP handlers set up successfully")
	http.Serve()
}
