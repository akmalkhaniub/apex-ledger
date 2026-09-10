package rails

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PayoutRequest specifies money movement payload for external banking rails
type PayoutRequest struct {
	TransferID       uuid.UUID
	Amount           int64
	Currency         string
	RecipientName    string
	AccountNumber    string
	RoutingOrBIC     string
	DestinationRail  string // ACH, FEDNOW, SWIFT, SEPA
	IdempotencyToken string
}

// PayoutResponse specifies external banking partner execution result
type PayoutResponse struct {
	ExternalID     string
	ProviderStatus string // SUBMITTED, SETTLED, REJECTED
	EstimatedDate  time.Time
	FeeCharged     int64
}

// WebhookEvent represents an asynchronous banking partner status update
type WebhookEvent struct {
	EventID    string
	TransferID uuid.UUID
	Status     string
	RawPayload string
}

// RailProvider represents an abstracted, provider-agnostic banking infrastructure partner
type RailProvider interface {
	Name() string
	SupportedRails() []string
	InitiatePayout(ctx context.Context, req PayoutRequest) (*PayoutResponse, error)
	FetchStatus(ctx context.Context, externalID string) (*PayoutResponse, error)
	ProcessWebhook(ctx context.Context, payload []byte) (*WebhookEvent, error)
}

// ColumnAdapter integrates with Column BaaS (ACH / FedNow)
type ColumnAdapter struct {
	apiKey string
}

func NewColumnAdapter(apiKey string) *ColumnAdapter {
	return &ColumnAdapter{apiKey: apiKey}
}

func (c *ColumnAdapter) Name() string {
	return "COLUMN_BAAS"
}

func (c *ColumnAdapter) SupportedRails() []string {
	return []string{"ACH", "FEDNOW", "WIRE"}
}

func (c *ColumnAdapter) InitiatePayout(ctx context.Context, req PayoutRequest) (*PayoutResponse, error) {
	// Simulates Column BaaS payment origination
	extID := fmt.Sprintf("col_ach_%s", uuid.New().String()[:8])
	return &PayoutResponse{
		ExternalID:     extID,
		ProviderStatus: "SUBMITTED",
		EstimatedDate:  time.Now().UTC().Add(24 * time.Hour),
		FeeCharged:     25, // $0.25 ACH origination cost
	}, nil
}

func (c *ColumnAdapter) FetchStatus(ctx context.Context, externalID string) (*PayoutResponse, error) {
	return &PayoutResponse{
		ExternalID:     externalID,
		ProviderStatus: "SETTLED",
		EstimatedDate:  time.Now().UTC(),
		FeeCharged:     25,
	}, nil
}

func (c *ColumnAdapter) ProcessWebhook(ctx context.Context, payload []byte) (*WebhookEvent, error) {
	return &WebhookEvent{
		EventID:    uuid.New().String(),
		TransferID: uuid.New(),
		Status:     "ach.origination.settled",
		RawPayload: string(payload),
	}, nil
}

// WiseAdapter integrates with Wise Platform (Cross-border SWIFT & Multi-currency FX)
type WiseAdapter struct {
	apiKey string
}

func NewWiseAdapter(apiKey string) *WiseAdapter {
	return &WiseAdapter{apiKey: apiKey}
}

func (w *WiseAdapter) Name() string {
	return "WISE_FX"
}

func (w *WiseAdapter) SupportedRails() []string {
	return []string{"SWIFT", "SEPA", "FX"}
}

func (w *WiseAdapter) InitiatePayout(ctx context.Context, req PayoutRequest) (*PayoutResponse, error) {
	extID := fmt.Sprintf("wise_tr_%s", uuid.New().String()[:8])
	return &PayoutResponse{
		ExternalID:     extID,
		ProviderStatus: "PROCESSING",
		EstimatedDate:  time.Now().UTC().Add(4 * time.Hour),
		FeeCharged:     150, // $1.50 international FX fee
	}, nil
}

func (w *WiseAdapter) FetchStatus(ctx context.Context, externalID string) (*PayoutResponse, error) {
	return &PayoutResponse{
		ExternalID:     externalID,
		ProviderStatus: "FUNDS_CONVERTED_AND_SENT",
		EstimatedDate:  time.Now().UTC(),
		FeeCharged:     150,
	}, nil
}

func (w *WiseAdapter) ProcessWebhook(ctx context.Context, payload []byte) (*WebhookEvent, error) {
	return &WebhookEvent{
		EventID:    uuid.New().String(),
		TransferID: uuid.New(),
		Status:     "transfer.state-change.success",
		RawPayload: string(payload),
	}, nil
}

// RailRouter dispatches money movement to the appropriate banking partner
type RailRouter struct {
	providers map[string]RailProvider
}

func NewRailRouter() *RailRouter {
	r := &RailRouter{providers: make(map[string]RailProvider)}
	// Register default providers
	column := NewColumnAdapter("mock_column_key")
	wise := NewWiseAdapter("mock_wise_key")

	for _, rail := range column.SupportedRails() {
		r.providers[rail] = column
	}
	for _, rail := range wise.SupportedRails() {
		r.providers[rail] = wise
	}
	return r
}

func (r *RailRouter) Route(rail string) (RailProvider, error) {
	provider, exists := r.providers[rail]
	if !exists {
		return nil, fmt.Errorf("no banking rail provider registered for rail: %s", rail)
	}
	return provider, nil
}
