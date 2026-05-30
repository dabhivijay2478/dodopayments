package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"invoice-service/internal/config"
	"invoice-service/internal/db"
	"invoice-service/internal/handlers"
	"invoice-service/internal/middleware"
	"invoice-service/internal/models"
	"invoice-service/internal/payment"

	"github.com/go-chi/chi/v5"
)

func main() {
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required: copy .env.example to .env or set the variable in your environment")
	}

	if err := db.Connect(cfg.DatabaseURL); err != nil {
		log.Fatalf("database connect failed: %v", err)
	}

	if cfg.SeedData {
		var count int64
		db.DB.Model(&models.Business{}).Count(&count)
		if count == 0 {
			business := models.Business{Name: "Test Business"}
			if err := db.DB.Create(&business).Error; err != nil {
				log.Printf("seed business failed: %v", err)
			} else if _, fullKey, err := handlers.IssueAPIKey(business.ID); err != nil {
				log.Printf("seed api key failed: %v", err)
			} else {
				fmt.Printf("\n=============================\n")
				fmt.Printf("TEST API KEY: %s\n", fullKey)
				fmt.Printf("=============================\n\n")
			}
		}
	}

	payment.DefaultClient = payment.NewClient(cfg.PSPBaseURL)

	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware)

		r.Post("/api-keys", handlers.CreateAPIKey)
		r.Get("/api-keys", handlers.ListAPIKeys)
		r.Delete("/api-keys/{id}", handlers.RevokeAPIKey)

		r.Post("/customers", handlers.CreateCustomer)
		r.Get("/customers", handlers.ListCustomers)
		r.Get("/customers/{id}", handlers.GetCustomer)

		r.Post("/invoices", handlers.CreateInvoice)
		r.Get("/invoices", handlers.ListInvoices)
		r.Get("/invoices/{id}", handlers.GetInvoice)
		r.Post("/invoices/{id}/finalize", handlers.FinalizeInvoice)
		r.Post("/invoices/{id}/void", handlers.VoidInvoice)
		r.Post("/invoices/{id}/pay", handlers.PayInvoice)

		r.Post("/webhook-endpoints", handlers.CreateWebhookEndpoint)
		r.Get("/webhook-endpoints", handlers.ListWebhookEndpoints)
	})

	log.Printf("server starting on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, r))
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
