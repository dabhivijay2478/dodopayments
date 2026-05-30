package handlers

import (
	"encoding/json"
	"net/http"

	"invoice-service/internal/db"
	"invoice-service/internal/middleware"
	"invoice-service/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type createCustomerRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func CreateCustomer(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())

	var req createCustomerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.Name == "" || req.Email == "" {
		respondError(w, http.StatusBadRequest, "bad_request", "name and email are required")
		return
	}

	customer := models.Customer{
		BusinessID: businessID,
		Name:       req.Name,
		Email:      req.Email,
	}
	if err := db.DB.Create(&customer).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to create customer")
		return
	}

	respondJSON(w, http.StatusCreated, customer)
}

func ListCustomers(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())

	var customers []models.Customer
	if err := db.DB.Where("business_id = ?", businessID).Find(&customers).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to list customers")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"data":  customers,
		"count": len(customers),
	})
}

func GetCustomer(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())
	customerID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid customer id")
		return
	}

	var customer models.Customer
	if err := db.DB.Where("id = ? AND business_id = ?", customerID, businessID).First(&customer).Error; err != nil {
		respondError(w, http.StatusNotFound, "not_found", "customer not found")
		return
	}

	respondJSON(w, http.StatusOK, customer)
}
