package payment_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"invoice-service/internal/db"
	"invoice-service/internal/models"

	"github.com/google/uuid"
)

// TestIdempotency retries the same pay request with the same Idempotency-Key
// and asserts:
//   - Both responses return 200 with identical bodies
//   - The PSP is called exactly once (no double-charge)
//   - Only 1 payment_attempt row exists for that key
func TestIdempotency(t *testing.T) {
	gdb := setupTestDB(t)
	invoice := createTestInvoice(t, gdb, "open")
	apiKey := createTestAPIKey(t, gdb, invoice.BusinessID)

	var pspCalls atomic.Int32
	mockPSP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pspCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "succeeded", "psp_ref": "psp-ref-fixed"})
	}))
	defer mockPSP.Close()

	router := buildPayRouter(mockPSP.URL)
	idempKey := "idem-test-" + uuid.New().String()

	resp1 := callPayEndpoint(router, apiKey, invoice.ID, "tok_success", idempKey)
	resp2 := callPayEndpoint(router, apiKey, invoice.ID, "tok_success", idempKey)

	if resp1.Code != http.StatusOK {
		t.Fatalf("first call: expected 200, got %d — body: %s", resp1.Code, resp1.Body.String())
	}
	if resp2.Code != http.StatusOK {
		t.Fatalf("second call: expected 200, got %d — body: %s", resp2.Code, resp2.Body.String())
	}
	if resp1.Body.String() != resp2.Body.String() {
		t.Fatalf("response bodies differ:\n  first:  %s\n  second: %s", resp1.Body.String(), resp2.Body.String())
	}

	var count int64
	gdb.Model(&models.PaymentAttempt{}).Where("idempotency_key = ?", idempKey).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 payment_attempt row, got %d", count)
	}

	if pspCalls.Load() != 1 {
		t.Fatalf("expected PSP called exactly once, got %d", pspCalls.Load())
	}
}

// TestIdempotencyConflict verifies that reusing an idempotency key with a
// different card_token returns 422 idempotency_conflict.
func TestIdempotencyConflict(t *testing.T) {
	gdb := setupTestDB(t)
	invoice := createTestInvoice(t, gdb, "open")
	apiKey := createTestAPIKey(t, gdb, invoice.BusinessID)

	mockPSP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "succeeded", "psp_ref": "ref-1"})
	}))
	defer mockPSP.Close()

	router := buildPayRouter(mockPSP.URL)
	idempKey := "conflict-test-" + uuid.New().String()

	resp1 := callPayEndpoint(router, apiKey, invoice.ID, "tok_success", idempKey)
	if resp1.Code != http.StatusOK {
		t.Fatalf("first call: expected 200, got %d", resp1.Code)
	}

	resp2 := callPayEndpoint(router, apiKey, invoice.ID, "tok_card_declined", idempKey)
	if resp2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("second call (different token): expected 422, got %d — body: %s", resp2.Code, resp2.Body.String())
	}

	var body map[string]interface{}
	json.Unmarshal(resp2.Body.Bytes(), &body)
	errObj, _ := body["error"].(map[string]interface{})
	if errObj["code"] != "idempotency_conflict" {
		t.Fatalf("expected code 'idempotency_conflict', got %v", errObj["code"])
	}

	var count int64
	db.DB.Model(&models.PaymentAttempt{}).Where("idempotency_key = ?", idempKey).Count(&count)
	if count != 1 {
		t.Fatalf("expected still 1 payment_attempt row, got %d", count)
	}
}
