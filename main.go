package main

import (
	"fmt"
	"main/config"
	"main/http"
	"main/whatsapp"
)

func main() {
	config.Load()
	fmt.Println("Starting connect the WhatsappApi")
	whatsapp.InitDB()
	whatsapp.StartCleanupLoop()
	whatsAppClient := whatsapp.NewWhatsAppClient()
	http.SetupHandlers(whatsAppClient)
	fmt.Println("HTTP handlers set up successfully")
	go whatsAppClient.Connect()
	http.Serve()
}
