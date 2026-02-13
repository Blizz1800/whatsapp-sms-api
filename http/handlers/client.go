package handlers

import "main/whatsapp"

var whatAppClient *whatsapp.WhatsAppClient

func SetClient(client *whatsapp.WhatsAppClient) {
	whatAppClient = client
}
