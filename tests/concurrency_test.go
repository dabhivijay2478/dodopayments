package payment_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"invoice-service/internal/db"
	"invoice-service/internal/models"

	"github.com/google/uuid"
)

// TestConcurrentPayments fires 10 concurrent POST /invoices/{id}/pay requests
// for the same open invoice and asserts:
//   - Exactly 1 succeeds with 200
//   - The other 9 get 409 (conflict / invalid_state)
//   - Invoice final state is "paid"
//   - Only 1 payment_attempt has status "succeeded"
//   - No double-charges occur
func TestConcurrentPayments(t *testing.T) {
	gdb := setupTestDB(t)
	invoice := createTestInvoice(t, gdb, "open")
	apiKey := createTestAPIKey(t, gdb, invoice.BusinessID)

	mockPSP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "succeeded", "psp_ref": uuid.New().String()})
	}))
	defer mockPSP.Close()

	router := buildPayRouter(mockPSP.URL)

	const N = 10
	var wg sync.WaitGroup
	results := make([]int, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rr := callPayEndpoint(router, apiKey, invoice.ID, "tok_success", uuid.New().String())
			results[idx] = rr.Code
		}(i)
	}
	wg.Wait()

	successCount, conflictCount := 0, 0
	for _, code := range results {
		if code == http.StatusOK {
			successCount++
		}
		if code == http.StatusConflict {
			conflictCount++
		}
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 success (200), got %d — results: %v", successCount, results)
	}
	if conflictCount != N-1 {
		t.Fatalf("expected %d conflicts (409), got %d — results: %v", N-1, conflictCount, results)
	}

	var inv models.Invoice
	db.DB.First(&inv, "id = ?", invoice.ID)
	if inv.State != "paid" {
		t.Fatalf("expected invoice state 'paid', got '%s'", inv.State)
	}

	var succeededAttempts int64
	db.DB.Model(&models.PaymentAttempt{}).
		Where("invoice_id = ? AND status = ?", invoice.ID, "succeeded").
		Count(&succeededAttempts)
	if succeededAttempts != 1 {
		t.Fatalf("expected exactly 1 succeeded payment_attempt, got %d", succeededAttempts)
	}
}
