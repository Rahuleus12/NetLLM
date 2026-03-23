// internal/billing/invoices.go
// Invoice generation and management
// Handles invoice creation, line items, calculations, and delivery

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
	ErrInvoiceNotFound       = errors.New("invoice not found")
	ErrInvoiceAlreadyExists  = errors.New("invoice already exists")
	ErrInvalidInvoice       = errors.New("invalid invoice data")
	ErrInvoiceCannotGenerate = errors.New("cannot generate invoice for this period")
	ErrInvoiceCannotFinalize = errors.New("cannot finalize invoice with pending items")
)

// InvoiceStatus represents the status of an invoice
type InvoiceStatus string

const (
	InvoiceStatusDraft      InvoiceStatus = "draft"
	InvoiceStatusPending    InvoiceStatus = "pending"
	InvoiceStatusPaid       InvoiceStatus = "paid"
	InvoiceStatusOverdue    InvoiceStatus = "overdue"
	InvoiceStatusVoid       InvoiceStatus = "void"
	InvoiceStatusRefunded   InvoiceStatus = "refunded"
	InvoiceStatusPartial    InvoiceStatus = "partially_paid"
)

// Invoice represents a billing invoice
type Invoice struct {
	ID              uuid.UUID       `json:"id" db:"id"`
	TenantID        uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	SubscriptionID  *uuid.UUID      `json:"subscription_id,omitempty" db:"subscription_id"`

	// Invoice details
	InvoiceNumber  string          `json:"invoice_number" db:"invoice_number"`
	Status        InvoiceStatus   `json:"status" db:"status"`
	Currency      string          `json:"currency" db:"currency"`

	// Dates
	IssueDate      time.Time       `json:"issue_date" db:"issue_date"`
	DueDate        time.Time       `json:"due_date" db:"due_date"`
	PaidAt         *time.Time      `json:"paid_at,omitempty" db:"paid_at"`

	// Amounts
	Subtotal       float64         `json:"subtotal" db:"subtotal"`
	Tax            float64         `json:"tax" db:"tax"`
	Discount       float64         `json:"discount" db:"discount"`
	Total          float64         `json:"total" db:"total"`
	AmountPaid     float64         `json:"amount_paid" db:"amount_paid"`
	AmountDue      float64         `json:"amount_due" db:"amount_due"`

	// Line items
	Items          []InvoiceItem  `json:"items" db:"-"`

	// Customer information
	BillingAddress *Address         `json:"billing_address,omitempty" db:"billing_address"`
	ShippingAddress *Address         `json:"shipping_address,omitempty" db:"shipping_address"`

	// Payment information
	PaymentMethodID *uuid.UUID      `json:"payment_method_id,omitempty" db:"payment_method_id"`
	PaymentReference string         `json:"payment_reference,omitempty" db:"payment_reference"`

	// Invoice settings
	AutoApplyCredits bool         `json:"auto_apply_credits" db:"auto_apply_credits"`
	CreditsApplied   float64    `json:"credits_applied" db:"credits_applied"`

	// Metadata
	Notes          string         `json:"notes,omitempty" db:"notes"`
	Metadata       map[string]interface{} `json:"metadata,omitempty" db:"metadata"`

	// File references
	PDFURL         string         `json:"pdf_url,omitempty" db:"pdf_url"`

	// Timestamps
	CreatedAt      time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at" db:"updated_at"`
	DeletedAt      *time.Time     `json:"deleted_at,omitempty" db:"deleted_at"`
}

// InvoiceItem represents a line item in an invoice
type InvoiceItem struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	InvoiceID        uuid.UUID  `json:"invoice_id" db:"invoice_id"`

	// Item details
	Description      string     `json:"description" db:"description"`
	Quantity         float64    `json:"quantity" db:"quantity"`
	UnitPrice        float64    `json:"unit_price" db:"unit_price"`

	// Totals
	Subtotal         float64    `json:"subtotal" db:"subtotal"`
	Tax              float64    `json:"tax" db:"tax"`
	Total            float64    `json:"total" db:"total"`

	// Item classification
	Type             string     `json:"type" db:"type"` // usage, recurring, one_time, credit, discount
	ResourceType     string     `json:"resource_type,omitempty" db:"resource_type"`

	// Usage period for usage-based items
	UsageStartDate   *time.Time `json:"usage_start_date,omitempty" db:"usage_start_date"`
	UsageEndDate     *time.Time `json:"usage_end_date,omitempty" db:"usage_end_date"`

	// References
	ResourceID       *string    `json:"resource_id,omitempty" db:"resource_id"`
	SubscriptionID   *uuid.UUID `json:"subscription_id,omitempty" db:"subscription_id"`

	// Timestamps
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
}

// Address represents a billing address
type Address struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

