package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type PSPRequest struct {
	CardToken   string    `json:"card_token"`
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	InvoiceID   uuid.UUID `json:"invoice_id"`
}

type PSPResponse struct {
	Status string `json:"status"`
	PSPRef string `json:"psp_ref"`
	Code   string `json:"code"`
}

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

var DefaultClient *Client

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 35 * time.Second,
		},
	}
}

func (c *Client) Charge(ctx context.Context, token string, amountCents int64, invoiceID uuid.UUID) (*PSPResponse, error) {
	body, err := json.Marshal(PSPRequest{
		CardToken:   token,
		AmountCents: amountCents,
		Currency:    "usd",
		InvoiceID:   invoiceID,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/charge", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("psp error: status %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var pspResp PSPResponse
	if err := json.Unmarshal(respBody, &pspResp); err != nil {
		return nil, err
	}

	return &pspResp, nil
}
