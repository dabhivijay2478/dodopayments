package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"invoice-service/internal/db"
	"invoice-service/internal/middleware"
	"invoice-service/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type createAPIKeyRequest struct {
	RevokeKeyID *string `json:"revoke_key_id"`
}

// NewAPIKeyMaterial generates a new sk_ secret and its stored prefix + hash.
func NewAPIKeyMaterial() (fullKey, prefix, keyHash string, err error) {
	keyBytes := make([]byte, 32)
	if _, err = rand.Read(keyBytes); err != nil {
		return "", "", "", err
	}
	fullKey = "sk_" + hex.EncodeToString(keyBytes)
	prefix = fullKey[:11]
	hash := sha256.Sum256([]byte(fullKey))
	keyHash = hex.EncodeToString(hash[:])
	return fullKey, prefix, keyHash, nil
}

// IssueAPIKey persists a new API key for the business and returns the record and full secret.
func IssueAPIKey(businessID uuid.UUID) (*models.APIKey, string, error) {
	fullKey, prefix, keyHash, err := NewAPIKeyMaterial()
	if err != nil {
		return nil, "", err
	}
	record := models.APIKey{
		BusinessID: businessID,
		KeyPrefix:  prefix,
		KeyHash:    keyHash,
	}
	if err := db.DB.Create(&record).Error; err != nil {
		return nil, "", err
	}
	return &record, fullKey, nil
}

func CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())

	var req createAPIKeyRequest
	if r.Body != nil && r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	record, fullKey, err := IssueAPIKey(businessID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to create API key")
		return
	}

	if req.RevokeKeyID != nil && *req.RevokeKeyID != "" {
		oldID, err := uuid.Parse(*req.RevokeKeyID)
		if err != nil {
			respondError(w, http.StatusBadRequest, "bad_request", "invalid revoke_key_id")
			return
		}
		if err := revokeAPIKeyByID(businessID, oldID); err != nil {
			if err == errAPIKeyNotFound {
				respondError(w, http.StatusNotFound, "not_found", "API key to revoke not found")
				return
			}
			if err == errAPIKeyAlreadyRevoked {
				respondError(w, http.StatusConflict, "already_revoked", "API key is already revoked")
				return
			}
			respondError(w, http.StatusInternalServerError, "internal_error", "failed to revoke previous API key")
			return
		}
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"id":            record.ID,
		"key_prefix":    record.KeyPrefix,
		"api_key":       fullKey,
		"token_type":    "Bearer",
		"authorization": "Bearer " + fullKey,
		"created_at":    record.CreatedAt,
		"message":       "Store api_key now. Revoked keys return 401 immediately.",
	})
}

func ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())

	var keys []models.APIKey
	if err := db.DB.Where("business_id = ?", businessID).Order("created_at DESC").Find(&keys).Error; err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to list API keys")
		return
	}

	type listItem struct {
		ID         uuid.UUID  `json:"id"`
		KeyPrefix  string     `json:"key_prefix"`
		RevokedAt  *time.Time `json:"revoked_at,omitempty"`
		CreatedAt  time.Time  `json:"created_at"`
		BusinessID uuid.UUID  `json:"business_id"`
	}

	data := make([]listItem, len(keys))
	for i, k := range keys {
		data[i] = listItem{
			ID:         k.ID,
			KeyPrefix:  k.KeyPrefix,
			RevokedAt:  k.RevokedAt,
			CreatedAt:  k.CreatedAt,
			BusinessID: k.BusinessID,
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"data":  data,
		"count": len(data),
	})
}

var (
	errAPIKeyNotFound       = errSentinel("api key not found")
	errAPIKeyAlreadyRevoked = errSentinel("api key already revoked")
)

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

func revokeAPIKeyByID(businessID, keyID uuid.UUID) error {
	var apiKey models.APIKey
	if err := db.DB.Where("id = ? AND business_id = ?", keyID, businessID).First(&apiKey).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errAPIKeyNotFound
		}
		return err
	}
	if apiKey.RevokedAt != nil {
		return errAPIKeyAlreadyRevoked
	}
	now := time.Now()
	return db.DB.Model(&apiKey).Update("revoked_at", now).Error
}

func RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	businessID := middleware.GetBusinessID(r.Context())
	keyID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid API key id")
		return
	}

	if err := revokeAPIKeyByID(businessID, keyID); err != nil {
		switch err {
		case errAPIKeyNotFound:
			respondError(w, http.StatusNotFound, "not_found", "API key not found")
		case errAPIKeyAlreadyRevoked:
			respondError(w, http.StatusConflict, "already_revoked", "API key is already revoked")
		default:
			respondError(w, http.StatusInternalServerError, "internal_error", "failed to revoke API key")
		}
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"id":         keyID,
		"revoked_at": time.Now(),
		"message":    "API key revoked; requests using it will receive 401",
	})
}
