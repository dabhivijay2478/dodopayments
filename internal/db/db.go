package db

import (
	"fmt"
	"log"
	"os"
	"time"

	"invoice-service/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Connect(dsn string) error {
	var err error
	for i := 0; i < 10; i++ {
		DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
				SlowThreshold:             time.Second,
				LogLevel:                  logger.Warn,
				IgnoreRecordNotFoundError: true,
			}),
		})
		if err == nil {
			break
		}
		log.Printf("database connection attempt %d/10 failed: %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("failed to connect after 10 attempts: %w", err)
	}

	if err := DB.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto").Error; err != nil {
		return fmt.Errorf("create pgcrypto extension: %w", err)
	}

	if err := DB.AutoMigrate(
		&models.Business{},
		&models.APIKey{},
		&models.Customer{},
		&models.Invoice{},
		&models.LineItem{},
		&models.PaymentAttempt{},
		&models.WebhookEndpoint{},
		&models.WebhookDelivery{},
	); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}

	log.Println("database connected and migrated")
	return nil
}
