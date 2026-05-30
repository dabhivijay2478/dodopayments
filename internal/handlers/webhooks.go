package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"invoice-service/internal/db"
	"invoice-service/internal/middleware"
	"invoice-service/internal/models"
)

type createWebhookRequest struct {
	URL string `json:"url"`
}

func CreateWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())

	var req createWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.URL == "" || !strings.HasPrefix(req.URL, "http") {
		respondError(w, http.StatusBadRequest, "bad_request", "url is required and must start with http")
		return
	}

	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to generate secret")
		return
	}
	secret := hex.EncodeToString(secretBytes)

	endpoint := models.WebhookEndpoint{
		BusinessID: businessID,
		URL:        req.URL,
		Secret:     secret,
	}
	if err := db.DB.Create(&endpoint).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to create webhook endpoint")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         endpoint.ID,
		"url":        endpoint.URL,
		"secret":     endpoint.Secret,
		"created_at": endpoint.CreatedAt,
	})
}

func ListWebhookEndpoints(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())

	var endpoints []models.WebhookEndpoint
	if err := db.DB.Where("business_id = ?", businessID).Find(&endpoints).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to list webhook endpoints")
		return
	}

	type endpointResponse struct {
		ID        interface{} `json:"id"`
		URL       string      `json:"url"`
		CreatedAt interface{} `json:"created_at"`
	}

	data := make([]endpointResponse, len(endpoints))
	for i, ep := range endpoints {
		data[i] = endpointResponse{
			ID:        ep.ID,
			URL:       ep.URL,
			CreatedAt: ep.CreatedAt,
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"data":  data,
		"count": len(data),
	})
}
