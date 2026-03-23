// internal/billing/stripe.go
// Stripe payment integration
// Handles Stripe payments, webhooks, customers, and subscriptions

package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v76"
)

var (
	ErrStripeClientNotFound     = errors.New("stripe client not found")
	ErrStripePaymentFailed     = errors.New("stripe payment failed")
	ErrStripeWebhookInvalid   = errors.New("invalid stripe webhook")
	ErrStripeCustomerNotFound  = errors.New("stripe customer not found")
	ErrStripeSubscriptionNotFound = errors.New("stripe subscription not found")
)

// StripeClient wraps the Stripe API client
type StripeClient struct {
	apiKey         string
	client         *stripe.Client
	webhookSecret string
	baseURL       string
}

// StripeConfig represents Stripe configuration
type StripeConfig struct {
	APIKey         string `json:"api_key"`
	WebhookSecret  string `json:"webhook_secret"`
	PublishableKey string `json:"publishable_key"`
	WebhookURL     string `json:"webhook_url"`
}

// StripeCustomer represents a Stripe customer
type StripeCustomer struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Phone          string `json:"phone"`
	Address        *Address `json:"address,omitempty"`
	DefaultPaymentMethod string `json:"default_payment_method"`
	Metadata       map[string]string `json:"metadata"`
	CreatedAt      int64  `json:"created_at"`
}

// StripePaymentMethod represents a payment method in Stripe
type StripePaymentMethod struct {
	ID           string `json:"id"`
	Type         string `json:"type"` // card, bank_account, etc.
	CustomerID   string `json:"customer_id"`
	CardLastFour string `json:"card_last_four"`
	CardBrand    string `json:"card_brand"`
	CardExpiry   string `json:"card_expiry"`
	IsDefault    bool   `json:"is_default"`
	Metadata     map[string]string `json:"metadata"`
}

// StripeCharge represents a payment charge
type StripeCharge struct {
	ID            string  `json:"id"`
	Amount        int64   `json:"amount"`
	Currency      string  `json:"currency"`
	CustomerID    string  `json:"customer_id"`
	PaymentIntent string  `json:"payment_intent"`
	Status        string  `json:"status"`
	ReceiptURL    string  `json:"receipt_url"`
	FailureReason string  `json:"failure_reason,omitempty"`
	Metadata      map[string]string `json:"metadata"`
	CreatedAt     int64   `json:"created_at"`
}

// StripeSubscription represents a Stripe subscription
type StripeSubscription struct {
	ID              string          `json:"id"`
	CustomerID      string          `json:"customer_id"`
	PriceID         string          `json:"price_id"`
	Status          string          `json:"status"`
	CurrentPeriodStart int64        `json:"current_period_start"`
	CurrentPeriodEnd   int64        `json:"current_period_end"`
	TrialStart      *int64         `json:"trial_start,omitempty"`
	TrialEnd        *int64         `json:"trial_end,omitempty"`
	CancelAt        *int64         `json:"cancel_at,omitempty"`
	Quantity        int64           `json:"quantity"`
	Metadata        map[string]string `json:"metadata"`
}

// StripeInvoice represents an invoice from Stripe
type StripeInvoice struct {
	ID                string  `json:"id"`
	CustomerID        string  `json:"customer_id"`
	SubscriptionID    string  `json:"subscription,omitempty"`
	Status            string  `json:"status"`
	AmountDue         int64   `json:"amount_due"`
	AmountPaid        int64   `json:"amount_paid"`
	AmountRemaining   int64   `json:"amount_remaining"`
	Currency          string  `json:"currency"`
	InvoicePDF       string  `json:"invoice_pdf"`
	Number            string  `json:"number"`
	PeriodStart       int64  `json:"period_start"`
	PeriodEnd         int64  `json:"period_end"`
	Subscription       *StripeSubscription `json:"subscription,omitempty"`
	Lines             []StripeInvoiceLine `json:"lines"`
}

// StripeInvoiceLine represents a line item in a Stripe invoice
type StripeInvoiceLine struct {
	ID             string  `json:"id"`
	InvoiceID      string  `json:"invoice"`
	Amount         int64  `json:"amount"`
	Currency       string  `json:"currency"`
	Description     string  `json:"description"`
	Quantity       int64  `json:"quantity"`
	Period         *StripeInvoicePeriod `json:"period,omitempty"`
	Price          *StripePrice `json:"price,omitempty"`
}

// StripeInvoicePeriod represents the billing period
type StripeInvoicePeriod struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// StripePrice represents a price object
type StripePrice struct {
	ID            string           `json:"id"`
	ProductID     string           `json:"product_id"`
	Amount        int64            `json:"amount"` // in cents
	Currency      string           `json:"currency"`
	Interval      stripe.PriceRecurringInterval `json:"interval"`
	Active        bool             `json:"active"`
	Metadata      map[string]string `json:"metadata"`
}

