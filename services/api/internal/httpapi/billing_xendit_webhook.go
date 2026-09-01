package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type xenditWebhookPayload struct {
	Event   string                `json:"event"`
	Created time.Time             `json:"created"`
	Data    xenditWebhookResource `json:"data"`
}

type xenditWebhookResource struct {
	ID                 string     `json:"id"`
	ReferenceID        string     `json:"reference_id"`
	PlanID             string     `json:"plan_id"`
	CustomerID         string     `json:"customer_id"`
	PaymentSessionID   string     `json:"payment_session_id"`
	Status             string     `json:"status"`
	Currency           string     `json:"currency"`
	Amount             int64      `json:"amount"`
	ScheduledTimestamp *time.Time `json:"scheduled_timestamp"`
	Updated            *time.Time `json:"updated"`
}

func (api *API) xenditWebhook(writer http.ResponseWriter, request *http.Request) {
	token := strings.TrimSpace(os.Getenv("XENDIT_WEBHOOK_TOKEN"))
	provided := request.Header.Get("x-callback-token")
	if token == "" {
		errorJSON(writer, http.StatusServiceUnavailable, "BILLING_NOT_CONFIGURED", "Xendit webhook is not configured")
		return
	}
	if len(provided) != len(token) || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		errorJSON(writer, http.StatusUnauthorized, "INVALID_SIGNATURE", "Xendit webhook token is invalid")
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
	if err != nil || len(body) == 0 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", "webhook body is required")
		return
	}
	var payload xenditWebhookPayload
	if err = json.Unmarshal(body, &payload); err != nil || payload.Event == "" || payload.Created.IsZero() {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", "Xendit webhook JSON is invalid")
		return
	}
	resourceID := xenditWebhookResourceID(payload)
	if resourceID == "" || !supportedXenditEvent(payload.Event) {
		errorJSON(writer, http.StatusBadRequest, "UNSUPPORTED_XENDIT_EVENT", "Xendit webhook event is not supported")
		return
	}
	eventKey := payload.Event + ":" + resourceID + ":" + payload.Created.UTC().Format(time.RFC3339Nano)
	hash := sha256.Sum256(body)
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not process Xendit event")
		return
	}
	defer tx.Rollback()
	var eventID string
	err = tx.QueryRowContext(request.Context(), `INSERT INTO billing_webhook_events(provider,provider_event_id,event_type,payload_sha256) VALUES('xendit',$1,$2,$3) ON CONFLICT(provider,provider_event_id) DO NOTHING RETURNING id`, eventKey, payload.Event, hex.EncodeToString(hash[:])).Scan(&eventID)
	if errors.Is(err, sql.ErrNoRows) {
		var existingHash string
		if err = tx.QueryRowContext(request.Context(), `SELECT payload_sha256 FROM billing_webhook_events WHERE provider='xendit' AND provider_event_id=$1`, eventKey).Scan(&existingHash); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not verify duplicate Xendit event")
			return
		}
		if existingHash != hex.EncodeToString(hash[:]) {
			errorJSON(writer, http.StatusConflict, "EVENT_ID_REUSED", "Xendit event id was reused with a different payload")
			return
		}
		respondJSON(writer, http.StatusOK, map[string]any{"received": true, "duplicate": true})
		return
	}
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not register Xendit event")
		return
	}
	if err = applyXenditWebhook(request.Context(), tx, eventID, payload); err != nil {
		errorJSON(writer, http.StatusConflict, "XENDIT_EVENT_REJECTED", "Xendit event could not be matched to a checkout or subscription")
		return
	}
	if err = queueXenditBillingNotice(request.Context(), tx, payload, eventKey); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not queue billing notice")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `UPDATE billing_webhook_events SET status='PROCESSED',processed_at=NOW() WHERE id=$1`, eventID); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not finalize Xendit event")
		return
	}
	if err = tx.Commit(); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not commit Xendit event")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"received": true, "duplicate": false})
}

func queueXenditBillingNotice(ctx context.Context, tx *sql.Tx, payload xenditWebhookPayload, eventKey string) error {
	message := "Your Xspace billing status changed to " + strings.ToLower(strings.TrimSpace(payload.Data.Status)) + ". Review billing for the latest details."
	if strings.TrimSpace(payload.Data.Status) == "" {
		message = "Your Xspace subscription or invoice was updated. Review billing for the latest details."
	}
	encoded, err := json.Marshal(map[string]any{"event": payload.Event, "message": message, "status": payload.Data.Status, "amount": payload.Data.Amount, "currency": payload.Data.Currency})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `WITH target AS (
		SELECT tenant_id FROM billing_checkout_sessions WHERE provider='xendit' AND (provider_session_id=NULLIF($1,'') OR 'xpace_'||id::text=NULLIF($2,''))
		UNION
		SELECT tenant_id FROM tenant_subscriptions WHERE billing_subscription_id IN (NULLIF($3,''),NULLIF($4,''))
		LIMIT 1
	) INSERT INTO email_outbox(tenant_id,recipient_email,template,token_encrypted,payload,dedupe_key)
	SELECT target.tenant_id,u.email,'BILLING_NOTICE','',$5,'billing:xendit:'||$6||':'||u.id::text FROM target JOIN users u ON u.tenant_id=target.tenant_id
	WHERE u.status='ACTIVE' AND u.deleted_at IS NULL AND u.role IN ('TENANT_ADMIN','SUPER_ADMIN')
	ON CONFLICT(dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING`, payload.Data.PaymentSessionID, payload.Data.ReferenceID, payload.Data.ID, payload.Data.PlanID, encoded, eventKey)
	return err
}

