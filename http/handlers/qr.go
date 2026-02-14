package handlers

import (
	"net/http"
	"os"
)

// Ruta donde se guarda el QR (debe coincidir con la usada en whatsapp.Connect)
const qrFilePath = "whatsapp-qr.png"

func QRHandler(w http.ResponseWriter, r *http.Request) {
	// Verificar si el archivo existe
	if _, err := os.Stat(qrFilePath); os.IsNotExist(err) {
		http.Error(w, "QR no disponible", http.StatusNotFound)
		return
	}

	// Leer el archivo
	data, err := os.ReadFile(qrFilePath)
	if err != nil {
		http.Error(w, "Error leyendo QR", http.StatusInternalServerError)
		return
	}

	// Servir la imagen
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