// StripeWebhookEvent represents a webhook event from Stripe
type StripeWebhookEvent struct {
	ID         string              `json:"id"`
	Type       string              `json:"type"` // payment_intent.succeeded, invoice.payment_succeeded, etc.
	APIVersion string              `json:"api_version"`
	Data       json.RawMessage     `json:"data"`
	Created    int64              `json:"created"`
}

// CreateCustomerRequest represents a request to create a Stripe customer
type CreateCustomerRequest struct {
	Email            string            `json:"email"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Phone            string            `json:"phone"`
	Address          *Address          `json:"address,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	PaymentMethodID string            `json:"payment_method_id,omitempty"`
}

// CreatePaymentIntentRequest represents a request to create a payment intent
type CreatePaymentIntentRequest struct {
	Amount           int64             `json:"amount"`
	Currency         string            `json:"currency"`
	CustomerID       string            `json:"customer_id"`
	PaymentMethodID  string            `json:"payment_method_id,omitempty"`
	Description      string            `json:"description"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// CreateSubscriptionRequest represents a request to create a Stripe subscription
type CreateSubscriptionRequest struct {
	CustomerID       string            `json:"customer_id"`
	PriceID          string            `json:"price_id"`
	TrialPeriodDays int64            `json:"trial_period_days,omitempty"`
	Quantity         int64            `json:"quantity"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// CreateSetupIntentRequest represents a request for subscription setup
type CreateSetupIntentRequest struct {
	CustomerID     string `json:"customer_id"`
	PaymentMethodID string `json:"payment_method,omitempty"`
	PriceID        string `json:"price_id"`
	TrialDays      int64  `json:"trial_days"`
}

// Address represents an address
type Address struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

// NewStripeClient creates a new Stripe client
func NewStripeClient(config *StripeConfig) *StripeClient {
	return &StripeClient{
		apiKey:         config.APIKey,
		webhookSecret:  config.WebhookSecret,
		client:         stripe.New(config.APIKey, nil),
	}
}

// CreateCustomer creates a new Stripe customer
func (sc *StripeClient) CreateCustomer(ctx context.Context, req CreateCustomerRequest) (*StripeCustomer, error) {
	// Build customer parameters
	params := &stripe.CustomerParams{
		Email:       stripe.String(req.Email),
		Name:         stripe.String(req.Name),
		Description:  stripe.String(req.Description),
		Phone:       stripe.String(req.Phone),
		Address:      sc.buildAddressParams(req.Address),
		Metadata:    sc.buildMetadata(req.Metadata),
		PaymentMethod: stripe.String(req.PaymentMethodID),
	}

	// Create customer in Stripe
	customer, err := sc.client.Customers.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create Stripe customer: %w", err)
	}

	return &StripeCustomer{
		ID:             customer.ID,
		Email:          customer.Email,
		Name:           customer.Name,
		Description:    customer.Description,
		Phone:          customer.Phone,
		DefaultPaymentMethod: customer.DefaultSource,
		Metadata:       customer.Metadata,
		CreatedAt:      customer.Created,
	}, nil
}

// GetCustomer retrieves a customer by ID
func (sc *StripeClient) GetCustomer(ctx context.Context, customerID string) (*StripeCustomer, error) {
	customer, err := sc.client.Customers.Get(customerID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Stripe customer: %w", err)
	}

	return &StripeCustomer{
		ID:             customer.ID,
		Email:          customer.Email,
		Name:           customer.Name,
		Description:    customer.Description,
		Phone:          customer.Phone,
		DefaultPaymentMethod: customer.DefaultSource,
		Metadata:       customer.Metadata,
		CreatedAt:      customer.Created,
	}, nil
}

// UpdateCustomer updates an existing customer
func (sc *StripeClient) UpdateCustomer(ctx context.Context, customerID string, req CreateCustomerRequest) (*StripeCustomer, error) {
	params := &stripe.CustomerParams{
		Email:       stripe.String(req.Email),
		Name:         stripe.String(req.Name),
		Description:  stripe.String(req.Description),
		Phone:       stripe.String(req.Phone),
		Address:      sc.buildAddressParams(req.Address),
		Metadata:    sc.buildMetadata(req.Metadata),
	}

	customer, err := sc.client.Customers.Update(customerID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to update Stripe customer: %w", err)
	}

	return &StripeCustomer{
		ID:             customer.ID,
		Email:          customer.Email,
		Name:           customer.Name,
		Description:    customer.Description,
		Phone:          customer.Phone,
		DefaultPaymentMethod: customer.DefaultSource,
		Metadata:       customer.Metadata,
		CreatedAt:      customer.Created,
	}, nil
}

