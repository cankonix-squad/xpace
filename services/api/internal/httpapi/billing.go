package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var billingProviderPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

type billingInvoice struct {
	ID                string     `json:"id"`
	Provider          string     `json:"provider"`
	ProviderInvoiceID string     `json:"providerInvoiceId"`
	InvoiceNumber     string     `json:"invoiceNumber"`
	Status            string     `json:"status"`
	Currency          string     `json:"currency"`
	SubtotalAmount    int64      `json:"subtotalAmount"`
	TaxAmount         int64      `json:"taxAmount"`
	TotalAmount       int64      `json:"totalAmount"`
	HostedInvoiceURL  *string    `json:"hostedInvoiceUrl,omitempty"`
	PeriodStartedAt   *time.Time `json:"periodStartedAt,omitempty"`
	PeriodEndsAt      *time.Time `json:"periodEndsAt,omitempty"`
	DueAt             *time.Time `json:"dueAt,omitempty"`
	PaidAt            *time.Time `json:"paidAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}

type billingWebhookPayload struct {
	ID              string                 `json:"id"`
	Type            string                 `json:"type"`
	TenantID        string                 `json:"tenantId"`
	PlanKey         string                 `json:"planKey"`
	CustomerID      string                 `json:"customerId"`
	SubscriptionID  string                 `json:"subscriptionId"`
	PeriodStartedAt *time.Time             `json:"periodStartedAt"`
	PeriodEndsAt    *time.Time             `json:"periodEndsAt"`
	Invoice         *billingInvoicePayload `json:"invoice"`
}

type billingInvoicePayload struct {
	ID              string     `json:"id"`
	Number          string     `json:"number"`
	Currency        string     `json:"currency"`
	SubtotalAmount  int64      `json:"subtotalAmount"`
	TaxAmount       int64      `json:"taxAmount"`
	TotalAmount     int64      `json:"totalAmount"`
	HostedURL       string     `json:"hostedUrl"`
	PeriodStartedAt *time.Time `json:"periodStartedAt"`
	PeriodEndsAt    *time.Time `json:"periodEndsAt"`
	DueAt           *time.Time `json:"dueAt"`
	PaidAt          *time.Time `json:"paidAt"`
}

func (api *API) adminBillingInvoices(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "tenant.manage") {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT id,provider,provider_invoice_id,invoice_number,status,currency,subtotal_amount,tax_amount,total_amount,hosted_invoice_url,period_started_at,period_ends_at,due_at,paid_at,created_at FROM billing_invoices WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 100`, actor.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load invoices")
		return
	}
	defer rows.Close()
	items := make([]billingInvoice, 0)
	for rows.Next() {
		var item billingInvoice
		if err := rows.Scan(&item.ID, &item.Provider, &item.ProviderInvoiceID, &item.InvoiceNumber, &item.Status, &item.Currency, &item.SubtotalAmount, &item.TaxAmount, &item.TotalAmount, &item.HostedInvoiceURL, &item.PeriodStartedAt, &item.PeriodEndsAt, &item.DueAt, &item.PaidAt, &item.CreatedAt); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load invoices")
			return
		}
		items = append(items, item)
	}
	respondJSON(writer, http.StatusOK, map[string]any{"invoices": items})
}

func (api *API) adminBillingCancellation(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "tenant.manage") {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	cancel := strings.HasSuffix(request.URL.Path, "/cancel")
	var providerSubscriptionID string
	if err := api.database.QueryRowContext(request.Context(), `SELECT COALESCE(billing_subscription_id,'') FROM tenant_subscriptions WHERE tenant_id=$1`, actor.TenantID).Scan(&providerSubscriptionID); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load subscription provider")
		return
	}
	if strings.HasPrefix(providerSubscriptionID, "repl_") {
		if !cancel {
			errorJSON(writer, http.StatusConflict, "NEW_CHECKOUT_REQUIRED", "a deactivated Xendit subscription must be restarted through checkout")
			return
		}
		secret := strings.TrimSpace(os.Getenv("XENDIT_SECRET_KEY"))
		if secret == "" {
			errorJSON(writer, http.StatusServiceUnavailable, "BILLING_NOT_CONFIGURED", "Xendit cancellation is not configured")
			return
		}
		if err := deactivateXenditPlan(request, providerSubscriptionID, secret); err != nil {
			errorJSON(writer, http.StatusBadGateway, "CANCELLATION_PROVIDER_ERROR", "Xendit could not stop future billing cycles")
			return
		}
	}
	result, err := api.database.ExecContext(request.Context(), `UPDATE tenant_subscriptions SET cancel_at_period_end=$1,updated_at=NOW() WHERE tenant_id=$2 AND status IN ('TRIALING','ACTIVE','PAST_DUE')`, cancel, actor.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update cancellation")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		errorJSON(writer, http.StatusConflict, "SUBSCRIPTION_INACTIVE", "only a current subscription can be changed")
		return
	}
	action := "resume"
	if cancel {
		action = "cancel_scheduled"
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "billing.subscription."+action, "tenant", actor.TenantID, nil)
	respondJSON(writer, http.StatusOK, map[string]any{"cancelAtPeriodEnd": cancel})
}

