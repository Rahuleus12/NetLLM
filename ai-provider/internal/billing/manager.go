// internal/billing/manager.go
// Billing management and operations
// Handles subscriptions, invoices, payments, and billing cycles

package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSubscriptionNotFound    = errors.New("subscription not found")
	ErrSubscriptionActive      = errors.New("subscription is active")
	ErrPaymentFailed          = errors.New("payment failed")
	ErrInvoiceNotFound         = errors.New("invoice not found")
	ErrInvalidBillingPeriod   = errors.New("invalid billing period")
	ErrPaymentMethodNotFound  = errors.New("payment method not found")
	ErrPlanNotFound           = errors.New("plan not found")
)

// SubscriptionStatus represents status of a subscription
type SubscriptionStatus string

const (
	SubscriptionStatusActive    SubscriptionStatus = "active"
	SubscriptionStatusPending   SubscriptionStatus = "pending"
	SubscriptionStatusCanceled SubscriptionStatus = "canceled"
	SubscriptionStatusExpired  SubscriptionStatus = "expired"
	SubscriptionStatusPastDue  SubscriptionStatus = "past_due"
)

// BillingPeriod represents billing frequency
type BillingPeriod string

const (
	BillingPeriodMonthly  BillingPeriod = "monthly"
	BillingPeriodQuarterly BillingPeriod = "quarterly"
	BillingPeriodAnnually BillingPeriod = "annually"
)

// Subscription represents a customer subscription
type Subscription struct {
	ID              uuid.UUID          `json:"id" db:"id"`
	TenantID        uuid.UUID          `json:"tenant_id" db:"tenant_id"`
	PlanID          uuid.UUID          `json:"plan_id" db:"plan_id"`
	Status          SubscriptionStatus `json:"status" db:"status"`

	// Subscription details
	CurrentPeriodStart time.Time      `json:"current_period_start" db:"current_period_start"`
	CurrentPeriodEnd   time.Time      `json:"current_period_end" db:"current_period_end"`
	TrialStart         *time.Time     `json:"trial_start,omitempty" db:"trial_start"`
	TrialEnd           *time.Time     `json:"trial_end,omitempty"`

	// Billing settings
	Period           BillingPeriod `json:"period" db:"period"`
	AutoRenew        bool          `json:"auto_renew" db:"auto_renew"`
	CancelAtPeriodEnd bool          `json:"cancel_at_period_end" db:"cancel_at_period_end"`

	// Payment method
	DefaultPaymentMethodID *uuid.UUID `json:"default_payment_method_id,omitempty" db:"default_payment_method_id"`

	// Pricing
	BasePrice         float64 `json:"base_price" db:"base_price"`
	Currency         string   `json:"currency" db:"currency"`
	TaxRate          float64 `json:"tax_rate" db:"tax_rate"`

	// Usage-based pricing
	UsageBasedPricing  bool               `json:"usage_based_pricing" db:"usage_based_pricing"`
	UsageThresholds   json.RawMessage    `json:"usage_thresholds,omitempty" db:"usage_thresholds"`

	// Metadata
	Metadata          json.RawMessage    `json:"metadata,omitempty" db:"metadata"`

	// Timestamps
	CreatedAt         time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at" db:"updated_at"`
	CanceledAt        *time.Time         `json:"canceled_at,omitempty" db:"canceled_at"`
}

// PaymentMethod represents a payment method
type PaymentMethod struct {
	ID              uuid.UUID   `json:"id" db:"id"`
	TenantID        uuid.UUID   `json:"tenant_id" db:"tenant_id"`
	Type            string      `json:"type" db:"type"` // card, bank_account, paypal, etc.
	Provider        string      `json:"provider" db:"provider"` // stripe, paypal, etc.
	ProviderID      string      `json:"provider_id" db:"provider_id"`

	// Card details (if applicable)
	CardBrand       *string     `json:"card_brand,omitempty" db:"card_brand"`
	CardLastFour    *string     `json:"card_last_four,omitempty" db:"card_last_four"`
	CardExpiryMonth *int        `json:"card_expiry_month,omitempty" db:"card_expiry_month"`
	CardExpiryYear  *int        `json:"card_expiry_year,omitempty" db:"card_expiry_year"`

	// Status
	IsDefault       bool        `json:"is_default" db:"is_default"`
	Status          string      `json:"status" db:"status"` // active, expired, failed

	// Metadata
	Metadata        json.RawMessage `json:"metadata,omitempty" db:"metadata"`

	// Timestamps
	CreatedAt       time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at" db:"updated_at"`
	ExpiresAt       *time.Time  `json:"expires_at,omitempty" db:"expires_at"`
}