// DeleteCustomer deletes a customer from Stripe
func (sc *StripeClient) DeleteCustomer(ctx context.Context, customerID string) error {
	_, err := sc.client.Customers.Del(customerID, nil)
	if err != nil {
		return fmt.Errorf("failed to delete Stripe customer: %w", err)
	}

	return nil
}

// CreatePaymentIntent creates a payment intent
func (sc *StripeClient) CreatePaymentIntent(ctx context.Context, req CreatePaymentIntentRequest) (*stripe.PaymentIntent, error) {
	params := &stripe.PaymentIntentParams{
		Amount:             stripe.Int64(req.Amount),
		Currency:           stripe.String(req.Currency),
		Customer:           stripe.String(req.CustomerID),
		PaymentMethod:       stripe.String(req.PaymentMethodID),
		Description:         stripe.String(req.Description),
		Metadata:           sc.buildMetadata(req.Metadata),
		Confirm:            stripe.Bool(true),
	}

	paymentIntent, err := sc.client.PaymentIntents.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment intent: %w", err)
	}

	return paymentIntent, nil
}

// GetPaymentIntent retrieves a payment intent
func (sc *StripeClient) GetPaymentIntent(ctx context.Context, paymentIntentID string) (*stripe.PaymentIntent, error) {
	paymentIntent, err := sc.client.PaymentIntents.Get(paymentIntentID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment intent: %w", err)
	}

	return paymentIntent, nil
}

// ConfirmPaymentIntent confirms a payment intent
func (sc *StripeClient) ConfirmPaymentIntent(ctx context.Context, paymentIntentID string) (*stripe.PaymentIntent, error) {
	params := &stripe.PaymentIntentConfirmParams{
		PaymentMethod: stripe.String("pm_card_visa"), // In real app, get from client
	}

	paymentIntent, err := sc.client.PaymentIntents.Confirm(paymentIntentID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to confirm payment intent: %w", err)
	}

	return paymentIntent, nil
}

// CancelPaymentIntent cancels a payment intent
func (sc *StripeClient) CancelPaymentIntent(ctx context.Context, paymentIntentID string) (*stripe.PaymentIntent, error) {
	params := &stripe.PaymentIntentCancelParams{
		CancellationReason: stripe.String("requested_by_customer"),
	}

	paymentIntent, err := sc.client.PaymentIntents.Cancel(paymentIntentID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to cancel payment intent: %w", err)
	}

	return paymentIntent, nil
}

// CreateSubscription creates a new subscription
func (sc *StripeClient) CreateSubscription(ctx context.Context, req CreateSubscriptionRequest) (*stripe.Subscription, error) {
	params := &stripe.SubscriptionParams{
		Customer: stripe.String(req.CustomerID),
		Price:    stripe.String(req.PriceID),
		Quantity:  stripe.Int64(req.Quantity),
		Metadata: sc.buildMetadata(req.Metadata),
		TrialEnd: sc.buildTimestamp(req.TrialPeriodDays),
	}

	subscription, err := sc.client.Subscriptions.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create Stripe subscription: %w", err)
	}

	return subscription, nil
}

// GetSubscription retrieves a subscription by ID
func (sc *StripeClient) GetSubscription(ctx context.Context, subscriptionID string) (*stripe.Subscription, error) {
	subscription, err := sc.client.Subscriptions.Get(subscriptionID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Stripe subscription: %w", err)
	}

	return subscription, nil
}

// UpdateSubscription updates a subscription
func (sc *StripeClient) UpdateSubscription(ctx context.Context, subscriptionID string, params *stripe.SubscriptionParams) (*stripe.Subscription, error) {
	subscription, err := sc.client.Subscriptions.Update(subscriptionID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to update Stripe subscription: %w", err)
	}

	return subscription, nil
}

// CancelSubscription cancels a subscription
func (sc *StripeClient) CancelSubscription(ctx context.Context, subscriptionID string) error {
	params := &stripe.SubscriptionCancelParams{
		CancelAtPeriodEnd: stripe.Bool(false),
	}

	_, err := sc.client.Subscriptions.Cancel(subscriptionID, params)
	if err != nil {
		return fmt.Errorf("failed to cancel Stripe subscription: %w", err)
	}

	return nil
}

