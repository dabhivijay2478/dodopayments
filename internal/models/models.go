package models

import (
	"time"

	"github.com/google/uuid"
)

type Business struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string    `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type APIKey struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	BusinessID uuid.UUID  `gorm:"type:uuid;not null;index"`
	Business   Business   `gorm:"foreignKey:BusinessID"`
	KeyPrefix  string     `gorm:"not null;index;size:16"`
	KeyHash    string     `gorm:"not null"`
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

type Customer struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	BusinessID uuid.UUID `gorm:"type:uuid;not null;index"`
	Business   Business  `gorm:"foreignKey:BusinessID"`
	Name       string    `gorm:"not null"`
	Email      string    `gorm:"not null"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Invoice struct {
	ID              uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	BusinessID      uuid.UUID        `gorm:"type:uuid;not null;index"`
	Business        Business         `gorm:"foreignKey:BusinessID"`
	CustomerID      uuid.UUID        `gorm:"type:uuid;not null;index"`
	Customer        Customer         `gorm:"foreignKey:CustomerID"`
	State           string           `gorm:"not null;default:'draft';index"`
	DueDate         time.Time        `gorm:"not null"`
	TotalCents      int64            `gorm:"type:bigint;not null;default:0"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LineItems       []LineItem       `gorm:"foreignKey:InvoiceID"`
	PaymentAttempts []PaymentAttempt `gorm:"foreignKey:InvoiceID"`
}

type LineItem struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	InvoiceID       uuid.UUID `gorm:"type:uuid;not null;index"`
	Description     string    `gorm:"not null"`
	Quantity        int64     `gorm:"type:bigint;not null"`
	UnitAmountCents int64     `gorm:"type:bigint;not null"`
	CreatedAt       time.Time
}

type PaymentAttempt struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	InvoiceID      uuid.UUID `gorm:"type:uuid;not null;index"`
	IdempotencyKey string    `gorm:"not null;uniqueIndex;size:255"`
	Status         string    `gorm:"not null;default:'pending';index"`
	CardToken      string    `gorm:"not null"`
	PSPReference   *string
	FailureCode    *string
	RequestBody    string    `gorm:"type:text;not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type WebhookEndpoint struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	BusinessID uuid.UUID `gorm:"type:uuid;not null;index"`
	Business   Business  `gorm:"foreignKey:BusinessID"`
	URL        string    `gorm:"not null"`
	Secret     string    `gorm:"not null"`
	CreatedAt  time.Time
}

type WebhookDelivery struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	WebhookEndpointID uuid.UUID       `gorm:"type:uuid;not null;index"`
	WebhookEndpoint   WebhookEndpoint `gorm:"foreignKey:WebhookEndpointID"`
	EventType         string          `gorm:"not null;index"`
	Payload           string          `gorm:"type:text;not null"`
	Status            string          `gorm:"not null;default:'pending';index"`
	Attempts          int             `gorm:"not null;default:0"`
	NextRetryAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
