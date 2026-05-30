package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"invoice-service/internal/db"
	"invoice-service/internal/middleware"
	"invoice-service/internal/models"
	"invoice-service/internal/payment"
	"invoice-service/internal/webhook"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type payInvoiceRequest struct {
	CardToken string `json:"card_token"`
}

func PayInvoice(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())
	invoiceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid invoice id")
		return
	}

	idempKey := r.Header.Get("Idempotency-Key")
	if idempKey == "" {
		respondError(w, http.StatusBadRequest, "missing_idempotency_key", "Idempotency-Key header is required")
		return
	}

	var req payInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.CardToken == "" {
		respondError(w, http.StatusBadRequest, "bad_request", "card_token is required")
		return
	}

	var existing models.PaymentAttempt
	if err := db.DB.Where("idempotency_key = ?", idempKey).First(&existing).Error; err == nil {
		var stored map[string]string
		json.Unmarshal([]byte(existing.RequestBody), &stored)
		if stored["card_token"] != req.CardToken {
			respondError(w, http.StatusUnprocessableEntity, "idempotency_conflict",
				"idempotency key reused with different request body")
			return
		}
		switch existing.Status {
		case "succeeded":
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"status":             "succeeded",
				"payment_attempt_id": existing.ID,
				"psp_reference":      existing.PSPReference,
			})
			return
		case "failed":
			code := ""
			if existing.FailureCode != nil {
				code = *existing.FailureCode
			}
			respondJSON(w, http.StatusPaymentRequired, map[string]interface{}{
				"status": "failed",
				"code":   code,
			})
			return
		case "pending":
			respondJSON(w, http.StatusAccepted, map[string]interface{}{
				"status":  "pending",
				"message": "payment is already being processed, check invoice state",
			})
			return
		}
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to start transaction")
		return
	}

	var invoice models.Invoice
	result := tx.Raw(
		"SELECT * FROM invoices WHERE id = ? AND business_id = ? FOR UPDATE",
		invoiceID, businessID,
	).Scan(&invoice)
	if result.Error != nil || result.RowsAffected == 0 || invoice.ID == uuid.Nil {
		tx.Rollback()
		respondError(w, http.StatusNotFound, "not_found", "invoice not found")
		return
	}

	if invoice.State != "open" {
		tx.Rollback()
		respondError(w, http.StatusConflict, "invalid_state",
			"invoice is not in open state, current state: "+invoice.State)
		return
	}

	var inflight int64
	if err := tx.Model(&models.PaymentAttempt{}).
		Where("invoice_id = ? AND status IN ?", invoice.ID, []string{"pending", "succeeded"}).
		Count(&inflight).Error; err != nil {
		tx.Rollback()
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to check payment attempts")
		return
	}
	if inflight > 0 {
		tx.Rollback()
		respondError(w, http.StatusConflict, "invalid_state",
			"invoice is not in open state, current state: "+invoice.State)
		return
	}

	reqBodyJSON, _ := json.Marshal(map[string]string{"card_token": req.CardToken})
	attempt := models.PaymentAttempt{
		InvoiceID:      invoice.ID,
		IdempotencyKey: idempKey,
		Status:         "pending",
		CardToken:      req.CardToken,
		RequestBody:    string(reqBodyJSON),
	}
	if err := tx.Create(&attempt).Error; err != nil {
		tx.Rollback()
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to create payment attempt")
		return
	}

	if err := tx.Commit().Error; err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to commit transaction")
		return
	}

	pspCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	pspResp, err := payment.DefaultClient.Charge(pspCtx, req.CardToken, invoice.TotalCents, invoice.ID)

	if err != nil {
		failCode := "network_error"
		if errors.Is(err, context.DeadlineExceeded) {
			failCode = "timeout"
		}
		db.DB.Model(&attempt).Updates(map[string]interface{}{
			"status":       "failed",
			"failure_code": failCode,
		})
		go webhook.Dispatch(db.DB, businessID, "invoice.payment_failed", map[string]interface{}{
			"invoice_id":   invoice.ID,
			"failure_code": failCode,
		})
		respondJSON(w, http.StatusPaymentRequired, map[string]interface{}{
			"status": "failed",
			"code":   failCode,
		})
		return
	}

	if pspResp.Status == "failed" {
		db.DB.Model(&attempt).Updates(map[string]interface{}{
			"status":       "failed",
			"failure_code": pspResp.Code,
		})
		go webhook.Dispatch(db.DB, businessID, "invoice.payment_failed", map[string]interface{}{
			"invoice_id":   invoice.ID,
			"failure_code": pspResp.Code,
		})
		respondJSON(w, http.StatusPaymentRequired, map[string]interface{}{
			"status": "failed",
			"code":   pspResp.Code,
		})
		return
	}

	if pspResp.Status == "succeeded" {
		db.DB.Model(&attempt).Updates(map[string]interface{}{
			"status":        "succeeded",
			"psp_reference": pspResp.PSPRef,
		})
		db.DB.Model(&invoice).Update("state", "paid")
		go webhook.Dispatch(db.DB, businessID, "invoice.paid", map[string]interface{}{
			"invoice_id":    invoice.ID,
			"psp_reference": pspResp.PSPRef,
		})
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"status":             "succeeded",
			"payment_attempt_id": attempt.ID,
			"psp_reference":      pspResp.PSPRef,
		})
		return
	}

	db.DB.Model(&attempt).Updates(map[string]interface{}{
		"status":       "failed",
		"failure_code": "unknown_error",
	})
	respondJSON(w, http.StatusPaymentRequired, map[string]interface{}{
		"status": "failed",
		"code":   "unknown_error",
	})
}
