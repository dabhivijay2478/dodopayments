package payment_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"invoice-service/internal/config"
	"invoice-service/internal/db"
	"invoice-service/internal/handlers"
	"invoice-service/internal/middleware"
	"invoice-service/internal/models"
	"invoice-service/internal/payment"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		t.Fatal("DATABASE_URL is required: add it to .env in the repo root (cp .env.example .env) or export DATABASE_URL before go test")
	}
	if db.DB == nil {
		if err := db.Connect(cfg.DatabaseURL); err != nil {
			t.Fatalf("connect database: %v", err)
		}
	}
	return db.DB
}

func createTestAPIKey(t *testing.T, gdb *gorm.DB, businessID uuid.UUID) string {
	t.Helper()
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatal(err)
	}
	key := "sk_" + hex.EncodeToString(keyBytes)
	hash := sha256.Sum256([]byte(key))
	if err := gdb.Create(&models.APIKey{
		BusinessID: businessID,
		KeyPrefix:  key[:11],
		KeyHash:    hex.EncodeToString(hash[:]),
	}).Error; err != nil {
		t.Fatal(err)
	}
	return key
}

func createTestInvoice(t *testing.T, gdb *gorm.DB, state string) models.Invoice {
	t.Helper()
	business := models.Business{Name: "TestBiz_" + uuid.New().String()[:8]}
	if err := gdb.Create(&business).Error; err != nil {
		t.Fatal(err)
	}
	customer := models.Customer{
		BusinessID: business.ID,
		Name:       "Test Customer",
		Email:      "test@example.com",
	}
	if err := gdb.Create(&customer).Error; err != nil {
		t.Fatal(err)
	}
	invoice := models.Invoice{
		BusinessID: business.ID,
		CustomerID: customer.ID,
		State:      state,
		DueDate:    time.Now().Add(24 * time.Hour),
		TotalCents: 2000,
	}
	if err := gdb.Create(&invoice).Error; err != nil {
		t.Fatal(err)
	}
	lineItem := models.LineItem{
		InvoiceID:       invoice.ID,
		Description:     "Test item",
		Quantity:        1,
		UnitAmountCents: 2000,
	}
	if err := gdb.Create(&lineItem).Error; err != nil {
		t.Fatal(err)
	}
	return invoice
}

func buildPayRouter(pspURL string) http.Handler {
	payment.DefaultClient = payment.NewClient(pspURL)
	r := chi.NewRouter()
	r.Use(middleware.AuthMiddleware)
	r.Post("/invoices/{id}/pay", handlers.PayInvoice)
	return r
}

func callPayEndpoint(router http.Handler, apiKey string, invoiceID uuid.UUID, cardToken, idempKey string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"card_token": cardToken})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/invoices/%s/pay", invoiceID), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}