// PaymentTransaction represents a payment transaction
type PaymentTransaction struct {
	ID               uuid.UUID   `json:"id" db:"id"`
	SubscriptionID   uuid.UUID   `json:"subscription_id" db:"subscription_id"`
	InvoiceID        *uuid.UUID  `json:"invoice_id,omitempty" db:"invoice_id"`
	TenantID         uuid.UUID   `json:"tenant_id" db:"tenant_id"`

	// Payment details
	Amount           float64     `json:"amount" db:"amount"`
	Currency         string      `json:"currency" db:"currency"`
	Status           string      `json:"status" db:"status"` // pending, completed, failed, refunded
	PaymentMethodID  uuid.UUID   `json:"payment_method_id" db:"payment_method_id"`

	// Transaction details
	Provider         string      `json:"provider" db:"provider"`
	ProviderID      string      `json:"provider_id" db:"provider_id"`

	// Refund information
	RefundedAmount   float64     `json:"refunded_amount" db:"refunded_amount"`
	RefundedAt       *time.Time  `json:"refunded_at,omitempty" db:"refunded_at"`

	// Error handling
	FailureReason    *string     `json:"failure_reason,omitempty" db:"failure_reason"`

	// Metadata
	Metadata         json.RawMessage `json:"metadata,omitempty" db:"metadata"`

	// Timestamps
	CreatedAt        time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at" db:"updated_at"`
	CompletedAt      *time.Time  `json:"completed_at,omitempty" db:"completed_at"`
}

// CreateSubscriptionRequest represents a request to create a subscription
type CreateSubscriptionRequest struct {
	TenantID             uuid.UUID      `json:"tenant_id"`
	PlanID               uuid.UUID      `json:"plan_id"`
	Period               BillingPeriod  `json:"period"`
	PaymentMethodID      uuid.UUID      `json:"payment_method_id"`
	TrialEnd             *time.Time     `json:"trial_end,omitempty"`
	UsageBasedPricing    bool          `json:"usage_based_pricing"`
	Metadata             interface{}   `json:"metadata,omitempty"`
}

// UpdateSubscriptionRequest represents a request to update a subscription
type UpdateSubscriptionRequest struct {
	PlanID               *uuid.UUID     `json:"plan_id,omitempty"`
	Status               *SubscriptionStatus `json:"status,omitempty"`
	Period               *BillingPeriod  `json:"period,omitempty"`
	AutoRenew            *bool          `json:"auto_renew,omitempty"`
	CancelAtPeriodEnd    *bool          `json:"cancel_at_period_end,omitempty"`
	DefaultPaymentMethodID *uuid.UUID     `json:"default_payment_method_id,omitempty"`
	UsageBasedPricing    *bool          `json:"usage_based_pricing,omitempty"`
}

// Manager manages billing operations
type Manager struct {
	db *sql.DB
}

// NewManager creates a new billing manager
func NewManager(db *sql.DB) *Manager {
	return &Manager{
		db: db,
	}
}