// CreateSetupIntent creates a setup intent for subscriptions
func (sc *StripeClient) CreateSetupIntent(ctx context.Context, req *CreateSetupIntentRequest) (*stripe.SetupIntent, error) {
	// Create a subscription item
	items := []*stripe.SetupIntentItemParams{
		{
			Price: stripe.String(req.PriceID),
			Quantity: stripe.Int64(1),
			Metadata: sc.buildMetadata(req.Metadata),
		},
	}

	params := &stripe.SetupIntentParams{
		PaymentMethodTypes: stripe.Strings("card"),
		PaymentMethod:      stripe.String(req.PaymentMethodID),
		Customer:          stripe.String(req.CustomerID),
		Items:              items,
		TrialPeriod:        sc.buildTrialPeriod(req.TrialDays),
		Mode:               stripe.String("subscription"),
	}

	setupIntent, err := sc.client.SetupIntents.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create setup intent: %w", err)
	}

	return setupIntent, nil
}

// GetSetupIntent retrieves a setup intent
func (sc *StripeClient) GetSetupIntent(ctx context.Context, setupIntentID string) (*stripe.SetupIntent, error) {
	setupIntent, err := sc.client.SetupIntents.Get(setupIntentID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get setup intent: %w", err)
	}

	return setupIntent, nil
}

// CreatePrice creates a new price
func (sc *StripeClient) CreatePrice(ctx context.Context, amount int64, currency, productID, interval string) (*stripe.Price, error) {
	params := &stripe.PriceParams{
		Currency: stripe.String(currency),
		Product:  stripe.String(productID),
		UnitAmount: stripe.Int64(amount),
		Interval:  stripe.String(interval),
	}

	price, err := sc.client.Prices.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create Stripe price: %w", err)
	}

	return price, nil
}

// GetPrice retrieves a price by ID
func (sc *StripeClient) GetPrice(ctx context.Context, priceID string) (*stripe.Price, error) {
	price, err := sc.client.Prices.Get(priceID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Stripe price: %w", err)
	}

	return price, nil
}

// GetInvoice retrieves an invoice from Stripe
func (sc *StripeClient) GetInvoice(ctx context.Context, invoiceID string) (*stripe.Invoice, error) {
	invoice, err := sc.client.Invoices.Get(invoiceID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Stripe invoice: %w", err)
	}

	return invoice, nil
}

// ListInvoices retrieves invoices for a customer
func (sc *StripeClient) ListInvoices(ctx context.Context, customerID string, limit int64) ([]*stripe.Invoice, error) {
	params := &stripe.InvoiceListParams{
		Customer: stripe.String(customerID),
		Limit:     stripe.Int64(limit),
	}

	invoices := stripe.InvoiceList{}
	iter := sc.client.Invoices.List(params)
	for iter.Next() {
		invoices = append(invoices, iter.Invoice())
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to list Stripe invoices: %w", err)
	}

	return invoices, nil
}

// CreateInvoice creates an invoice
func (sc *StripeClient) CreateInvoice(ctx context.Context, customerID, subscriptionID string, description string, metadata map[string]string) (*stripe.Invoice, error) {
	params := &stripe.InvoiceParams{
		Customer:       stripe.String(customerID),
		Subscription:    stripe.String(subscriptionID),
		Description:    stripe.String(description),
		Metadata:       sc.buildMetadata(metadata),
	}

	invoice, err := sc.client.Invoices.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create Stripe invoice: %w", err)
	}

	return invoice, nil
}

// VerifyWebhookSignature verifies a Stripe webhook signature
func (sc *StripeClient) VerifyWebhookSignature(payload []byte, signature, timestamp string) bool {
	// In production, use Stripe's webhook signing verification
	// For now, this is a placeholder
	return true
}

// ParseWebhookEvent parses a webhook event from Stripe
func (sc *StripeClient) ParseWebhookEvent(r io.Reader) (*StripeWebhookEvent, error) {
	payload, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read webhook payload: %w", err)
	}

	var event map[string]interface{}
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal webhook payload: %w", err)
	}

	return &StripeWebhookEvent{
		ID:         event["id"].(string),
		Type:       event["type"].(string),
		APIVersion: event["api_version"].(string),
		Data:       payload,
		Created:    int64(time.Now().Unix()),
	}, nil
}

// HandlePaymentIntentSucceeded handles a successful payment
func (sc *StripeClient) HandlePaymentIntentSucceeded(ctx context.Context, event *StripeWebhookEvent, tenantID uuid.UUID) error {
	// Extract payment intent data
	var paymentIntent stripe.PaymentIntent
	if err := json.Unmarshal(event.Data, &paymentIntent); err != nil {
		return fmt.Errorf("failed to unmarshal payment intent: %w", err)
	}

	// Update payment transaction status
	// In a real implementation, you'd update your database with:
	// - Payment transaction status = "completed"
	// - Payment reference = paymentIntent.ID
	// - Receipt URL = paymentIntent.Charges.Data[0].ReceiptURL

	return nil
}