// CreateInvoiceRequest represents a request to create a new invoice
type CreateInvoiceRequest struct {
	TenantID        uuid.UUID              `json:"tenant_id"`
	SubscriptionID *uuid.UUID             `json:"subscription_id,omitempty"`

	// Invoice dates
	IssueDate       time.Time              `json:"issue_date"`
	DueDate         time.Time              `json:"due_date"`

	// Items
	Items           []InvoiceItemRequest   `json:"items"`

	// Customer information
	BillingAddress  *Address               `json:"billing_address,omitempty"`

	// Invoice settings
	Currency        string                 `json:"currency"`
	Notes           string                 `json:"notes,omitempty"`
	AutoApplyCredits bool                  `json:"auto_apply_credits"`

	// Metadata
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// InvoiceItemRequest represents a request to create an invoice item
type InvoiceItemRequest struct {
	Description      string     `json:"description"`
	Quantity         float64    `json:"quantity"`
	UnitPrice        float64    `json:"unit_price"`
	Type             string     `json:"type"`
	ResourceType     string     `json:"resource_type,omitempty"`
	UsageStartDate   *time.Time `json:"usage_start_date,omitempty"`
	UsageEndDate     *time.Time `json:"usage_end_date,omitempty"`
	ResourceID       *string    `json:"resource_id,omitempty"`
}

// UpdateInvoiceRequest represents a request to update an invoice
type UpdateInvoiceRequest struct {
	Status          *InvoiceStatus      `json:"status,omitempty"`
	Notes           *string             `json:"notes,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	PaymentMethodID *uuid.UUID          `json:"payment_method_id,omitempty"`
	PaymentReference string             `json:"payment_reference,omitempty"`
}

// ListInvoicesOptions represents options for listing invoices
type ListInvoicesOptions struct {
	TenantID        *uuid.UUID
	SubscriptionID   *uuid.UUID
	Status          *InvoiceStatus
	StartDate       *time.Time
	EndDate         *time.Time
	Limit           int
	Offset          int
}

// GenerateInvoiceRequest represents a request to generate an invoice for a billing period
type GenerateInvoiceRequest struct {
	TenantID        uuid.UUID       `json:"tenant_id"`
	SubscriptionID  uuid.UUID       `json:"subscription_id"`
	StartDate       time.Time       `json:"start_date"`
	EndDate         time.Time       `json:"end_date"`
}

// InvoiceManager manages invoices
type InvoiceManager struct {
	db *sql.DB
}

// NewInvoiceManager creates a new invoice manager
func NewInvoiceManager(db *sql.DB) *InvoiceManager {
	return &InvoiceManager{
		db: db,
	}
}

// GenerateInvoice generates an invoice based on usage for a billing period
func (im *InvoiceManager) GenerateInvoice(ctx context.Context, req GenerateInvoiceRequest) (*Invoice, error) {
	if req.TenantID == uuid.Nil {
		return nil, ErrInvalidInvoice
	}
	if req.SubscriptionID == uuid.Nil {
		return nil, ErrInvalidInvoice
	}
	if req.StartTime.After(req.EndTime) {
		return nil, errors.New("start_date must be before end_date")
	}

	// Get usage for the billing period
	usage, err := im.getUsageForPeriod(ctx, req.TenantID, req.StartDate, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage for period: %w", err)
	}

	// Get subscription for pricing
	subscription, err := im.getSubscription(ctx, req.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	// Build invoice items based on usage and pricing
	items, err := im.buildInvoiceItems(ctx, usage, subscription, req.StartDate, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to build invoice items: %w", err)
	}

	if len(items) == 0 {
		return nil, ErrInvoiceCannotGenerate
	}

	// Calculate totals
	subtotal, tax, total := im.calculateTotals(items)

	// Check for available credits
	creditsApplied := 0.0
	if subscription.AutoApplyCredits {
		credits, err := im.getAvailableCredits(ctx, req.TenantID)
		if err == nil {
			if credits > total {
				creditsApplied = total
			} else {
				creditsApplied = credits
			}
		}
	}

	// Generate invoice number
	invoiceNumber, err := im.generateInvoiceNumber(ctx, req.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate invoice number: %w", err)
	}

	invoice := &Invoice{
		ID:              uuid.New(),
		TenantID:        req.TenantID,
		SubscriptionID:  &req.SubscriptionID,
		InvoiceNumber:   invoiceNumber,
		Status:          InvoiceStatusPending,
		Currency:        subscription.Currency,
		IssueDate:       time.Now(),
		DueDate:         time.Now().AddDate(0, 0, 7), // Due in 7 days
		Subtotal:        subtotal,
		Tax:             tax,
		Discount:        0,
		CreditsApplied:   creditsApplied,
		Total:           total - creditsApplied,
		AmountPaid:      0,
		AmountDue:       total - creditsApplied,
		Items:           items,
		AutoApplyCredits: true,
		Notes:           fmt.Sprintf("Billing period: %s to %s", req.StartDate.Format("2006-01-02"), req.EndTime.Format("2006-01-02")),
		Metadata: map[string]interface{}{
			"billing_period_start": req.StartDate,
			"billing_period_end":   req.EndTime,
		},
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Save invoice
	err = im.saveInvoice(ctx, invoice)
	if err != nil {
		return nil, fmt.Errorf("failed to save invoice: %w", err)
	}

	return invoice, nil
}

// CreateInvoice creates a new invoice
func (im *InvoiceManager) CreateInvoice(ctx context.Context, req CreateInvoiceRequest) (*Invoice, error) {
	if req.TenantID == uuid.Nil {
		return nil, ErrInvalidInvoice
	}
	if len(req.Items) == 0 {
		return nil, ErrInvalidInvoice
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}

	// Build invoice items
	items := make([]InvoiceItem, 0, len(req.Items))
	for _, itemReq := range req.Items {
		subtotal := itemReq.Quantity * itemReq.UnitPrice
		tax := subtotal * 0.10 // 10% tax
		total := subtotal + tax

		item := InvoiceItem{
			ID:          uuid.New(),
			Description: itemReq.Description,
			Quantity:    itemReq.Quantity,
			UnitPrice:   itemReq.UnitPrice,
			Subtotal:    subtotal,
			Tax:         tax,
			Total:       total,
			Type:        itemReq.Type,
			ResourceType: itemReq.ResourceType,
			UsageStartDate: itemReq.UsageStartDate,
			UsageEndDate:   itemReq.UsageEndDate,
			ResourceID:    itemReq.ResourceID,
			SubscriptionID: req.SubscriptionID,
			CreatedAt:    time.Now(),
		}
		items = append(items, item)
	}

	// Calculate totals
	subtotal, tax, total := im.calculateTotals(items)

	// Generate invoice number
	invoiceNumber, err := im.generateInvoiceNumber(ctx, req.TenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate invoice number: %w", err)
	}

	invoice := &Invoice{
		ID:              uuid.New(),
		TenantID:        req.TenantID,
		SubscriptionID:  req.SubscriptionID,
		InvoiceNumber:   invoiceNumber,
		Status:          InvoiceStatusDraft,
		Currency:        req.Currency,
		IssueDate:       req.IssueDate,
		DueDate:         req.DueDate,
		Subtotal:        subtotal,
		Tax:             tax,
		Discount:        0,
		Total:           total,
		AmountPaid:      0,
		AmountDue:       total,
		Items:           items,
		BillingAddress:   req.BillingAddress,
		AutoApplyCredits: req.AutoApplyCredits,
		Notes:           req.Notes,
		Metadata:        req.Metadata,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Apply credits if enabled
	if req.AutoApplyCredits {
		credits, err := im.getAvailableCredits(ctx, req.TenantID)
		if err == nil {
			if credits > total {
				invoice.CreditsApplied = total
				invoice.AmountDue = 0
			} else {
				invoice.CreditsApplied = credits
				invoice.AmountDue = total - credits
			}
		}
	}

	// Save invoice
	err = im.saveInvoice(ctx, invoice)
	if err != nil {
		return nil, fmt.Errorf("failed to save invoice: %w", err)
	}

	return invoice, nil
}

// GetInvoice retrieves an invoice by ID
func (im *InvoiceManager) GetInvoice(ctx context.Context, id uuid.UUID) (*Invoice, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidInvoice
	}

	var invoice Invoice
	var billingAddressJSON, shippingAddressJSON, metadataJSON []byte

	query := `
		SELECT id, tenant_id, subscription_id, invoice_number, status, currency,
			issue_date, due_date, paid_at, subtotal, tax, discount, total,
			amount_paid, amount_due, auto_apply_credits, credits_applied,
			billing_address, shipping_address, payment_method_id, payment_reference,
			notes, metadata, pdf_url, created_at, updated_at, deleted_at
		FROM invoices
		WHERE id = $1 AND deleted_at IS NULL
	`

	err := im.db.QueryRowContext(ctx, query, id).Scan(
		&invoice.ID,
		&invoice.TenantID,
		&invoice.SubscriptionID,
		&invoice.InvoiceNumber,
		&invoice.Status,
		&invoice.Currency,
		&invoice.IssueDate,
		&invoice.DueDate,
		&invoice.PaidAt,
		&invoice.Subtotal,
		&invoice.Tax,
		&invoice.Discount,
		&invoice.Total,
		&invoice.AmountPaid,
		&invoice.AmountDue,
		&invoice.AutoApplyCredits,
		&invoice.CreditsApplied,
		&billingAddressJSON,
		&shippingAddressJSON,
		&invoice.PaymentMethodID,
		&invoice.PaymentReference,
		&invoice.Notes,
		&metadataJSON,
		&invoice.PDFURL,
		&invoice.CreatedAt,
		&invoice.UpdatedAt,
		&invoice.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("failed to get invoice: %w", err)
	}

	// Unmarshal JSON fields
	if billingAddressJSON != nil {
		json.Unmarshal(billingAddressJSON, &invoice.BillingAddress)
	}
	if shippingAddressJSON != nil {
		json.Unmarshal(shippingAddressJSON, &invoice.ShippingAddress)
	}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &invoice.Metadata)
	}

	// Load items
	items, err := im.getInvoiceItems(ctx, invoice.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get invoice items: %w", err)
	}
	invoice.Items = items

	return &invoice, nil
}

// GetInvoiceByNumber retrieves an invoice by invoice number
func (im *InvoiceManager) GetInvoiceByNumber(ctx context.Context, tenantID uuid.UUID, invoiceNumber string) (*Invoice, error) {
	if tenantID == uuid.Nil {
		return nil, ErrInvalidInvoice
	}
	if invoiceNumber == "" {
		return nil, ErrInvalidInvoice
	}

	var invoice Invoice
	var billingAddressJSON, shippingAddressJSON, metadataJSON []byte

	query := `
		SELECT id, tenant_id, subscription_id, invoice_number, status, currency,
			issue_date, due_date, paid_at, subtotal, tax, discount, total,
			amount_paid, amount_due, auto_apply_credits, credits_applied,
			billing_address, shipping_address, payment_method_id, payment_reference,
			notes, metadata, pdf_url, created_at, updated_at, deleted_at
		FROM invoices
		WHERE tenant_id = $1 AND invoice_number = $2 AND deleted_at IS NULL
	`

	err := im.db.QueryRowContext(ctx, query, tenantID, invoiceNumber).Scan(
		&invoice.ID,
		&invoice.TenantID,
		&invoice.SubscriptionID,
		&invoice.InvoiceNumber,
		&invoice.Status,
		&invoice.Currency,
		&invoice.IssueDate,
		&invoice.DueDate,
		&invoice.PaidAt,
		&invoice.Subtotal,
		&invoice.Tax,
		&invoice.Discount,
		&invoice.Total,
		&invoice.AmountPaid,
		&invoice.AmountDue,
		&invoice.AutoApplyCredits,
		&invoice.CreditsApplied,
		&billingAddressJSON,
		&shippingAddressJSON,
		&invoice.PaymentMethodID,
		&invoice.PaymentReference,
		&invoice.Notes,
		&metadataJSON,
		&invoice.PDFURL,
		&invoice.CreatedAt,
		&invoice.UpdatedAt,
		&invoice.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("failed to get invoice by number: %w", err)
	}

	// Unmarshal JSON fields
	if billingAddressJSON != nil {
		json.Unmarshal(billingAddressJSON, &invoice.BillingAddress)
	}
	if shippingAddressJSON != nil {
		json.Unmarshal(shippingAddressJSON, &invoice.ShippingAddress)
	}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &invoice.Metadata)
	}

	// Load items
	items, err := im.getInvoiceItems(ctx, invoice.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get invoice items: %w", err)
	}
	invoice.Items = items

	return &invoice, nil
}

// UpdateInvoice updates an invoice
func (im *InvoiceManager) UpdateInvoice(ctx context.Context, id uuid.UUID, req UpdateInvoiceRequest) (*Invoice, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidInvoice
	}

	invoice, err := im.GetInvoice(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if req.Status != nil {
		if !isValidInvoiceStatus(*req.Status) {
			return nil, errors.New("invalid invoice status")
		}
		invoice.Status = *req.Status

		// Update paid date if status is paid
		if *req.Status == InvoiceStatusPaid {
			now := time.Now()
			invoice.PaidAt = &now
			invoice.AmountPaid = invoice.AmountDue
			invoice.AmountDue = 0
		}
	}
	if req.Notes != nil {
		invoice.Notes = *req.Notes
	}
	if req.PaymentMethodID != nil {
		invoice.PaymentMethodID = req.PaymentMethodID
	}
	if req.PaymentReference != "" {
		invoice.PaymentReference = req.PaymentReference
	}
	if req.Metadata != nil {
		invoice.Metadata = req.Metadata
	}
	invoice.UpdatedAt = time.Now()

	// Update database
	billingAddressJSON, _ := json.Marshal(invoice.BillingAddress)
	shippingAddressJSON, _ := json.Marshal(invoice.ShippingAddress)
	metadataJSON, _ := json.Marshal(invoice.Metadata)

	query := `
		UPDATE invoices
		SET status = $1, notes = $2, payment_method_id = $3, payment_reference = $4,
			metadata = $5, updated_at = $6, paid_at = $7, amount_paid = $8, amount_due = $9
		WHERE id = $10
		RETURNING id, tenant_id, subscription_id, invoice_number, status, currency,
			issue_date, due_date, paid_at, subtotal, tax, discount, total,
			amount_paid, amount_due, auto_apply_credits, credits_applied,
			billing_address, shipping_address, payment_method_id, payment_reference,
			notes, metadata, pdf_url, created_at, updated_at, deleted_at
	`

	err = im.db.QueryRowContext(ctx, query,
		invoice.Status,
		invoice.Notes,
		invoice.PaymentMethodID,
		invoice.PaymentReference,
		metadataJSON,
		invoice.UpdatedAt,
		invoice.PaidAt,
		invoice.AmountPaid,
		invoice.AmountDue,
		invoice.ID,
	).Scan(
		&invoice.ID,
		&invoice.TenantID,
		&invoice.SubscriptionID,
		&invoice.InvoiceNumber,
		&invoice.Status,
		&invoice.Currency,
		&invoice.IssueDate,
		&invoice.DueDate,
		&invoice.PaidAt,
		&invoice.Subtotal,
		&invoice.Tax,
		&invoice.Discount,
		&invoice.Total,
		&invoice.AmountPaid,
		&invoice.AmountDue,
		&invoice.AutoApplyCredits,
		&invoice.CreditsApplied,
		&billingAddressJSON,
		&shippingAddressJSON,
		&invoice.PaymentMethodID,
		&invoice.PaymentReference,
		&invoice.Notes,
		&metadataJSON,
		&invoice.PDFURL,
		&invoice.CreatedAt,
		&invoice.UpdatedAt,
		&invoice.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update invoice: %w", err)
	}

	// Unmarshal JSON fields
	json.Unmarshal(billingAddressJSON, &invoice.BillingAddress)
	json.Unmarshal(shippingAddressJSON, &invoice.ShippingAddress)
	json.Unmarshal(metadataJSON, &invoice.Metadata)

	return invoice, nil
}

// DeleteInvoice voids an invoice
func (im *InvoiceManager) DeleteInvoice(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidInvoice
	}

	invoice, err := im.GetInvoice(ctx, id)
	if err != nil {
		return err
	}

	// Can only void draft or pending invoices
	if invoice.Status != InvoiceStatusDraft && invoice.Status != InvoiceStatusPending {
		return errors.New("cannot void paid or overdue invoices")
	}

	query := `
		UPDATE invoices
		SET status = 'void', deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err = im.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to void invoice: %w", err)
	}

	return nil
}

