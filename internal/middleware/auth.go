package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"invoice-service/internal/db"
	"invoice-service/internal/models"

	"github.com/google/uuid"
)

type contextKey string

const businessIDKey contextKey = "business_id"

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			respondError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid authorization header")
			return
		}

		key := strings.TrimPrefix(auth, "Bearer ")
		if len(key) < 11 {
			respondError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid authorization header")
			return
		}

		prefix := key[:11]
		hash := sha256.Sum256([]byte(key))
		keyHash := hex.EncodeToString(hash[:])

		var apiKey models.APIKey
		if err := db.DB.Where("key_prefix = ? AND revoked_at IS NULL", prefix).First(&apiKey).Error; err != nil {
			respondError(w, http.StatusUnauthorized, "unauthorized", "invalid API key")
			return
		}

		if apiKey.KeyHash != keyHash {
			respondError(w, http.StatusUnauthorized, "unauthorized", "invalid API key")
			return
		}

		ctx := context.WithValue(r.Context(), businessIDKey, apiKey.BusinessID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetBusinessID(ctx context.Context) uuid.UUID {
	return ctx.Value(businessIDKey).(uuid.UUID)
}

func respondError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{"code": code, "message": message},
	})
}
