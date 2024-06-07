package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func main() {
	router := http.NewServeMux()
	router.HandleFunc("/payment/webhook", handlePaymentWebhook)
	log.Println("Starting server on :3000")
	if err := http.ListenAndServe(":3000", router); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

type WebhookRequest struct {
	Amount  int    `json:"amount"`
	Message string `json:"message"`
}

func handlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req WebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	fmt.Printf("Received webhook data: %+v\n", req)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Webhook received successfully\n"))
}