// ListInvoices lists invoices with optional filters
func (im *InvoiceManager) ListInvoices(ctx context.Context, opts ListInvoicesOptions) ([]*Invoice, int64, error) {
	invoices := []*Invoice{}
	var total int64

	// Build query with dynamic filters
	baseQuery := `
		SELECT id, tenant_id, subscription_id, invoice_number, status, currency,
			issue_date, due_date, paid_at, subtotal, tax, discount, total,
			amount_paid, amount_due, auto_apply_credits, credits_applied,
			billing_address, shipping_address, payment_method_id, payment_reference,
			notes, metadata, pdf_url, created_at, updated_at, deleted_at
		FROM invoices
		WHERE deleted_at IS NULL
	`
	countQuery := `SELECT COUNT(*) FROM invoices WHERE deleted_at IS NULL`

	args := []interface{}{}
	argPos := 1

	if opts.TenantID != nil {
		baseQuery += fmt.Sprintf(" AND tenant_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND tenant_id = $%d", argPos)
		args = append(args, *opts.TenantID)
		argPos++
	}

	if opts.SubscriptionID != nil {
		baseQuery += fmt.Sprintf(" AND subscription_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND subscription_id = $%d", argPos)
		args = append(args, *opts.SubscriptionID)
		argPos++
	}

	if opts.Status != nil {
		baseQuery += fmt.Sprintf(" AND status = $%d", argPos)
		countQuery += fmt.Sprintf(" AND status = $%d", argPos)
		args = append(args, *opts.Status)
		argPos++
	}

	if opts.StartDate != nil {
		baseQuery += fmt.Sprintf(" AND issue_date >= $%d", argPos)
		countQuery += fmt.Sprintf(" AND issue_date >= $%d", argPos)
		args = append(args, *opts.StartDate)
		argPos++
	}

	if opts.EndDate != nil {
		baseQuery += fmt.Sprintf(" AND issue_date <= $%d", argPos)
		countQuery += fmt.Sprintf(" AND issue_date <= $%d", argPos)
		args = append(args, *opts.EndDate)
		argPos++
	}

	// Get total count
	err := im.db.QueryRowContext(ctx, countQuery, args...[:argPos-1]...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count invoices: %w", err)
	}

	// Add pagination
	if opts.Limit > 0 {
		baseQuery += fmt.Sprintf(" LIMIT $%d", argPos)
		args = append(args, opts.Limit)
		argPos++
	}
	if opts.Offset > 0 {
		baseQuery += fmt.Sprintf(" OFFSET $%d", argPos)
		args = append(args, opts.Offset)
		argPos++
	}

	baseQuery += " ORDER BY issue_date DESC"

	rows, err := im.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list invoices: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var invoice Invoice
		var billingAddressJSON, shippingAddressJSON, metadataJSON []byte

		err := rows.Scan(
			&invoice.ID,
			&invoice.TenantID,
			&invoice.SubscriptionID,
			&invoice.InvoiceNumber,
			&invoice.Status,
			&invoice.Currency,
			&invoice.IssueDate,
			&invoice.DueDate,
			&invoice.PaidAt,
			&invoice.Subtotal,
			&invoice.Tax,
			&invoice.Discount,
			&invoice.Total,
			&invoice.AmountPaid,
			&invoice.AmountDue,
			&invoice.AutoApplyCredits,
			&invoice.CreditsApplied,
			&billingAddressJSON,
			&shippingAddressJSON,
			&invoice.PaymentMethodID,
			&invoice.PaymentReference,
			&invoice.Notes,
			&metadataJSON,
			&invoice.PDFURL,
			&invoice.CreatedAt,
			&invoice.UpdatedAt,
			&invoice.DeletedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan invoice: %w", err)
		}

		// Unmarshal JSON fields
		if billingAddressJSON != nil {
			json.Unmarshal(billingAddressJSON, &invoice.BillingAddress)
		}
		if shippingAddressJSON != nil {
			json.Unmarshal(shippingAddressJSON, &invoice.ShippingAddress)
		}
		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &invoice.Metadata)
		}

		// Load items
		items, err := im.getInvoiceItems(ctx, invoice.ID)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to get invoice items: %w", err)
		}
		invoice.Items = items

		invoices = append(invoices, &invoice)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating invoices: %w", err)
	}

	return invoices, total, nil
}