// HandleInvoicePaymentSucceeded handles a successful invoice payment
func (sc *StripeClient) HandleInvoicePaymentSucceeded(ctx context.Context, event *StripeWebhookEvent, tenantID uuid.UUID) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(event.Data, &invoice); err != nil {
		return fmt.Errorf("failed to unmarshal invoice: %w", err)
	}

	// Update invoice status in your database
	// - Set status = "paid"
	// - Set paid_at timestamp
	// - Set payment_reference = invoice.Charges.Data[0].PaymentIntent

	return nil
}

// HandleSubscriptionUpdated handles subscription updates
func (sc *StripeClient) HandleSubscriptionUpdated(ctx context.Context, event *StripeWebhookEvent, tenantID uuid.UUID) error {
	var subscription stripe.Subscription
	if err := json.Unmarshal(event.Data, &subscription); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	// Update subscription in your database
	// - Update status
	// - Update current_period_start/end
	// - Handle trial_start/trial_end

	return nil
}

// HandleCustomerDeleted handles customer deletion
func (sc *StripeClient) HandleCustomerDeleted(ctx context.Context, event *StripeWebhookEvent, tenantID uuid.UUID) error {
	var customer stripe.Customer
	if err := json.Unmarshal(event.Data, &customer); err != nil {
		return fmt.Errorf("failed to unmarshal customer: %w", err)
	}

	// Cancel all subscriptions for this customer in your database

	return nil
}

// HandleWebhook handles incoming webhooks from Stripe
func (sc *StripeClient) HandleWebhook(ctx context.Context, r io.Reader, signature, timestamp string) error {
	// Verify webhook signature
	body, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("failed to read webhook body: %w", err)
	}

	if !sc.VerifyWebhookSignature(body, signature, timestamp) {
		return ErrStripeWebhookInvalid
	}

	// Parse webhook event
	event, err := sc.ParseWebhookEvent(r)
	if err != nil {
		return fmt.Errorf("failed to parse webhook event: %w", err)
	}

	// Extract tenant ID from event metadata or customer
	tenantID, err := sc.getTenantIDFromEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to get tenant ID from event: %w", err)
	}

	// Handle event based on type
	switch event.Type {
	case "payment_intent.succeeded":
		if err := sc.HandlePaymentIntentSucceeded(ctx, event, tenantID); err != nil {
			return fmt.Errorf("failed to handle payment_intent.succeeded: %w", err)
		}
	case "payment_intent.payment_failed":
		// Handle failed payment
	case "invoice.payment_succeeded":
		if err := sc.HandleInvoicePaymentSucceeded(ctx, event, tenantID); err != nil {
			return fmt.Errorf("failed to handle invoice.payment_succeeded: %w", err)
		}
	case "invoice.payment_failed":
		// Handle failed invoice payment
	case "customer.subscription.created":
		// Handle new subscription
	case "customer.subscription.updated":
		if err := sc.HandleSubscriptionUpdated(ctx, event, tenantID); err != nil {
			return fmt.Errorf("failed to handle customer.subscription.updated: %w", err)
		}
	case "customer.subscription.deleted":
		if err := sc.HandleCustomerDeleted(ctx, event, tenantID); err != nil {
			return fmt.Errorf("failed to handle customer.subscription.deleted: %w", err)
		}
	case "customer.deleted":
		if err := sc.HandleCustomerDeleted(ctx, event, tenantID); err != nil {
			return fmt.Errorf("failed to handle customer.deleted: %w", err)
		}
	default:
		// Log unsupported event type
	}

	return nil
}

// getTenantIDFromEvent extracts tenant ID from webhook event
func (sc *StripeClient) getTenantIDFromEvent(ctx context.Context, event *StripeWebhookEvent) (uuid.UUID, err) {
	// Extract data from event based on event type
	var data map[string]interface{}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return uuid.Nil, fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	// Try to get tenant ID from metadata
	tenantIDStr := ""

	// Check different event structures
	if obj, ok := data["object"].(map[string]interface{}); ok {
		if customer, ok := obj["customer"].(map[string]interface{}); ok {
			if metadata, ok := customer["metadata"].(map[string]interface{}); ok {
				if tenantID, ok := metadata["tenant_id"].(string); ok {
					tenantIDStr = tenantID
				}
			}
		}
		}
	}

	if tenantIDStr == "" {
		if obj, ok := data["previous_attributes"].(map[string]interface{}); ok {
			if customer, ok := obj["customer"].(map[string]interface{}); ok {
				if metadata, ok := customer["metadata"].(map[string]interface{}); ok {
					if tenantID, ok := metadata["tenant_id"].(string); ok {
						tenantIDStr = tenantID
					}
				}
			}
		}
		}
	}

	if tenantIDStr == "" {
		return uuid.Nil, errors.New("tenant_id not found in event metadata")
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to parse tenant ID: %w", err)
	}

	return tenantID, nil
}