// CreateSubscription creates a new subscription
func (m *Manager) CreateSubscription(ctx context.Context, req CreateSubscriptionRequest) (*Subscription, error) {
	if req.TenantID == uuid.Nil {
		return nil, errors.New("tenant_id is required")
	}
	if req.PlanID == uuid.Nil {
		return nil, errors.New("plan_id is required")
	}
	if req.PaymentMethodID == uuid.Nil {
		return nil, errors.New("payment_method_id is required")
	}

	// Get plan details
	plan, err := m.getPlan(ctx, req.PlanID)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	// Calculate billing period
	now := time.Now()
	var periodStart, periodEnd time.Time

	switch req.Period {
	case BillingPeriodMonthly:
		periodStart = now
		periodEnd = now.AddDate(0, 1, 0)
	case BillingPeriodQuarterly:
		periodStart = now
		periodEnd = now.AddDate(0, 3, 0)
	case BillingPeriodAnnually:
		periodStart = now
		periodEnd = now.AddDate(1, 0, 0)
	default:
		return nil, ErrInvalidBillingPeriod
	}

	// Apply trial if specified
	if req.TrialEnd != nil && req.TrialEnd.After(now) {
		periodEnd = *req.TrialEnd
	}

	subscription := &Subscription{
		ID:                   uuid.New(),
		TenantID:             req.TenantID,
		PlanID:               req.PlanID,
		Status:               SubscriptionStatusPending,
		CurrentPeriodStart:   periodStart,
		CurrentPeriodEnd:     periodEnd,
		TrialStart:          nil,
		TrialEnd:            req.TrialEnd,
		Period:               req.Period,
		AutoRenew:            true,
		CancelAtPeriodEnd:    false,
		DefaultPaymentMethodID: &req.PaymentMethodID,
		BasePrice:            plan.BasePrice,
		Currency:             plan.Currency,
		TaxRate:             plan.TaxRate,
		UsageBasedPricing:    req.UsageBasedPricing,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	// Marshal metadata
	if req.Metadata != nil {
		metadataJSON, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		subscription.Metadata = metadataJSON
	} else {
		subscription.Metadata = []byte("{}")
	}

	query := `
		INSERT INTO subscriptions (id, tenant_id, plan_id, status,
			current_period_start, current_period_end, trial_start, trial_end,
			period, auto_renew, cancel_at_period_end, default_payment_method_id,
			base_price, currency, tax_rate, usage_based_pricing, metadata,
			created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17, $18, $19)
		RETURNING id, tenant_id, plan_id, status, current_period_start, current_period_end,
			trial_start, trial_end, period, auto_renew, cancel_at_period_end,
			default_payment_method_id, base_price, currency, tax_rate,
			usage_based_pricing, usage_thresholds, metadata, created_at,
			updated_at, canceled_at
	`

	err = m.db.QueryRowContext(ctx, query,
		subscription.ID,
		subscription.TenantID,
		subscription.PlanID,
		subscription.Status,
		subscription.CurrentPeriodStart,
		subscription.CurrentPeriodEnd,
		subscription.TrialStart,
		subscription.TrialEnd,
		subscription.Period,
		subscription.AutoRenew,
		subscription.CancelAtPeriodEnd,
		subscription.DefaultPaymentMethodID,
		subscription.BasePrice,
		subscription.Currency,
		subscription.TaxRate,
		subscription.UsageBasedPricing,
		subscription.UsageThresholds,
		subscription.Metadata,
		subscription.CreatedAt,
		subscription.UpdatedAt,
	).Scan(
		&subscription.ID,
		&subscription.TenantID,
		&subscription.PlanID,
		&subscription.Status,
		&subscription.CurrentPeriodStart,
		&subscription.CurrentPeriodEnd,
		&subscription.TrialStart,
		&subscription.TrialEnd,
		&subscription.Period,
		&subscription.AutoRenew,
		&subscription.CancelAtPeriodEnd,
		&subscription.DefaultPaymentMethodID,
		&subscription.BasePrice,
		&subscription.Currency,
		&subscription.TaxRate,
		&subscription.UsageBasedPricing,
		&subscription.UsageThresholds,
		&subscription.Metadata,
		&subscription.CreatedAt,
		&subscription.UpdatedAt,
		&subscription.CanceledAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	return subscription, nil
}

// GetSubscription retrieves a subscription by ID
func (m *Manager) GetSubscription(ctx context.Context, id uuid.UUID) (*Subscription, error) {
	if id == uuid.Nil {
		return nil, errors.New("invalid subscription ID")
	}

	var subscription Subscription

	query := `
		SELECT id, tenant_id, plan_id, status, current_period_start,
			current_period_end, trial_start, trial_end, period, auto_renew,
			cancel_at_period_end, default_payment_method_id, base_price,
			currency, tax_rate, usage_based_pricing, usage_thresholds,
			metadata, created_at, updated_at, canceled_at
		FROM subscriptions
		WHERE id = $1 AND deleted_at IS NULL
	`

	err := m.db.QueryRowContext(ctx, query, id).Scan(
		&subscription.ID,
		&subscription.TenantID,
		&subscription.PlanID,
		&subscription.Status,
		&subscription.CurrentPeriodStart,
		&subscription.CurrentPeriodEnd,
		&subscription.TrialStart,
		&subscription.TrialEnd,
		&subscription.Period,
		&subscription.AutoRenew,
		&subscription.CancelAtPeriodEnd,
		&subscription.DefaultPaymentMethodID,
		&subscription.BasePrice,
		&subscription.Currency,
		&subscription.TaxRate,
		&subscription.UsageBasedPricing,
		&subscription.UsageThresholds,
		&subscription.Metadata,
		&subscription.CreatedAt,
		&subscription.UpdatedAt,
		&subscription.CanceledAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	return &subscription, nil
}

// GetSubscriptionByTenant retrieves the subscription for a tenant
func (m *Manager) GetSubscriptionByTenant(ctx context.Context, tenantID uuid.UUID) (*Subscription, error) {
	if tenantID == uuid.Nil {
		return nil, errors.New("invalid tenant ID")
	}

	var subscription Subscription

	query := `
		SELECT id, tenant_id, plan_id, status, current_period_start,
			current_period_end, trial_start, trial_end, period, auto_renew,
			cancel_at_period_end, default_payment_method_id, base_price,
			currency, tax_rate, usage_based_pricing, usage_thresholds,
			metadata, created_at, updated_at, canceled_at
		FROM subscriptions
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`

	err := m.db.QueryRowContext(ctx, query, tenantID).Scan(
		&subscription.ID,
		&subscription.TenantID,
		&subscription.PlanID,
		&subscription.Status,
		&subscription.CurrentPeriodStart,
		&subscription.CurrentPeriodEnd,
		&subscription.TrialStart,
		&subscription.TrialEnd,
		&subscription.Period,
		&subscription.AutoRenew,
		&subscription.CancelAtPeriodEnd,
		&subscription.DefaultPaymentMethodID,
		&subscription.BasePrice,
		&subscription.Currency,
		&subscription.TaxRate,
		&subscription.UsageBasedPricing,
		&subscription.UsageThresholds,
		&subscription.Metadata,
		&subscription.CreatedAt,
		&subscription.UpdatedAt,
		&subscription.CanceledAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("failed to get subscription by tenant: %w", err)
	}

	return &subscription, nil
}

// UpdateSubscription updates a subscription
func (m *Manager) UpdateSubscription(ctx context.Context, id uuid.UUID, req UpdateSubscriptionRequest) (*Subscription, error) {
	if id == uuid.Nil {
		return nil, errors.New("invalid subscription ID")
	}

	subscription, err := m.GetSubscription(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if req.PlanID != nil {
		subscription.PlanID = *req.PlanID
	}
	if req.Status != nil {
		subscription.Status = *req.Status
	}
	if req.Period != nil {
		subscription.Period = *req.Period
	}
	if req.AutoRenew != nil {
		subscription.AutoRenew = *req.AutoRenew
	}
	if req.CancelAtPeriodEnd != nil {
		subscription.CancelAtPeriodEnd = *req.CancelAtPeriodEnd
	}
	if req.DefaultPaymentMethodID != nil {
		subscription.DefaultPaymentMethodID = req.DefaultPaymentMethodID
	}
	if req.UsageBasedPricing != nil {
		subscription.UsageBasedPricing = *req.UsageBasedPricing
	}
	subscription.UpdatedAt = time.Now()

	// Serialize
	usageThresholdsJSON := []byte("{}")
	if subscription.UsageThresholds != nil {
		usageThresholdsJSON = subscription.UsageThresholds
	}

	query := `
		UPDATE subscriptions
		SET plan_id = $1, status = $2, period = $3, auto_renew = $4,
			cancel_at_period_end = $5, default_payment_method_id = $6,
			usage_based_pricing = $7, updated_at = CURRENT_TIMESTAMP
		WHERE id = $8
		RETURNING id, tenant_id, plan_id, status, current_period_start,
			current_period_end, trial_start, trial_end, period, auto_renew,
			cancel_at_period_end, default_payment_method_id, base_price,
			currency, tax_rate, usage_based_pricing, usage_thresholds,
			metadata, created_at, updated_at, canceled_at
	`

	err = m.db.QueryRowContext(ctx, query,
		subscription.PlanID,
		subscription.Status,
		subscription.Period,
		subscription.AutoRenew,
		subscription.CancelAtPeriodEnd,
		subscription.DefaultPaymentMethodID,
		subscription.UsageBasedPricing,
		subscription.ID,
	).Scan(
		&subscription.ID,
		&subscription.TenantID,
		&subscription.PlanID,
		&subscription.Status,
		&subscription.CurrentPeriodStart,
		&subscription.CurrentPeriodEnd,
		&subscription.TrialStart,
		&subscription.TrialEnd,
		&subscription.Period,
		&subscription.AutoRenew,
		&subscription.CancelAtPeriodEnd,
		&subscription.DefaultPaymentMethodID,
		&subscription.BasePrice,
		&subscription.Currency,
		&subscription.TaxRate,
		&subscription.UsageBasedPricing,
		&usageThresholdsJSON,
		&subscription.Metadata,
		&subscription.CreatedAt,
		&subscription.UpdatedAt,
		&subscription.CanceledAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}

	subscription.UsageThresholds = usageThresholdsJSON

	return subscription, nil
}

// CancelSubscription cancels a subscription
func (m *Manager) CancelSubscription(ctx context.Context, id uuid.UUID, cancelAtPeriodEnd bool) error {
	if id == uuid.Nil {
		return errors.New("invalid subscription ID")
	}

	subscription, err := m.GetSubscription(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()

	if cancelAtPeriodEnd {
		// Mark for cancellation at period end
		query := `
			UPDATE subscriptions
			SET cancel_at_period_end = true, updated_at = $1
			WHERE id = $2
		`

		_, err = m.db.ExecContext(ctx, query, now, id)
		if err != nil {
			return fmt.Errorf("failed to cancel subscription: %w", err)
		}
	} else {
		// Cancel immediately
		query := `
			UPDATE subscriptions
			SET status = 'canceled', canceled_at = $1, updated_at = $1
			WHERE id = $2
		`

		_, err = m.db.ExecContext(ctx, query, now, id)
		if err != nil {
			return fmt.Errorf("failed to cancel subscription: %w", err)
		}
	}

	return nil
}

// RenewSubscription renews a subscription
func (m *Manager) RenewSubscription(ctx context.Context, id uuid.UUID) (*Subscription, error) {
	if id == uuid.Nil {
		return nil, errors.New("invalid subscription ID")
	}

	subscription, err := m.GetSubscription(ctx, id)
	if err != nil {
		return nil, err
	}

	// Calculate new period
	oldEnd := subscription.CurrentPeriodEnd
	newStart := oldEnd
	var newEnd time.Time

	switch subscription.Period {
	case BillingPeriodMonthly:
		newEnd = newStart.AddDate(0, 1, 0)
	case BillingPeriodQuarterly:
		newEnd = newStart.AddDate(0, 3, 0)
	case BillingPeriodAnnually:
		newEnd = newStart.AddDate(1, 0, 0)
	default:
		return nil, ErrInvalidBillingPeriod
	}

	// Update subscription period
	query := `
		UPDATE subscriptions
		SET current_period_start = $1, current_period_end = $2,
			status = 'active', updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
		RETURNING id, tenant_id, plan_id, status, current_period_start,
			current_period_end, trial_start, trial_end, period, auto_renew,
			cancel_at_period_end, default_payment_method_id, base_price,
			currency, tax_rate, usage_based_pricing, usage_thresholds,
			metadata, created_at, updated_at, canceled_at
	`

	err = m.db.QueryRowContext(ctx, query, newStart, newEnd, id).Scan(
		&subscription.ID,
		&subscription.TenantID,
		&subscription.PlanID,
		&subscription.Status,
		&subscription.CurrentPeriodStart,
		&subscription.CurrentPeriodEnd,
		&subscription.TrialStart,
		&subscription.TrialEnd,
		&subscription.Period,
		&subscription.AutoRenew,
		&subscription.CancelAtPeriodEnd,
		&subscription.DefaultPaymentMethodID,
		&subscription.BasePrice,
		&subscription.Currency,
		&subscription.TaxRate,
		&subscription.UsageBasedPricing,
		&subscription.UsageThresholds,
		&subscription.Metadata,
		&subscription.CreatedAt,
		&subscription.UpdatedAt,
		&subscription.CanceledAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to renew subscription: %w", err)
	}

	return subscription, nil
}

// ProcessPayment processes a payment for a subscription
func (m *Manager) ProcessPayment(ctx context.Context, subscriptionID uuid.UUID, amount float64) (*PaymentTransaction, error) {
	if subscriptionID == uuid.Nil {
		return nil, errors.New("invalid subscription ID")
	}

	subscription, err := m.GetSubscription(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}

	transaction := &PaymentTransaction{
		ID:             uuid.New(),
		SubscriptionID: subscriptionID,
		TenantID:       subscription.TenantID,
		Amount:         amount,
		Currency:       subscription.Currency,
		Status:         "pending",
		PaymentMethodID: *subscription.DefaultPaymentMethodID,
		Provider:       "stripe", // Default provider
		Metadata:       []byte("{}"),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	query := `
		INSERT INTO payment_transactions (id, subscription_id, invoice_id, tenant_id,
			amount, currency, status, payment_method_id, provider,
			provider_id, refunded_amount, refunded_at, failure_reason,
			metadata, created_at, updated_at, completed_at)
		VALUES ($1, $2, NULL, $3, $4, $5, $6, $7, $8, $9, NULL, NULL, NULL,
			$10, $11, $12, NULL)
		RETURNING id, subscription_id, invoice_id, tenant_id, amount, currency,
			status, payment_method_id, provider, provider_id,
			refunded_amount, refunded_at, failure_reason, metadata,
			created_at, updated_at, completed_at
	`

	err = m.db.QueryRowContext(ctx, query,
		transaction.ID,
		transaction.SubscriptionID,
		transaction.TenantID,
		transaction.Amount,
		transaction.Currency,
		transaction.Status,
		transaction.PaymentMethodID,
		transaction.Provider,
		transaction.Metadata,
		transaction.CreatedAt,
		transaction.UpdatedAt,
	).Scan(
		&transaction.ID,
		&transaction.SubscriptionID,
		&transaction.InvoiceID,
		&transaction.TenantID,
		&transaction.Amount,
		&transaction.Currency,
		&transaction.Status,
		&transaction.PaymentMethodID,
		&transaction.Provider,
		&transaction.ProviderID,
		&transaction.RefundedAmount,
		&transaction.RefundedAt,
		&transaction.FailureReason,
		&transaction.Metadata,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
		&transaction.CompletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create payment transaction: %w", err)
	}

	return transaction, nil
}

// GetPaymentTransaction retrieves a payment transaction by ID
func (m *Manager) GetPaymentTransaction(ctx context.Context, id uuid.UUID) (*PaymentTransaction, error) {
	if id == uuid.Nil {
		return nil, errors.New("invalid transaction ID")
	}

	var transaction PaymentTransaction

	query := `
		SELECT id, subscription_id, invoice_id, tenant_id, amount, currency,
			status, payment_method_id, provider, provider_id, refunded_amount,
			refunded_at, failure_reason, metadata, created_at, updated_at,
			completed_at
		FROM payment_transactions
		WHERE id = $1
	`

	err := m.db.QueryRowContext(ctx, query, id).Scan(
		&transaction.ID,
		&transaction.SubscriptionID,
		&transaction.InvoiceID,
		&transaction.TenantID,
		&transaction.Amount,
		&transaction.Currency,
		&transaction.Status,
		&transaction.PaymentMethodID,
		&transaction.Provider,
		&transaction.ProviderID,
		&transaction.RefundedAmount,
		&transaction.RefundedAt,
		&transaction.FailureReason,
		&transaction.Metadata,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
		&transaction.CompletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("transaction not found")
		}
		return nil, fmt.Errorf("failed to get payment transaction: %w", err)
	}

	return &transaction, nil
}

// AddPaymentMethod adds a new payment method
func (m *Manager) AddPaymentMethod(ctx context.Context, tenantID uuid.UUID, paymentMethod *PaymentMethod) error {
	if tenantID == uuid.Nil {
		return errors.New("invalid tenant ID")
	}
	if paymentMethod.Type == "" {
		return errors.New("payment method type is required")
	}
	if paymentMethod.Provider == "" {
		return errors.New("payment provider is required")
	}

	paymentMethod.ID = uuid.New()
	paymentMethod.TenantID = tenantID
	paymentMethod.IsDefault = false
	paymentMethod.Status = "active"
	paymentMethod.CreatedAt = time.Now()
	paymentMethod.UpdatedAt = time.Now()

	query := `
		INSERT INTO payment_methods (id, tenant_id, type, provider, provider_id,
			card_brand, card_last_four, card_expiry_month, card_expiry_year,
			is_default, status, metadata, created_at, updated_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`

	metadata := []byte("{}")
	if paymentMethod.Metadata != nil {
		metadata = paymentMethod.Metadata
	}

	_, err := m.db.ExecContext(ctx, query,
		paymentMethod.ID,
		paymentMethod.TenantID,
		paymentMethod.Type,
		paymentMethod.Provider,
		paymentMethod.ProviderID,
		paymentMethod.CardBrand,
		paymentMethod.CardLastFour,
		paymentMethod.CardExpiryMonth,
		paymentMethod.CardExpiryYear,
		paymentMethod.IsDefault,
		paymentMethod.Status,
		metadata,
		paymentMethod.CreatedAt,
		paymentMethod.UpdatedAt,
		paymentMethod.ExpiresAt,
	)

	if err != nil {
		return fmt.Errorf("failed to add payment method: %w", err)
	}

	return nil
}

// GetPaymentMethods retrieves all payment methods for a tenant
func (m *Manager) GetPaymentMethods(ctx context.Context, tenantID uuid.UUID) ([]*PaymentMethod, error) {
	if tenantID == uuid.Nil {
		return nil, errors.New("invalid tenant ID")
	}

	query := `
		SELECT id, tenant_id, type, provider, provider_id, card_brand,
			card_last_four, card_expiry_month, card_expiry_year,
			is_default, status, metadata, created_at, updated_at, expires_at
		FROM payment_methods
		WHERE tenant_id = $1 AND status != 'deleted'
		ORDER BY is_default DESC, created_at DESC
	`

	rows, err := m.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment methods: %w", err)
	}
	defer rows.Close()

	var methods []*PaymentMethod
	for rows.Next() {
		var method PaymentMethod

		err := rows.Scan(
			&method.ID,
			&method.TenantID,
			&method.Type,
			&method.Provider,
			&method.ProviderID,
			&method.CardBrand,
			&method.CardLastFour,
			&method.CardExpiryMonth,
			&method.CardExpiryYear,
			&method.IsDefault,
			&method.Status,
			&method.Metadata,
			&method.CreatedAt,
			&method.UpdatedAt,
			&method.ExpiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payment method: %w", err)
		}

		methods = append(methods, &method)
	}

	return methods, nil
}

// SetDefaultPaymentMethod sets a payment method as default
func (m *Manager) SetDefaultPaymentMethod(ctx context.Context, tenantID, methodID uuid.UUID) error {
	if tenantID == uuid.Nil || methodID == uuid.Nil {
		return errors.New("invalid IDs")
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Remove default from all methods
	_, err = tx.ExecContext(ctx, `
		UPDATE payment_methods SET is_default = false WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		return fmt.Errorf("failed to clear default payment method: %w", err)
	}

	// Set new default
	_, err = tx.ExecContext(ctx, `
		UPDATE payment_methods SET is_default = true WHERE id = $1 AND tenant_id = $2
	`, methodID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to set default payment method: %w", err)
	}

	// Update subscription's default payment method
	_, err = tx.ExecContext(ctx, `
		UPDATE subscriptions SET default_payment_method_id = $1
		WHERE tenant_id = $2 AND status = 'active'
	`, methodID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DeletePaymentMethod deletes a payment method
func (m *Manager) DeletePaymentMethod(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return errors.New("invalid payment method ID")
	}

	query := `UPDATE payment_methods SET status = 'deleted' WHERE id = $1`

	_, err := m.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete payment method: %w", err)
	}

	return nil
}

// GetBillingSummary returns billing summary for a tenant
func (m *Manager) GetBillingSummary(ctx context.Context, tenantID uuid.UUID) (map[string]interface{}, error) {
	if tenantID == uuid.Nil {
		return nil, errors.New("invalid tenant ID")
	}

	summary := make(map[string]interface{})

	// Get subscription
	subscription, err := m.GetSubscriptionByTenant(ctx, tenantID)
	if err == nil {
		summary["subscription"] = subscription
	}

	// Get current billing period
	if subscription != nil {
		summary["current_period_start"] = subscription.CurrentPeriodStart
		summary["current_period_end"] = subscription.CurrentPeriodEnd
		summary["auto_renew"] = subscription.AutoRenew

		// Calculate days until renewal
		daysUntilRenewal := int(subscription.CurrentPeriodEnd.Sub(time.Now()).Hours() / 24)
		summary["days_until_renewal"] = daysUntilRenewal
	}

	// Get payment methods
	methods, err := m.GetPaymentMethods(ctx, tenantID)
	if err == nil {
		summary["payment_methods"] = methods
		summary["payment_method_count"] = len(methods)
	}

	// Get recent transactions
	query := `
		SELECT COUNT(*), SUM(amount) as total_spent
		FROM payment_transactions
		WHERE tenant_id = $1 AND status = 'completed'
		AND created_at >= CURRENT_TIMESTAMP - INTERVAL '30 days'
	`

	var transactionCount int
	var totalSpent float64
	err = m.db.QueryRowContext(ctx, query, tenantID).Scan(&transactionCount, &totalSpent)
	if err == nil {
		summary["recent_transaction_count"] = transactionCount
		summary["recent_total_spent"] = totalSpent
	}

	summary["currency"] = "USD"

	return summary, nil
}

// getPlan retrieves plan details
func (m *Manager) getPlan(ctx context.Context, planID uuid.UUID) (*Plan, error) {
	var plan Plan

	query := `
		SELECT id, name, description, base_price, currency, period,
			features, metadata, active, created_at, updated_at
		FROM plans
		WHERE id = $1 AND deleted_at IS NULL
	`

	err := m.db.QueryRowContext(ctx, query, planID).Scan(
		&plan.ID,
		&plan.Name,
		&plan.Description,
		&plan.BasePrice,
		&plan.Currency,
		&plan.Period,
		&plan.Features,
		&plan.Metadata,
		&plan.Active,
		&plan.CreatedAt,
		&plan.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	return &plan, nil
}

// CheckRenewals checks and processes subscription renewals
func (m *Manager) CheckRenewals(ctx context.Context) (int, error) {
	renewed := 0

	// Find subscriptions that need renewal
	query := `
		SELECT id, tenant_id, plan_id, period
		FROM subscriptions
		WHERE status = 'active'
			AND current_period_end <= CURRENT_TIMESTAMP
			AND auto_renew = true
			AND cancel_at_period_end = false
		ORDER BY current_period_end ASC
	`

	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to find renewals: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, tenantID, planID uuid.UUID
		var period BillingPeriod

		err := rows.Scan(&id, &tenantID, &planID, &period)
		if err != nil {
			continue
		}

		_, err = m.RenewSubscription(ctx, id)
		if err != nil {
			// Log error but continue
			continue
		}

		renewed++
	}

	return renewed, nil
}

// CalculateBillableAmount calculates billable amount based on usage
func (m *Manager) CalculateBillableAmount(ctx context.Context, subscriptionID uuid.UUID, startDate, endDate time.Time) (float64, error) {
	subscription, err := m.GetSubscription(ctx, subscriptionID)
	if err != nil {
		return 0, err
	}

	if !subscription.UsageBasedPricing {
		// Fixed pricing
		return subscription.BasePrice, nil
	}

	// Usage-based pricing - calculate based on actual usage
	// This would integrate with the usage tracking module
	// For now, return base price
	return subscription.BasePrice, nil
}
