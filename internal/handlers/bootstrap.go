package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"invoice-service/internal/config"
	"invoice-service/internal/db"
	"invoice-service/internal/models"

	"gorm.io/gorm"
)

type bootstrapRequest struct {
	Force bool `json:"force"`
}

// Bootstrap issues the first API key when none are active. No authentication required.
// When BOOTSTRAP_ALLOW_FORCE=true (local dev), revokes existing active keys and issues a new one.
func Bootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}

	cfg := config.Load()

	var req bootstrapRequest
	if r.Body != nil {
		body, _ := io.ReadAll(r.Body)
		if len(body) > 0 {
			_ = json.Unmarshal(body, &req)
		}
	}

	var activeCount int64
	if err := db.DB.Model(&models.APIKey{}).Where("revoked_at IS NULL").Count(&activeCount).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to check API keys")
		return
	}

	forceReset := req.Force || cfg.BootstrapAllowForce
	resetPerformed := false

	if activeCount > 0 {
		if !forceReset {
			respondError(w, http.StatusConflict, "already_bootstrapped",
				"An active API key already exists. Set BOOTSTRAP_ALLOW_FORCE=true (docker-compose), send {\"force\":true}, use POST /api-keys to rotate, or docker compose down -v")
			return
		}
		now := time.Now()
		if err := db.DB.Model(&models.APIKey{}).Where("revoked_at IS NULL").Update("revoked_at", now).Error; err != nil {
			respondError(w, http.StatusInternalServerError, "internal_error", "failed to revoke existing API keys")
			return
		}
		resetPerformed = true
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

	msg := "First API key created. Store api_key now; it is not shown again."
	if resetPerformed {
		msg = "New API key created; previous active keys were revoked. Store api_key now."
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"id":            record.ID,
		"business_id":   business.ID,
		"key_prefix":    record.KeyPrefix,
		"api_key":       fullKey,
		"token_type":    "Bearer",
		"authorization": "Bearer " + fullKey,
		"created_at":    record.CreatedAt,
		"reset":         resetPerformed,
		"message":       msg,
	})
}