// buildAddressParams builds Stripe address parameters
func (sc *StripeClient) buildAddressParams(address *Address) *stripe.AddressParams {
	if address == nil {
		return nil
	}

	return &stripe.AddressParams{
		Line1:      stripe.String(address.Line1),
		Line2:      stripe.String(address.Line2),
		City:       stripe.String(address.City),
		State:      stripe.String(address.State),
		PostalCode: stripe.String(address.PostalCode),
		Country:    stripe.String(address.Country),
	}
}

// buildMetadata builds Stripe metadata parameters
func (sc *StripeClient) buildMetadata(metadata map[string]string) *map[string]string {
	// Add tenant ID to metadata if present
	if metadata == nil {
		metadata = make(map[string]string)
	}

	return &metadata
}

// buildTimestamp builds a Stripe timestamp parameter from days
func (sc *StripeClient) buildTimestamp(days int64) *stripe.Int64 {
	if days == 0 {
		return nil
	}

	timestamp := time.Now().AddDate(0, 0, int(days)).Unix()
	return stripe.Int64(timestamp)
}

// buildTrialPeriod builds a Stripe trial period parameter
func (sc *StripeClient) buildTrialPeriod(days int64) *stripe.SubscriptionTrialPeriodParams {
	if days == 0 {
		return nil
	}

	params := &stripe.SubscriptionTrialPeriodParams{
		Period: stripe.String("day"),
	}

	switch {
	case days > 30:
		params.TrialPeriod = stripe.String("custom")
		params.TrialPeriodDays = stripe.Int64(days)
	default:
		params.TrialPeriod = stripe.String("custom")
		params.TrialPeriodDays = stripe.Int64(days)
	}

	return params
}

