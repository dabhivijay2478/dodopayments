package payment_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"invoice-service/internal/db"
	"invoice-service/internal/models"

	"github.com/google/uuid"
)

// TestPSPTimeout verifies that when the PSP hangs (tok_timeout, 30s sleep),
// the API times out at ~10s and:
//   - Returns 402 with code "timeout"
//   - Invoice stays "open" (not corrupted)
//   - payment_attempt is marked "failed" with failure_code "timeout"
//   - Total elapsed time is under 15 seconds
func TestPSPTimeout(t *testing.T) {
	gdb := setupTestDB(t)
	invoice := createTestInvoice(t, gdb, "open")
	apiKey := createTestAPIKey(t, gdb, invoice.BusinessID)

	mockPSP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(12 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "succeeded", "psp_ref": "late-ref"})
	}))
	defer mockPSP.Close()

	router := buildPayRouter(mockPSP.URL)
	start := time.Now()
	resp := callPayEndpoint(router, apiKey, invoice.ID, "tok_timeout", uuid.New().String())
	elapsed := time.Since(start)

	if elapsed >= 15*time.Second {
		t.Fatalf("endpoint hung for %v; expected < 15s", elapsed)
	}
	if resp.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d — body: %s", resp.Code, resp.Body.String())
	}

	var body map[string]interface{}
	json.Unmarshal(resp.Body.Bytes(), &body)
	if body["code"] != "timeout" {
		t.Fatalf("expected code 'timeout', got '%v'", body["code"])
	}

	var inv models.Invoice
	gdb.First(&inv, "id = ?", invoice.ID)
	if inv.State != "open" {
		t.Fatalf("invoice should remain 'open' after timeout, got '%s'", inv.State)
	}

	var attempt models.PaymentAttempt
	gdb.Where("invoice_id = ?", invoice.ID).First(&attempt)
	if attempt.Status != "failed" {
		t.Fatalf("payment_attempt should be 'failed', got '%s'", attempt.Status)
	}
	if attempt.FailureCode == nil || *attempt.FailureCode != "timeout" {
		fc := "<nil>"
		if attempt.FailureCode != nil {
			fc = *attempt.FailureCode
		}
		t.Fatalf("expected failure_code 'timeout', got '%s'", fc)
	}
}

// TestPSPNetworkError verifies that when the PSP returns 500 (tok_network_error):
//   - Returns 402 with code "network_error"
//   - Invoice stays "open"
//   - payment_attempt is marked "failed" with failure_code "network_error"
func TestPSPNetworkError(t *testing.T) {
	gdb := setupTestDB(t)
	invoice := createTestInvoice(t, gdb, "open")
	apiKey := createTestAPIKey(t, gdb, invoice.BusinessID)

	mockPSP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockPSP.Close()

	router := buildPayRouter(mockPSP.URL)
	resp := callPayEndpoint(router, apiKey, invoice.ID, "tok_network_error", uuid.New().String())

	if resp.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d — body: %s", resp.Code, resp.Body.String())
	}

	var body map[string]interface{}
	json.Unmarshal(resp.Body.Bytes(), &body)
	if body["code"] != "network_error" {
		t.Fatalf("expected code 'network_error', got '%v'", body["code"])
	}

	var inv models.Invoice
	db.DB.First(&inv, "id = ?", invoice.ID)
	if inv.State != "open" {
		t.Fatalf("invoice should remain 'open' after network error, got '%s'", inv.State)
	}

	var attempt models.PaymentAttempt
	db.DB.Where("invoice_id = ?", invoice.ID).First(&attempt)
	if attempt.Status != "failed" {
		t.Fatalf("payment_attempt should be 'failed', got '%s'", attempt.Status)
	}
	if attempt.FailureCode == nil || *attempt.FailureCode != "network_error" {
		fc := "<nil>"
		if attempt.FailureCode != nil {
			fc = *attempt.FailureCode
		}
		t.Fatalf("expected failure_code 'network_error', got '%s'", fc)
	}
}
