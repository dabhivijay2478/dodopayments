package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"invoice-service/internal/db"
	"invoice-service/internal/middleware"
	"invoice-service/internal/models"
	"invoice-service/internal/webhook"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type lineItemInput struct {
	Description     string `json:"description"`
	Quantity        int64  `json:"quantity"`
	UnitAmountCents int64  `json:"unit_amount_cents"`
}

type createInvoiceRequest struct {
	CustomerID string          `json:"customer_id"`
	DueDate    string          `json:"due_date"`
	LineItems  []lineItemInput `json:"line_items"`
}

func parseDueDate(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}

func CreateInvoice(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())

	var req createInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	customerID, err := uuid.Parse(req.CustomerID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "customer_id is required")
		return
	}

	var customer models.Customer
	if err := db.DB.Where("id = ? AND business_id = ?", customerID, businessID).First(&customer).Error; err != nil {
		respondError(w, http.StatusNotFound, "not_found", "customer not found")
		return
	}

	dueDate, err := parseDueDate(req.DueDate)
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid due_date")
		return
	}

	if len(req.LineItems) == 0 {
		respondError(w, http.StatusBadRequest, "bad_request", "line_items must not be empty")
		return
	}

	var totalCents int64
	for _, item := range req.LineItems {
		if item.Quantity <= 0 || item.UnitAmountCents <= 0 {
			respondError(w, http.StatusBadRequest, "bad_request", "quantity and unit_amount_cents must be positive")
			return
		}
		totalCents += item.Quantity * item.UnitAmountCents
	}

	var invoice models.Invoice
	err = db.DB.Transaction(func(tx *gorm.DB) error {
		invoice = models.Invoice{
			BusinessID: businessID,
			CustomerID: customerID,
			State:      "draft",
			DueDate:    dueDate,
			TotalCents: totalCents,
		}
		if err := tx.Create(&invoice).Error; err != nil {
			return err
		}
		for _, item := range req.LineItems {
			li := models.LineItem{
				InvoiceID:       invoice.ID,
				Description:     item.Description,
				Quantity:        item.Quantity,
				UnitAmountCents: item.UnitAmountCents,
			}
			if err := tx.Create(&li).Error; err != nil {
				return err
			}
			invoice.LineItems = append(invoice.LineItems, li)
		}
		return nil
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to create invoice")
		return
	}

	db.DB.Preload("LineItems").Preload("Customer").First(&invoice, invoice.ID)
	go webhook.Dispatch(db.DB, businessID, "invoice.created", invoice)

	respondJSON(w, http.StatusCreated, invoice)
}

func ListInvoices(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())
	state := r.URL.Query().Get("state")

	query := db.DB.Where("business_id = ?", businessID).Preload("Customer")
	if state != "" {
		query = query.Where("state = ?", state)
	}

	var invoices []models.Invoice
	if err := query.Find(&invoices).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to list invoices")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"data":  invoices,
		"count": len(invoices),
	})
}

func GetInvoice(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())
	invoiceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid invoice id")
		return
	}

	var invoice models.Invoice
	if err := db.DB.Where("id = ? AND business_id = ?", invoiceID, businessID).
		Preload("LineItems").
		Preload("Customer").
		Preload("PaymentAttempts").
		First(&invoice).Error; err != nil {
		respondError(w, http.StatusNotFound, "not_found", "invoice not found")
		return
	}

	respondJSON(w, http.StatusOK, invoice)
}

func FinalizeInvoice(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())
	invoiceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid invoice id")
		return
	}

	var invoice models.Invoice
	if err := db.DB.Where("id = ? AND business_id = ?", invoiceID, businessID).First(&invoice).Error; err != nil {
		respondError(w, http.StatusNotFound, "not_found", "invoice not found")
		return
	}

	if invoice.State != "draft" {
		respondError(w, http.StatusConflict, "invalid_transition",
			"can only finalize a draft invoice, current state: "+invoice.State)
		return
	}

	db.DB.Model(&invoice).Update("state", "open")
	invoice.State = "open"
	db.DB.Preload("LineItems").Preload("Customer").First(&invoice, invoice.ID)

	respondJSON(w, http.StatusOK, invoice)
}

func VoidInvoice(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())
	invoiceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid invoice id")
		return
	}

	var invoice models.Invoice
	if err := db.DB.Where("id = ? AND business_id = ?", invoiceID, businessID).First(&invoice).Error; err != nil {
		respondError(w, http.StatusNotFound, "not_found", "invoice not found")
		return
	}

	if invoice.State != "draft" && invoice.State != "open" {
		respondError(w, http.StatusConflict, "invalid_transition",
			"can only void a draft or open invoice, current state: "+invoice.State)
		return
	}

	db.DB.Model(&invoice).Update("state", "void")
	invoice.State = "void"
	db.DB.Preload("LineItems").Preload("Customer").First(&invoice, invoice.ID)

	respondJSON(w, http.StatusOK, invoice)
}