func supportedXenditEvent(event string) bool {
	switch event {
	case "payment_session.completed", "payment_session.expired", "recurring.plan.activated", "recurring.plan.inactivated", "recurring.cycle.created", "recurring.cycle.retrying", "recurring.cycle.succeeded", "recurring.cycle.failed", "recurring.cycle.force_attempt_failed":
		return true
	default:
		return false
	}
}

func xenditWebhookResourceID(payload xenditWebhookPayload) string {
	if payload.Data.ID != "" {
		return payload.Data.ID
	}
	if payload.Data.PaymentSessionID != "" {
		return payload.Data.PaymentSessionID
	}
	return payload.Data.ReferenceID
}

func applyXenditWebhook(ctx context.Context, tx *sql.Tx, eventID string, payload xenditWebhookPayload) error {
	switch payload.Event {
	case "payment_session.completed", "payment_session.expired":
		status := "COMPLETED"
		if payload.Event == "payment_session.expired" {
			status = "EXPIRED"
		}
		result, err := tx.ExecContext(ctx, `UPDATE billing_checkout_sessions SET status=$1,updated_at=NOW() WHERE provider='xendit' AND provider_session_id=$2`, status, payload.Data.PaymentSessionID)
		if err != nil {
			return err
		}
		count, _ := result.RowsAffected()
		return requireBillingMatch(count)
	case "recurring.plan.activated":
		tenantID, planKey, err := xenditCheckoutIdentity(ctx, tx, payload.Data.PaymentSessionID, payload.Data.ReferenceID)
		if err != nil {
			return err
		}
		var previous string
		if err = tx.QueryRowContext(ctx, `SELECT status FROM tenant_subscriptions WHERE tenant_id=$1 FOR UPDATE`, tenantID).Scan(&previous); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE tenant_subscriptions SET plan_key=$1,status='ACTIVE',current_period_started_at=NOW(),current_period_ends_at=NOW()+INTERVAL '1 month',cancel_at_period_end=FALSE,billing_customer_id=COALESCE(NULLIF($2,''),billing_customer_id),billing_subscription_id=$3,updated_at=NOW() WHERE tenant_id=$4`, planKey, payload.Data.CustomerID, payload.Data.ID, tenantID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO billing_subscription_events(tenant_id,webhook_event_id,event_type,from_status,to_status,plan_key) VALUES($1,$2,$3,$4,'ACTIVE',$5)`, tenantID, eventID, payload.Event, previous, planKey)
		return err
	case "recurring.plan.inactivated":
		return inactivateXenditSubscription(ctx, tx, eventID, payload)
	case "recurring.cycle.created", "recurring.cycle.retrying":
		return upsertXenditCycle(ctx, tx, payload, "PENDING")
	case "recurring.cycle.succeeded":
		if err := upsertXenditCycle(ctx, tx, payload, "PAID"); err != nil {
			return err
		}
		return updateXenditSubscriptionStatus(ctx, tx, eventID, payload, "ACTIVE")
	case "recurring.cycle.failed", "recurring.cycle.force_attempt_failed":
		if err := upsertXenditCycle(ctx, tx, payload, "FAILED"); err != nil {
			return err
		}
		return updateXenditSubscriptionStatus(ctx, tx, eventID, payload, "PAST_DUE")
	default:
		return errors.New("unsupported event")
	}
}

