package main

import (
	"fmt"
	"main/config"
	"main/http"
	"main/whatsapp"
	"time"
)

func main() {
	config.Load()
	fmt.Println("Starting connect the WhatsappApi")
	whatsapp.InitDB()
	whatsapp.StartCleanupLoop()
	whatsAppClient := whatsapp.NewWhatsAppClient()
	go whatsAppClient.Connect()

	// Wait for WhatsApp connection (with timeout)
	select {
	case <-whatsAppClient.Connected:
		fmt.Println("WhatsappApi connected successfully")
	case <-time.After(30 * time.Second):
		fmt.Println("Warning: Timeout waiting for WhatsApp connection, starting server anyway")
	}

	http.SetupHandlers(whatsAppClient)
	fmt.Println("HTTP handlers set up successfully")
	http.Serve()
}
