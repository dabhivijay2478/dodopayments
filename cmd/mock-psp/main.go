package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

type chargeRequest struct {
	CardToken   string `json:"card_token"`
	AmountCents int64  `json:"amount_cents"`
}

type chargeResponse struct {
	Status string `json:"status"`
	PSPRef string `json:"psp_ref,omitempty"`
	Code   string `json:"code,omitempty"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/charge", handleCharge)

	log.Printf("mock PSP listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func handleCharge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chargeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.CardToken {
	case "tok_success":
		time.Sleep(100 * time.Millisecond)
		json.NewEncoder(w).Encode(chargeResponse{Status: "succeeded", PSPRef: ""})

	case "tok_insufficient_funds":
		time.Sleep(100 * time.Millisecond)
		json.NewEncoder(w).Encode(chargeResponse{Status: "failed", Code: "insufficient_funds"})

	case "tok_card_declined":
		time.Sleep(100 * time.Millisecond)
		json.NewEncoder(w).Encode(chargeResponse{Status: "failed", Code: "card_declined"})

	case "tok_timeout":
		time.Sleep(30 * time.Second)
		json.NewEncoder(w).Encode(chargeResponse{Status: "succeeded", PSPRef: ""})

	case "tok_network_error":
		w.WriteHeader(http.StatusInternalServerError)

	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "unknown token"})
	}
}