// HandleWebhookHTTP is an HTTP handler for webhooks
func (sc *StripeClient) HandleWebhookHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract signature and timestamp from headers
	signature := r.Header.Get("Stripe-Signature")
	timestamp := r.Header.Get("Stripe-Signature-Timestamp")

	// Handle webhook
	if err := sc.HandleWebhook(r.Context(), r.Body, signature, timestamp); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Return 200 OK
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "received"}`))
}

// ListPaymentMethods lists payment methods for a customer
func (sc *StripeClient) ListPaymentMethods(ctx context.Context, customerID string, limit int64) ([]*stripe.PaymentMethod, error) {
	params := &stripe.PaymentMethodListParams{
		Customer: stripe.String(customerID),
		Type:     stripe.String("card"),
		Limit:     stripe.Int64(limit),
	}

	paymentMethods := stripe.PaymentMethodList{}
	iter := sc.client.PaymentMethods.List(params)
	for iter.Next() {
		paymentMethods = append(paymentMethods, iter.PaymentMethod())
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to list payment methods: %w", err)
	}

	return paymentMethods, nil
}

// CreatePaymentMethod creates a payment method
func (sc *StripeClient) CreatePaymentMethod(ctx context.Context, customerID string, token string) (*stripe.PaymentMethod, error) {
	params := &stripe.PaymentMethodParams{
		Customer: stripe.String(customerID),
		Card: &stripe.PaymentMethodCardParams{
			Token: stripe.String(token),
		},
	}

	paymentMethod, err := sc.client.PaymentMethods.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment method: %w", err)
	}

	return paymentMethod, nil
}

// AttachPaymentMethod attaches a payment method to a customer
func (sc *StripeClient) AttachPaymentMethod(ctx context.Context, paymentMethodID, customerID string) (*stripe.PaymentMethod, error) {
	params := &stripe.PaymentMethodAttachParams{
		Customer: stripe.String(customerID),
	}

	paymentMethod, err := sc.client.PaymentMethods.Attach(paymentMethodID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to attach payment method: %w", err)
	}

	return paymentMethod, nil
}

// DetachPaymentMethod detaches a payment method
func (sc *StripeClient) DetachPaymentMethod(ctx context.Context, paymentMethodID string) error {
	params := &stripe.PaymentMethodDetachParams{}
	_, err := sc.client.PaymentMethods.Detach(paymentMethodID, params)
	if err != nil {
		return fmt.Errorf("failed to detach payment method: %w", err)
	}

	return nil
}

// CreateToken creates a Stripe token for payment
func (sc *StripeClient) CreateToken(ctx context.Context, card stripe.PaymentMethodCardParams) (*stripe.Token, error) {
	params := &stripe.TokenParams{
		Card: card,
	}

	token, err := sc.client.Tokens.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create Stripe token: %w", err)
	}

	return token, nil
}

// GetAccount retrieves Stripe account information
func (sc *StripeClient) GetAccount(ctx context.Context) (*stripe.Account, error) {
	account, err := sc.client.Account.Get(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Stripe account: %w", err)
	}

	return account, nil
}

// CreateProduct creates a product
func (sc *StripeClient) CreateProduct(ctx context.Context, name, description string, metadata map[string]string) (*stripe.Product, error) {
	params := &stripe.ProductParams{
		Name:        stripe.String(name),
		Description: stripe.String(description),
		Metadata:    sc.buildMetadata(metadata),
	}

	product, err := sc.client.Products.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create Stripe product: %w", err)
	}

	return product, nil
}

// GetProduct retrieves a product
func (sc *StripeClient) GetProduct(ctx context.Context, productID string) (*stripe.Product, error) {
	product, err := sc.client.Products.Get(productID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Stripe product: %w", err)
	}

	return product, nil
}

// UpdateProduct updates a product
func (sc *StripeClient) UpdateProduct(ctx context.Context, productID string, params *stripe.ProductParams) (*stripe.Product, error) {
	product, err := sc.client.Products.Update(productID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to update Stripe product: %w", err)
	}

	return product, nil
}

// ListProducts lists products
func (sc *StripeClient) ListProducts(ctx context.Context, limit int64) ([]*stripe.Product, error) {
	params := &stripe.ProductListParams{
		Limit: stripe.Int64(limit),
	}

	products := stripe.ProductList{}
	iter := sc.client.Products.List(params)
	for iter.Next() {
		products = append(products, iter.Product())
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to list products: %w", err)
	}

	return products, nil
}

// DeleteProduct deletes a product
func (sc *StripeClient) DeleteProduct(ctx context.Context, productID string) error {
	_, err := sc.client.Products.Del(productID, nil)
	if err != nil {
		return fmt.Errorf("failed to delete Stripe product: %w", err)
	}

	return nil
}

// GetBalance retrieves Stripe balance
func (sc *StripeClient) GetBalance(ctx context.Context) (*stripe.Balance, error) {
	balance, err := sc.client.Balance.Get(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Stripe balance: %w", err)
	}

	return balance, nil
}

// CreateRefund creates a refund
func (sc *StripeClient) CreateRefund(ctx context.Context, chargeID, paymentIntentID string, amount int64, reason string) (*stripe.Refund, error) {
	var params *stripe.RefundParams

	if chargeID != "" {
		params = &stripe.RefundParams{
			Charge:  stripe.String(chargeID),
			Amount:  stripe.Int64(amount),
			Reason:  stripe.String(reason),
		}
	} else {
		params = &stripe.RefundParams{
			PaymentIntent: stripe.String(paymentIntentID),
			Amount: stripe.Int64(amount),
			Reason:  stripe.String(reason),
		}
	}

	refund, err := sc.client.Refunds.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create refund: %w", err)
	}

	return refund, nil
}

// GetRefund retrieves a refund
func (sc *StripeClient) GetRefund(ctx context.Context, refundID string) (*stripe.Refund, error) {
	refund, err := sc.client.Refunds.Get(refundID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get refund: %w", err)
	}

	return refund, nil
}

// CreateTransfer creates a transfer (for marketplace apps)
func (sc *StripeClient) CreateTransfer(ctx context.Context, amount int64, currency, destination, description string, metadata map[string]string) (*stripe.Transfer, error) {
	params := &stripe.TransferParams{
		Amount:      stripe.Int64(amount),
		Currency:    stripe.String(currency),
		Destination: stripe.String(destination),
		Description: stripe.String(description),
		Metadata:    sc.buildMetadata(metadata),
	}

	transfer, err := sc.client.Transfers.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create transfer: %w", err)
	}

	return transfer, nil
}

// GetTransfer retrieves a transfer
func (sc *StripeClient) GetTransfer(ctx context.Context, transferID string) (*stripe.Transfer, error) {
	transfer, err := sc.client.Transfers.Get(transferID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get transfer: %w", err)
	}

	return transfer, nil
}

// ListTransfers lists transfers
func (sc *StripeClient) ListTransfers(ctx context.Context, limit int64) ([]*stripe.Transfer, error) {
	params := &stripe.TransferListParams{
		Limit: stripe.Int64(limit),
	}

	transfers := stripe.TransferList{}
	iter := sc.client.Transfers.List(params)
	for iter.Next() {
		transfers = append(transfers, iter.Transfer())
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to list transfers: %w", err)
	}

	return transfers, nil
}

// CreateConnectAccount creates a Connect account
func (sc *StripeClient) CreateConnectAccount(ctx context.Context, country, type, businessProfile map[string]string, tosAcceptance map[string]string) (*stripe.ConnectAccount, error) {
	params := &stripe.ConnectAccountParams{
		Country:            stripe.String(country),
		Type:                stripe.String(type),
		BusinessProfile:      sc.buildBusinessProfile(businessProfile),
		TosAcceptance:       sc.buildTOSAcceptance(tosAcceptance),
	}

	account, err := sc.client.ConnectAccounts.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create Connect account: %w", err)
	}

	return account, nil
}

// buildBusinessProfile builds business profile parameters
func (sc *StripeClient) buildBusinessProfile(profile map[string]string) *stripe.ConnectAccountBusinessProfileParams {
	if profile == nil {
		return nil
	}

	return &stripe.ConnectAccountBusinessProfileParams{
		URL:      stripe.String(profile["url"]),
		Name:     stripe.String(profile["name"]),
		SupportEmail: stripe.String(profile["support_email"]),
	}
}

// buildTOSAcceptance builds TOS acceptance parameters
func (sc *StripeClient) buildTOSAcceptance(tosAcceptance map[string]string) *stripe.ConnectAccountTOSAcceptanceParams {
	if tosAcceptance == nil {
		return nil
	}

	return &stripe.ConnectAccountTOSAcceptanceParams{
		ServiceAgreement: stripe.String(tosAcceptance["service_agreement"]),
		IPAssignee:      stripe.Bool(true),
	}
}

// GetConnectAccount retrieves a Connect account
func (sc *StripeClient) GetConnectAccount(ctx context.Context, accountID string) (*stripe.ConnectAccount, error) {
	account, err := sc.client.ConnectAccounts.Get(accountID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Connect account: %w", err)
	}

	return account, nil
}

// GetBalanceTransaction retrieves a balance transaction
func (sc *StripeClient) GetBalanceTransaction(ctx context.Context, txnID string) (*stripe.BalanceTransaction, error) {
	txn, err := sc.client.BalanceTransactions.Get(txnID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance transaction: %w", err)
	}

	return txn, nil
}

// ListBalanceTransactions lists balance transactions
func (sc *StripeClient) ListBalanceTransactions(ctx context.Context, customerID string, limit int64) ([]*stripe.BalanceTransaction, error) {
	params := &stripe.BalanceTransactionListParams{
		Customer: stripe.String(customerID),
		Limit:    stripe.Int64(limit),
	}

	txnList := stripe.BalanceTransactionList{}
	iter := sc.client.BalanceTransactions.List(params)
	for iter.Next() {
		txnList = append(txnList, iter.BalanceTransaction())
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to list balance transactions: %w", err)
	}

	return txnList, nil
}

// CreateCustomerSession creates a customer session
func (sc *StripeClient) CreateCustomerSession(ctx context.Context, customerID string) (*stripe.CustomerSession, error) {
	params := &stripe.CustomerSessionParams{
		Customer: stripe.String(customerID),
	}

	session, err := sc.client.CustomerSessions.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create customer session: %w", err)
	}

	return session, nil
}

// GetCustomerSession retrieves a customer session
func (sc *StripeClient) GetCustomerSession(ctx context.Context, sessionID string) (*stripe.CustomerSession, error) {
	session, err := sc.client.CustomerSessions.Get(sessionID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get customer session: %w", err)
	}

	return session, nil
}

// ExpireCustomerSession expires a customer session
func (sc *StripeClient) ExpireCustomerSession(ctx context.Context, sessionID string) error {
	_, err := sc.client.CustomerSessions.Expire(sessionID, nil)
	if err != nil {
		return fmt.Errorf("failed to expire customer session: %w", err)
	}

	return nil
}

// CreateApplicationFee creates an application fee
func (sc *StripeClient) CreateApplicationFee(ctx context.Context, amount int64, currency, applicationFeeAmount int64, description string) (*stripe.ApplicationFee, error) {
	params := &stripe.ApplicationFeeParams{
		Amount:          stripe.Int64(amount),
		Currency:        stripe.String(currency),
		ApplicationFee: stripe.Int64(applicationFeeAmount),
		Description:     stripe.String(description),
	}

	fee, err := sc.client.ApplicationFees.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create application fee: %w", err)
	}

	return fee, nil
}

// GetApplicationFee retrieves an application fee
func (sc *StripeClient) GetApplicationFee(ctx context.Context, feeID string) (*stripe.ApplicationFee, error) {
	fee, err := sc.client.ApplicationFees.Get(feeID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get application fee: %w", err)
	}

	return fee, nil
}
