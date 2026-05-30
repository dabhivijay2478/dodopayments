package handlers

import (
	"errors"
	"net/http"

	"invoice-service/internal/db"
	"invoice-service/internal/models"

	"gorm.io/gorm"
)

// Bootstrap issues the first API key when none are active. No authentication required.
// Safe to call from Postman on a fresh or reset database. Returns 409 if an active key already exists.
func Bootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}

	var activeCount int64
	if err := db.DB.Model(&models.APIKey{}).Where("revoked_at IS NULL").Count(&activeCount).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to check API keys")
		return
	}
	if activeCount > 0 {
		respondError(w, http.StatusConflict, "already_bootstrapped",
			"An active API key already exists. Use POST /api-keys with your current key to rotate, or run docker compose down -v for a fresh database.")
		return
	}

	var business models.Business
	err := db.DB.Order("created_at ASC").First(&business).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		business = models.Business{Name: "Test Business"}
		if err := db.DB.Create(&business).Error; err != nil {
			respondError(w, http.StatusInternalServerError, "internal_error", "failed to create business")
			return
		}
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to load business")
		return
	}

	record, fullKey, err := IssueAPIKey(business.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to create API key")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"id":            record.ID,
		"business_id":   business.ID,
		"key_prefix":    record.KeyPrefix,
		"api_key":       fullKey,
		"token_type":    "Bearer",
		"authorization": "Bearer " + fullKey,
		"created_at":    record.CreatedAt,
		"message":       "First API key created. Store api_key now; it is not shown again.",
	})
}
