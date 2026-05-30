package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

type chargeRequest struct {
	CardToken   string `json:"card_token"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	InvoiceID   string `json:"invoice_id"`
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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func handleCharge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req chargeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.CardToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "card_token is required"})
		return
	}

	switch req.CardToken {
	case "tok_success":
		time.Sleep(100 * time.Millisecond)
		writeJSON(w, http.StatusOK, chargeResponse{
			Status: "succeeded",
			PSPRef: uuid.New().String(),
		})

	case "tok_insufficient_funds":
		time.Sleep(100 * time.Millisecond)
		writeJSON(w, http.StatusOK, chargeResponse{Status: "failed", Code: "insufficient_funds"})

	case "tok_card_declined":
		time.Sleep(100 * time.Millisecond)
		writeJSON(w, http.StatusOK, chargeResponse{Status: "failed", Code: "card_declined"})

	case "tok_timeout":
		time.Sleep(30 * time.Second)
		writeJSON(w, http.StatusOK, chargeResponse{
			Status: "succeeded",
			PSPRef: uuid.New().String(),
		})

	case "tok_network_error":
		// Simulated PSP outage — 500 is intentional (API maps this to 402 network_error).
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"status":  "failed",
			"code":    "network_error",
			"message": "simulated PSP network failure",
		})

	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown token"})
	}
}