// FinalizeInvoice marks an invoice as ready to be paid
func (im *InvoiceManager) FinalizeInvoice(ctx context.Context, id uuid.UUID) (*Invoice, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidInvoice
	}

	invoice, err := im.GetInvoice(ctx, id)
	if err != nil {
		return nil, err
	}

	// Can only finalize draft invoices
	if invoice.Status != InvoiceStatusDraft {
		return nil, errors.New("can only finalize draft invoices")
	}

	return im.UpdateInvoice(ctx, id, UpdateInvoiceRequest{
		Status: func() *InvoiceStatus { s := InvoiceStatusPending; return &s }(),
	})
}

// GenerateInvoicePDF generates a PDF for an invoice
func (im *InvoiceManager) GenerateInvoicePDF(ctx context.Context, id uuid.UUID) (string, error) {
	invoice, err := im.GetInvoice(ctx, id)
	if err != nil {
		return "", err
	}

	// In a real implementation, this would generate a PDF file
	// For now, we'll just return a mock URL
	pdfURL := fmt.Sprintf("https://invoices.example.com/%s.pdf", invoice.ID.String())

	// Update invoice with PDF URL
	query := `UPDATE invoices SET pdf_url = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`
	_, err = im.db.ExecContext(ctx, query, pdfURL, id)
	if err != nil {
		return "", fmt.Errorf("failed to update invoice PDF URL: %w", err)
	}

	return pdfURL, nil
}