func (api *API) billingWebhook(writer http.ResponseWriter, request *http.Request) {
	secret := os.Getenv("BILLING_WEBHOOK_SECRET")
	if secret == "" {
		errorJSON(writer, http.StatusServiceUnavailable, "BILLING_NOT_CONFIGURED", "billing webhook is not configured")
		return
	}
	provider := strings.ToLower(strings.TrimSpace(request.PathValue("provider")))
	if !billingProviderPattern.MatchString(provider) {
		errorJSON(writer, http.StatusBadRequest, "INVALID_PROVIDER", "billing provider is invalid")
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
	if err != nil || len(body) == 0 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", "webhook body is required")
		return
	}
	if !validBillingSignature(body, request.Header.Get("X-Xpace-Signature"), secret) {
		errorJSON(writer, http.StatusUnauthorized, "INVALID_SIGNATURE", "billing webhook signature is invalid")
		return
	}
	var payload billingWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", "billing webhook JSON is invalid")
		return
	}
	payload.ID, payload.Type, payload.TenantID, payload.PlanKey = strings.TrimSpace(payload.ID), strings.TrimSpace(payload.Type), strings.TrimSpace(payload.TenantID), strings.ToUpper(strings.TrimSpace(payload.PlanKey))
	status, invoiceStatus, err := validateBillingWebhook(payload)
	if err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_BILLING_EVENT", err.Error())
		return
	}
	hash := sha256.Sum256(body)
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not process billing event")
		return
	}
	defer tx.Rollback()
	var eventID string
	err = tx.QueryRowContext(request.Context(), `INSERT INTO billing_webhook_events(provider,provider_event_id,event_type,payload_sha256) VALUES($1,$2,$3,$4) ON CONFLICT(provider,provider_event_id) DO NOTHING RETURNING id`, provider, payload.ID, payload.Type, hex.EncodeToString(hash[:])).Scan(&eventID)
	if errors.Is(err, sql.ErrNoRows) {
		var existingHash string
		if err = tx.QueryRowContext(request.Context(), `SELECT payload_sha256 FROM billing_webhook_events WHERE provider=$1 AND provider_event_id=$2`, provider, payload.ID).Scan(&existingHash); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not verify duplicate billing event")
			return
		}
		if existingHash != hex.EncodeToString(hash[:]) {
			errorJSON(writer, http.StatusConflict, "EVENT_ID_REUSED", "billing event id was reused with a different payload")
			return
		}
		respondJSON(writer, http.StatusOK, map[string]any{"received": true, "duplicate": true})
		return
	}
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not register billing event")
		return
	}
	if status != "" {
		if err = applyBillingSubscriptionEvent(request.Context(), tx, eventID, payload, status); err != nil {
			errorJSON(writer, http.StatusConflict, "SUBSCRIPTION_UPDATE_FAILED", "billing subscription update could not be applied")
			return
		}
	}
	if payload.Invoice != nil {
		if err = upsertBillingInvoice(request.Context(), tx, provider, payload.TenantID, invoiceStatus, *payload.Invoice); err != nil {
			errorJSON(writer, http.StatusConflict, "INVOICE_UPDATE_FAILED", "billing invoice update could not be applied")
			return
		}
	}
	billingMessage := "Your Xspace subscription or invoice status changed. Review billing for the latest details."
	if invoiceStatus != "" {
		billingMessage = "Invoice " + payload.Invoice.Number + " is now " + strings.ToLower(invoiceStatus) + ". Review billing for the latest details."
	}
	if err = queueWorkspaceAdminNotice(request.Context(), tx, payload.TenantID, "BILLING_NOTICE", "billing:"+provider+":"+payload.ID, map[string]any{"event": payload.Type, "message": billingMessage, "planKey": payload.PlanKey, "status": status, "invoiceStatus": invoiceStatus}); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not queue billing notice")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `UPDATE billing_webhook_events SET status='PROCESSED',processed_at=NOW() WHERE id=$1`, eventID); err != nil || tx.Commit() != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not finalize billing event")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"received": true, "duplicate": false})
}

func validBillingSignature(body []byte, provided, secret string) bool {
	provided = strings.TrimSpace(strings.TrimPrefix(provided, "sha256="))
	decoded, err := hex.DecodeString(provided)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hmac.Equal(decoded, mac.Sum(nil))
}

