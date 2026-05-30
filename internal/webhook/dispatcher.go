package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"invoice-service/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EventPayload struct {
	Event     string      `json:"event"`
	CreatedAt time.Time   `json:"created_at"`
	Data      interface{} `json:"data"`
}

var retrySchedule = []time.Duration{
	0,
	30 * time.Second,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
}

func Dispatch(gdb *gorm.DB, businessID uuid.UUID, eventType string, data interface{}) {
	var endpoints []models.WebhookEndpoint
	if err := gdb.Where("business_id = ?", businessID).Find(&endpoints).Error; err != nil || len(endpoints) == 0 {
		return
	}

	for _, ep := range endpoints {
		payload := EventPayload{
			Event:     eventType,
			CreatedAt: time.Now().UTC(),
			Data:      data,
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			continue
		}

		delivery := models.WebhookDelivery{
			WebhookEndpointID: ep.ID,
			EventType:         eventType,
			Payload:           string(payloadJSON),
			Status:            "pending",
		}
		if err := gdb.Create(&delivery).Error; err != nil {
			continue
		}

		go deliverWithRetry(gdb, delivery, ep, payloadJSON)
	}
}

func deliverWithRetry(gdb *gorm.DB, delivery models.WebhookDelivery, ep models.WebhookEndpoint, payload []byte) {
	for i, wait := range retrySchedule {
		if i > 0 {
			time.Sleep(wait)
		}

		mac := hmac.New(sha256.New, []byte(ep.Secret))
		mac.Write(payload)
		sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		ts := strconv.FormatInt(time.Now().Unix(), 10)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(payload))
		if err != nil {
			cancel()
			recordFailedAttempt(gdb, &delivery, i)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-Signature", sig)
		req.Header.Set("X-Webhook-Timestamp", ts)

		resp, err := http.DefaultClient.Do(req)
		cancel()
		delivery.Attempts++

		if err == nil && resp != nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				gdb.Model(&delivery).Updates(map[string]interface{}{
					"status":   "delivered",
					"attempts": delivery.Attempts,
				})
				return
			}
		}

		nextIdx := min(i+1, len(retrySchedule)-1)
		nextRetry := time.Now().Add(retrySchedule[nextIdx])
		gdb.Model(&delivery).Updates(map[string]interface{}{
			"status":        "pending",
			"attempts":      delivery.Attempts,
			"next_retry_at": nextRetry,
		})
	}

	gdb.Model(&delivery).Updates(map[string]interface{}{
		"status":   "failed",
		"attempts": delivery.Attempts,
	})
}

func recordFailedAttempt(gdb *gorm.DB, delivery *models.WebhookDelivery, i int) {
	delivery.Attempts++
	nextIdx := min(i+1, len(retrySchedule)-1)
	nextRetry := time.Now().Add(retrySchedule[nextIdx])
	gdb.Model(delivery).Updates(map[string]interface{}{
		"status":        "pending",
		"attempts":      delivery.Attempts,
		"next_retry_at": nextRetry,
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