func inactivateXenditSubscription(ctx context.Context, tx *sql.Tx, eventID string, payload xenditWebhookPayload) error {
	var tenantID, planKey, previous string
	var cancelAtPeriodEnd bool
	var periodEnd *time.Time
	err := tx.QueryRowContext(ctx, `SELECT tenant_id,plan_key,status,cancel_at_period_end,current_period_ends_at FROM tenant_subscriptions WHERE billing_subscription_id=$1 FOR UPDATE`, payload.Data.ID).Scan(&tenantID, &planKey, &previous, &cancelAtPeriodEnd, &periodEnd)
	if err != nil {
		return err
	}
	status := "CANCELED"
	if cancelAtPeriodEnd && periodEnd != nil && periodEnd.After(time.Now()) {
		status = "ACTIVE"
	}
	_, err = tx.ExecContext(ctx, `UPDATE tenant_subscriptions SET status=$1,cancel_at_period_end=CASE WHEN $1='CANCELED' THEN FALSE ELSE cancel_at_period_end END,updated_at=NOW() WHERE tenant_id=$2`, status, tenantID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO billing_subscription_events(tenant_id,webhook_event_id,event_type,from_status,to_status,plan_key) VALUES($1,$2,$3,$4,$5,$6)`, tenantID, eventID, payload.Event, previous, status, planKey)
	return err
}

func xenditCheckoutIdentity(ctx context.Context, tx *sql.Tx, paymentSessionID, referenceID string) (string, string, error) {
	var tenantID, planKey string
	err := tx.QueryRowContext(ctx, `SELECT tenant_id,plan_key FROM billing_checkout_sessions WHERE provider='xendit' AND (provider_session_id=NULLIF($1,'') OR 'xpace_'||id::text=NULLIF($2,'')) ORDER BY created_at DESC LIMIT 1`, paymentSessionID, referenceID).Scan(&tenantID, &planKey)
	return tenantID, planKey, err
}

func updateXenditSubscriptionStatus(ctx context.Context, tx *sql.Tx, eventID string, payload xenditWebhookPayload, status string) error {
	planID := payload.Data.PlanID
	if planID == "" {
		planID = payload.Data.ID
	}
	var tenantID, planKey, previous string
	err := tx.QueryRowContext(ctx, `SELECT tenant_id,plan_key,status FROM tenant_subscriptions WHERE billing_subscription_id=$1 FOR UPDATE`, planID).Scan(&tenantID, &planKey, &previous)
	if err != nil {
		return err
	}
	periodEnd := time.Now().AddDate(0, 1, 0)
	if payload.Data.ScheduledTimestamp != nil {
		periodEnd = payload.Data.ScheduledTimestamp.AddDate(0, 1, 0)
	}
	_, err = tx.ExecContext(ctx, `UPDATE tenant_subscriptions SET status=$1,current_period_started_at=CASE WHEN $1='ACTIVE' THEN NOW() ELSE current_period_started_at END,current_period_ends_at=CASE WHEN $1='ACTIVE' THEN $2 ELSE current_period_ends_at END,cancel_at_period_end=CASE WHEN $1='CANCELED' THEN FALSE ELSE cancel_at_period_end END,updated_at=NOW() WHERE tenant_id=$3`, status, periodEnd, tenantID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO billing_subscription_events(tenant_id,webhook_event_id,event_type,from_status,to_status,plan_key) VALUES($1,$2,$3,$4,$5,$6)`, tenantID, eventID, payload.Event, previous, status, planKey)
	return err
}

func upsertXenditCycle(ctx context.Context, tx *sql.Tx, payload xenditWebhookPayload, status string) error {
	var tenantID string
	if err := tx.QueryRowContext(ctx, `SELECT tenant_id FROM tenant_subscriptions WHERE billing_subscription_id=$1`, payload.Data.PlanID).Scan(&tenantID); err != nil {
		return err
	}
	periodStart := payload.Data.ScheduledTimestamp
	var periodEnd *time.Time
	if periodStart != nil {
		value := periodStart.AddDate(0, 1, 0)
		periodEnd = &value
	}
	var paidAt *time.Time
	if status == "PAID" {
		value := time.Now().UTC()
		paidAt = &value
	}
	currency := strings.ToUpper(payload.Data.Currency)
	if currency == "" {
		currency = "IDR"
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO billing_invoices(tenant_id,provider,provider_invoice_id,invoice_number,status,currency,subtotal_amount,total_amount,period_started_at,period_ends_at,due_at,paid_at) VALUES($1,'xendit',$2,$3,$4,$5,$6,$6,$7,$8,$7,$9) ON CONFLICT(provider,provider_invoice_id) DO UPDATE SET status=EXCLUDED.status,currency=EXCLUDED.currency,subtotal_amount=EXCLUDED.subtotal_amount,total_amount=EXCLUDED.total_amount,period_started_at=EXCLUDED.period_started_at,period_ends_at=EXCLUDED.period_ends_at,due_at=EXCLUDED.due_at,paid_at=COALESCE(EXCLUDED.paid_at,billing_invoices.paid_at),updated_at=NOW()`, tenantID, payload.Data.ID, "XENDIT-"+payload.Data.ID, status, currency, payload.Data.Amount, periodStart, periodEnd, paidAt)
	return err
}

func requireBillingMatch(count int64) error {
	if count == 0 {
		return errors.New("billing resource not found")
	}
	return nil
}