func validateBillingWebhook(payload billingWebhookPayload) (string, string, error) {
	if payload.ID == "" || len(payload.ID) > 200 || payload.TenantID == "" {
		return "", "", errors.New("id and tenantId are required")
	}
	status, invoiceStatus := "", ""
	switch payload.Type {
	case "subscription.activated", "subscription.renewed":
		status = "ACTIVE"
	case "subscription.past_due":
		status = "PAST_DUE"
	case "subscription.canceled":
		status = "CANCELED"
	case "subscription.suspended":
		status = "SUSPENDED"
	case "invoice.pending":
		invoiceStatus = "PENDING"
	case "invoice.paid":
		status, invoiceStatus = "ACTIVE", "PAID"
	case "invoice.payment_failed":
		status, invoiceStatus = "PAST_DUE", "FAILED"
	case "invoice.voided":
		invoiceStatus = "VOID"
	case "invoice.refunded":
		invoiceStatus = "REFUNDED"
	default:
		return "", "", errors.New("unsupported billing event type")
	}
	if status != "" && payload.PlanKey == "" {
		return "", "", errors.New("planKey is required for subscription updates")
	}
	if invoiceStatus != "" && (payload.Invoice == nil || strings.TrimSpace(payload.Invoice.ID) == "") {
		return "", "", errors.New("invoice.id is required for invoice events")
	}
	if payload.Invoice != nil {
		currency := strings.ToUpper(strings.TrimSpace(payload.Invoice.Currency))
		if len(currency) != 3 || payload.Invoice.SubtotalAmount < 0 || payload.Invoice.TaxAmount < 0 || payload.Invoice.TotalAmount < 0 {
			return "", "", errors.New("invoice currency and amounts are invalid")
		}
	}
	return status, invoiceStatus, nil
}

func applyBillingSubscriptionEvent(ctx context.Context, tx *sql.Tx, eventID string, payload billingWebhookPayload, status string) error {
	var previous string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM tenant_subscriptions WHERE tenant_id=$1 FOR UPDATE`, payload.TenantID).Scan(&previous); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE tenant_subscriptions SET plan_key=$1,status=$2,current_period_started_at=COALESCE($3,current_period_started_at),current_period_ends_at=COALESCE($4,current_period_ends_at),cancel_at_period_end=CASE WHEN $2='CANCELED' THEN FALSE ELSE cancel_at_period_end END,billing_customer_id=COALESCE(NULLIF($5,''),billing_customer_id),billing_subscription_id=COALESCE(NULLIF($6,''),billing_subscription_id),updated_at=NOW() WHERE tenant_id=$7`, payload.PlanKey, status, payload.PeriodStartedAt, payload.PeriodEndsAt, payload.CustomerID, payload.SubscriptionID, payload.TenantID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO billing_subscription_events(tenant_id,webhook_event_id,event_type,from_status,to_status,plan_key) VALUES($1,$2,$3,$4,$5,$6)`, payload.TenantID, eventID, payload.Type, previous, status, payload.PlanKey)
	return err
}

func upsertBillingInvoice(ctx context.Context, tx *sql.Tx, provider, tenantID, status string, invoice billingInvoicePayload) error {
	currency := strings.ToUpper(strings.TrimSpace(invoice.Currency))
	_, err := tx.ExecContext(ctx, `INSERT INTO billing_invoices(tenant_id,provider,provider_invoice_id,invoice_number,status,currency,subtotal_amount,tax_amount,total_amount,hosted_invoice_url,period_started_at,period_ends_at,due_at,paid_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11,$12,$13,$14) ON CONFLICT(provider,provider_invoice_id) DO UPDATE SET tenant_id=EXCLUDED.tenant_id,invoice_number=EXCLUDED.invoice_number,status=EXCLUDED.status,currency=EXCLUDED.currency,subtotal_amount=EXCLUDED.subtotal_amount,tax_amount=EXCLUDED.tax_amount,total_amount=EXCLUDED.total_amount,hosted_invoice_url=EXCLUDED.hosted_invoice_url,period_started_at=EXCLUDED.period_started_at,period_ends_at=EXCLUDED.period_ends_at,due_at=EXCLUDED.due_at,paid_at=EXCLUDED.paid_at,updated_at=NOW()`, tenantID, provider, strings.TrimSpace(invoice.ID), strings.TrimSpace(invoice.Number), status, currency, invoice.SubtotalAmount, invoice.TaxAmount, invoice.TotalAmount, strings.TrimSpace(invoice.HostedURL), invoice.PeriodStartedAt, invoice.PeriodEndsAt, invoice.DueAt, invoice.PaidAt)
	return err
}