// SendInvoice sends an invoice to the customer
func (im *InvoiceManager) SendInvoice(ctx context.Context, id uuid.UUID) error {
	invoice, err := im.GetInvoice(ctx, id)
	if err != nil {
		return err
	}

	// Generate PDF if not exists
	if invoice.PDFURL == "" {
		_, err = im.GenerateInvoicePDF(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to generate PDF: %w", err)
		}
	}

	// In a real implementation, this would send an email with the invoice
	// For now, we'll just mark as sent
	return nil
}

// GetOverdueInvoices retrieves all overdue invoices
func (im *InvoiceManager) GetOverdueInvoices(ctx context.Context, tenantID uuid.UUID) ([]*Invoice, error) {
	if tenantID == uuid.Nil {
		return nil, ErrInvalidInvoice
	}

	opts := ListInvoicesOptions{
		TenantID: &tenantID,
		Status:   func() *InvoiceStatus { s := InvoiceStatusPending; return &s }(),
	}

	invoices, _, err := im.ListInvoices(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Filter for invoices past due date
	now := time.Now()
	overdueInvoices := make([]*Invoice, 0)
	for _, invoice := range invoices {
		if invoice.DueDate.Before(now) {
			overdueInvoices = append(overdueInvoices, invoice)
		}
	}

	return overdueInvoices, nil
}

// saveInvoice saves an invoice to the database
func (im *InvoiceManager) saveInvoice(ctx context.Context, invoice *Invoice) error {
	billingAddressJSON, _ := json.Marshal(invoice.BillingAddress)
	shippingAddressJSON, _ := json.Marshal(invoice.ShippingAddress)
	metadataJSON, _ := json.Marshal(invoice.Metadata)

	query := `
		INSERT INTO invoices (id, tenant_id, subscription_id, invoice_number, status, currency,
			issue_date, due_date, paid_at, subtotal, tax, discount, total,
			amount_paid, amount_due, auto_apply_credits, credits_applied,
			billing_address, shipping_address, payment_method_id, payment_reference,
			notes, metadata, pdf_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26)
		RETURNING id, tenant_id, subscription_id, invoice_number, status, currency,
			issue_date, due_date, paid_at, subtotal, tax, discount, total,
			amount_paid, amount_due, auto_apply_credits, credits_applied,
			billing_address, shipping_address, payment_method_id, payment_reference,
			notes, metadata, pdf_url, created_at, updated_at, deleted_at
	`

	err := im.db.QueryRowContext(ctx, query,
		invoice.ID,
		invoice.TenantID,
		invoice.SubscriptionID,
		invoice.InvoiceNumber,
		invoice.Status,
		invoice.Currency,
		invoice.IssueDate,
		invoice.DueDate,
		invoice.PaidAt,
		invoice.Subtotal,
		invoice.Tax,
		invoice.Discount,
		invoice.Total,
		invoice.AmountPaid,
		invoice.AmountDue,
		invoice.AutoApplyCredits,
		invoice.CreditsApplied,
		billingAddressJSON,
		shippingAddressJSON,
		invoice.PaymentMethodID,
		invoice.PaymentReference,
		invoice.Notes,
		metadataJSON,
		invoice.PDFURL,
		invoice.CreatedAt,
		invoice.UpdatedAt,
	).Scan(
		&invoice.ID,
		&invoice.TenantID,
		&invoice.SubscriptionID,
		&invoice.InvoiceNumber,
		&invoice.Status,
		&invoice.Currency,
		&invoice.IssueDate,
		&invoice.DueDate,
		&invoice.PaidAt,
		&invoice.Subtotal,
		&invoice.Tax,
		&invoice.Discount,
		&invoice.Total,
		&invoice.AmountPaid,
		&invoice.AmountDue,
		&invoice.AutoApplyCredits,
		&invoice.CreditsApplied,
		&billingAddressJSON,
		&shippingAddressJSON,
		&invoice.PaymentMethodID,
		&invoice.PaymentReference,
		&invoice.Notes,
		&metadataJSON,
		&invoice.PDFURL,
		&invoice.CreatedAt,
		&invoice.UpdatedAt,
		&invoice.DeletedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save invoice: %w", err)
	}

	// Save items
	for _, item := range invoice.Items {
		item.InvoiceID = invoice.ID
		err = im.saveInvoiceItem(ctx, &item)
		if err != nil {
			return fmt.Errorf("failed to save invoice item: %w", err)
		}
	}

	return nil
}

// saveInvoiceItem saves an invoice item to the database
func (im *InvoiceManager) saveInvoiceItem(ctx context.Context, item *InvoiceItem) error {
	query := `
		INSERT INTO invoice_items (id, invoice_id, description, quantity, unit_price,
			subtotal, tax, total, type, resource_type, usage_start_date,
			usage_end_date, resource_id, subscription_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`

	_, err := im.db.ExecContext(ctx, query,
		item.ID,
		item.InvoiceID,
		item.Description,
		item.Quantity,
		item.UnitPrice,
		item.Subtotal,
		item.Tax,
		item.Total,
		item.Type,
		item.ResourceType,
		item.UsageStartDate,
		item.UsageEndDate,
		item.ResourceID,
		item.SubscriptionID,
		item.CreatedAt,
	)

	return err
}

// getInvoiceItems retrieves all items for an invoice
func (im *InvoiceManager) getInvoiceItems(ctx context.Context, invoiceID uuid.UUID) ([]InvoiceItem, error) {
	query := `
		SELECT id, invoice_id, description, quantity, unit_price,
			subtotal, tax, total, type, resource_type, usage_start_date,
			usage_end_date, resource_id, subscription_id, created_at
		FROM invoice_items
		WHERE invoice_id = $1
		ORDER BY created_at ASC
	`

	rows, err := im.db.QueryContext(ctx, query, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []InvoiceItem{}
	for rows.Next() {
		var item InvoiceItem
		err := rows.Scan(
			&item.ID,
			&item.InvoiceID,
			&item.Description,
			&item.Quantity,
			&item.UnitPrice,
			&item.Subtotal,
			&item.Tax,
			&item.Total,
			&item.Type,
			&item.ResourceType,
			&item.UsageStartDate,
			&item.UsageEndDate,
			&item.ResourceID,
			&item.SubscriptionID,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, nil
}

// getUsageForPeriod retrieves usage data for a billing period
func (im *InvoiceManager) getUsageForPeriod(ctx context.Context, tenantID uuid.UUID, startDate, endDate time.Time) (map[string]float64, error) {
	usage := make(map[string]float64)

	query := `
		SELECT resource_type, SUM(quantity)
		FROM usage_records
		WHERE tenant_id = $1 AND recorded_at >= $2 AND recorded_at <= $3
		GROUP BY resource_type
	`

	rows, err := im.db.QueryContext(ctx, query, tenantID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var resourceType string
		var total float64
		err := rows.Scan(&resourceType, &total)
		if err != nil {
			continue
		}
		usage[resourceType] = total
	}

	return usage, nil
}

// getSubscription retrieves subscription details
func (im *InvoiceManager) getSubscription(ctx context.Context, subscriptionID uuid.UUID) (map[string]interface{}, error) {
	query := `
		SELECT plan_id, status, currency, trial_end_date
		FROM subscriptions
		WHERE id = $1
	`

	var planID uuid.UUID
	var status string
	var currency string
	var trialEndDate *time.Time

	err := im.db.QueryRowContext(ctx, query, subscriptionID).Scan(&planID, &status, &currency, &trialEndDate)
	if err != nil {
		return nil, err
	}

	// Get plan details
	var planPrice float64
	var planBillingInterval string
	planQuery := `SELECT price, billing_interval FROM plans WHERE id = $1`
	err = im.db.QueryRowContext(ctx, planQuery, planID).Scan(&planPrice, &planBillingInterval)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"plan_id":           planID,
		"status":             status,
		"currency":           currency,
		"price":              planPrice,
		"billing_interval":   planBillingInterval,
		"trial_end_date":    trialEndDate,
	}, nil
}

// buildInvoiceItems creates invoice items from usage and pricing
func (im *InvoiceManager) buildInvoiceItems(ctx context.Context, usage map[string]float64, subscription map[string]interface{}, startDate, endDate time.Time) ([]InvoiceItem, error) {
	items := []InvoiceItem{}
	price := subscription["price"].(float64)
	billingInterval := subscription["billing_interval"].(string)

	// Build items based on usage
	for resourceType, quantity := range usage {
		item := InvoiceItem{
			Description:    fmt.Sprintf("%s usage (%s)", resourceType, billingInterval),
			Quantity:      quantity,
			UnitPrice:     price,
			Type:          "usage",
			ResourceType:   resourceType,
			UsageStartDate: &startDate,
			UsageEndDate:   &endDate,
			CreatedAt:     time.Now(),
		}

		// Calculate totals
		item.Subtotal = item.Quantity * item.UnitPrice
		item.Tax = item.Subtotal * 0.10 // 10% tax
		item.Total = item.Subtotal + item.Tax

		items = append(items, item)
	}

	return items, nil
}

// calculateTotals calculates subtotal, tax, and total for invoice items
func (im *InvoiceManager) calculateTotals(items []InvoiceItem) (float64, float64, float64) {
	subtotal := 0.0
	for _, item := range items {
		subtotal += item.Subtotal
	}

	tax := 0.0
	for _, item := range items {
		tax += item.Tax
	}

	total := 0.0
	for _, item := range items {
		total += item.Total
	}

	return subtotal, tax, total
}

// getAvailableCredits retrieves available credits for a tenant
func (im *InvoiceManager) getAvailableCredits(ctx context.Context, tenantID uuid.UUID) (float64, error) {
	var credits float64

	query := `
		SELECT COALESCE(SUM(amount), 0)
		FROM credits
		WHERE tenant_id = $1 AND expires_at > CURRENT_TIMESTAMP AND used = false
	`

	err := im.db.QueryRowContext(ctx, query, tenantID).Scan(&credits)
	if err != nil {
		return 0, err
	}

	return credits, nil
}

// generateInvoiceNumber generates a unique invoice number
func (im *InvoiceManager) generateInvoiceNumber(ctx context.Context, tenantID uuid.UUID) (string, error) {
	var count int
	query := `SELECT COUNT(*) FROM invoices WHERE tenant_id = $1 AND YEAR(created_at) = YEAR(CURRENT_TIMESTAMP)`

	err := im.db.QueryRowContext(ctx, query, tenantID).Scan(&count)
	if err != nil {
		return "", err
	}

	// Format: INV-YYYYMM-XXXXX
	year := time.Now().Format("200601")
	invoiceNumber := fmt.Sprintf("INV-%s-%05d", year, count+1)

	return invoiceNumber, nil
}

// isValidInvoiceStatus checks if an invoice status is valid
func isValidInvoiceStatus(status InvoiceStatus) bool {
	switch status {
	case InvoiceStatusDraft, InvoiceStatusPending, InvoiceStatusPaid,
		InvoiceStatusOverdue, InvoiceStatusVoid, InvoiceStatusRefunded, InvoiceStatusPartial:
		return true
	default:
		return false
	}
}
