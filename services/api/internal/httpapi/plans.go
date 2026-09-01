package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

type planEntitlements struct {
	Key                       string          `json:"key"`
	Name                      string          `json:"name"`
	Description               string          `json:"description"`
	PriceMonthlyIDR           int64           `json:"priceMonthlyIdr"`
	TrialDays                 int             `json:"trialDays"`
	MaxUsers                  int64           `json:"maxUsers"`
	MaxMeetingsPerMonth       int64           `json:"maxMeetingsPerMonth"`
	MaxMeetingDurationMinutes int64           `json:"maxMeetingDurationMinutes"`
	MaxStorageBytes           int64           `json:"maxStorageBytes"`
	MaxRecordingsPerMonth     int64           `json:"maxRecordingsPerMonth"`
	Features                  map[string]bool `json:"features"`
}

type subscriptionUsage struct {
	Users               int64 `json:"users"`
	MeetingsThisMonth   int64 `json:"meetingsThisMonth"`
	StorageBytes        int64 `json:"storageBytes"`
	RecordingsThisMonth int64 `json:"recordingsThisMonth"`
}

type tenantSubscription struct {
	Plan                planEntitlements  `json:"plan"`
	Status              string            `json:"status"`
	TrialEndsAt         *time.Time        `json:"trialEndsAt"`
	CurrentPeriodEndsAt *time.Time        `json:"currentPeriodEndsAt"`
	CancelAtPeriodEnd   bool              `json:"cancelAtPeriodEnd"`
	Usage               subscriptionUsage `json:"usage"`
	CheckoutEnabled     bool              `json:"checkoutEnabled"`
	BillingProvider     string            `json:"billingProvider,omitempty"`
	ProviderManaged     bool              `json:"providerManaged"`
}

type entitlementError struct {
	code, message string
	status        int
}

func (err *entitlementError) Error() string { return err.message }

func (api *API) publicPlans(writer http.ResponseWriter, request *http.Request) {
	rows, err := api.database.QueryContext(request.Context(), `SELECT key,name,description,price_monthly_idr,trial_days,max_users,max_meetings_per_month,max_meeting_duration_minutes,max_storage_bytes,max_recordings_per_month,features FROM saas_plans WHERE active=TRUE ORDER BY sort_order,key`)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load plans")
		return
	}
	defer rows.Close()
	plans := make([]planEntitlements, 0)
	for rows.Next() {
		var plan planEntitlements
		var features []byte
		if err = rows.Scan(&plan.Key, &plan.Name, &plan.Description, &plan.PriceMonthlyIDR, &plan.TrialDays, &plan.MaxUsers, &plan.MaxMeetingsPerMonth, &plan.MaxMeetingDurationMinutes, &plan.MaxStorageBytes, &plan.MaxRecordingsPerMonth, &features); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load plans")
			return
		}
		_ = json.Unmarshal(features, &plan.Features)
		plans = append(plans, plan)
	}
	respondJSON(writer, 200, map[string]any{"plans": plans})
}

func (api *API) adminSubscription(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "analytics.read") {
		errorJSON(writer, 403, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	subscription, err := api.loadTenantSubscription(request.Context(), actor.TenantID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load subscription")
		return
	}
	if strings.TrimSpace(os.Getenv("XENDIT_SECRET_KEY")) != "" {
		subscription.CheckoutEnabled = true
		subscription.BillingProvider = "xendit"
	}
	if err = api.database.QueryRowContext(request.Context(), `SELECT billing_subscription_id IS NOT NULL FROM tenant_subscriptions WHERE tenant_id=$1`, actor.TenantID).Scan(&subscription.ProviderManaged); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load billing provider state")
		return
	}
	respondJSON(writer, 200, map[string]any{"subscription": subscription})
}

func (api *API) loadTenantSubscription(ctx context.Context, tenantID string) (tenantSubscription, error) {
	var item tenantSubscription
	var features []byte
	err := api.database.QueryRowContext(ctx, `SELECT p.key,p.name,p.description,p.price_monthly_idr,p.trial_days,p.max_users,p.max_meetings_per_month,p.max_meeting_duration_minutes,p.max_storage_bytes,p.max_recordings_per_month,p.features,s.status,s.trial_ends_at,s.current_period_ends_at,s.cancel_at_period_end FROM tenant_subscriptions s JOIN saas_plans p ON p.key=s.plan_key WHERE s.tenant_id=$1`, tenantID).Scan(&item.Plan.Key, &item.Plan.Name, &item.Plan.Description, &item.Plan.PriceMonthlyIDR, &item.Plan.TrialDays, &item.Plan.MaxUsers, &item.Plan.MaxMeetingsPerMonth, &item.Plan.MaxMeetingDurationMinutes, &item.Plan.MaxStorageBytes, &item.Plan.MaxRecordingsPerMonth, &features, &item.Status, &item.TrialEndsAt, &item.CurrentPeriodEndsAt, &item.CancelAtPeriodEnd)
	if err != nil {
		return item, err
	}
	_ = json.Unmarshal(features, &item.Plan.Features)
	rows, err := api.database.QueryContext(ctx, `SELECT entitlement_key,enabled,limit_value FROM tenant_entitlements WHERE tenant_id=$1`, tenantID)
	if err != nil {
		return item, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var enabled sql.NullBool
		var limit sql.NullInt64
		if err = rows.Scan(&key, &enabled, &limit); err != nil {
			return item, err
		}
		if enabled.Valid {
			item.Plan.Features[key] = enabled.Bool
		}
		if limit.Valid {
			applyLimitOverride(&item.Plan, key, limit.Int64)
		}
	}
	err = api.database.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM users WHERE tenant_id=$1 AND status!='DEACTIVATED'),(SELECT COUNT(*) FROM meetings WHERE tenant_id=$1 AND created_at>=date_trunc('month',NOW())),(SELECT COALESCE(SUM(size_bytes),0) FROM drive_nodes WHERE tenant_id=$1 AND kind='FILE' AND deleted_at IS NULL)+(SELECT COALESCE(SUM(size_bytes),0) FROM chat_attachments WHERE tenant_id=$1)+(SELECT COALESCE(SUM(size_bytes),0) FROM recordings WHERE tenant_id=$1 AND status='READY'),(SELECT COUNT(*) FROM recordings WHERE tenant_id=$1 AND created_at>=date_trunc('month',NOW()) AND status!='FAILED')`, tenantID).Scan(&item.Usage.Users, &item.Usage.MeetingsThisMonth, &item.Usage.StorageBytes, &item.Usage.RecordingsThisMonth)
	return item, err
}

func applyLimitOverride(plan *planEntitlements, key string, value int64) {
	switch key {
	case "users.max":
		plan.MaxUsers = value
	case "meetings.monthly":
		plan.MaxMeetingsPerMonth = value
	case "meeting.durationMinutes":
		plan.MaxMeetingDurationMinutes = value
	case "storage.bytes":
		plan.MaxStorageBytes = value
	case "recordings.monthly":
		plan.MaxRecordingsPerMonth = value
	}
}

func (api *API) enforceTenantQuota(ctx context.Context, tenantID, resource string, increment int64) error {
	item, err := api.loadTenantSubscription(ctx, tenantID)
	if err != nil {
		return err
	}
	now := time.Now()
	active := item.Status == "ACTIVE" && (item.CurrentPeriodEndsAt == nil || item.CurrentPeriodEndsAt.After(now)) || item.Status == "TRIALING" && item.TrialEndsAt != nil && item.TrialEndsAt.After(now)
	if !active {
		return &entitlementError{status: 402, code: "SUBSCRIPTION_INACTIVE", message: "workspace subscription is not active"}
	}
	var used, limit int64
	switch resource {
	case "users":
		used, limit = item.Usage.Users, item.Plan.MaxUsers
	case "meetings":
		used, limit = item.Usage.MeetingsThisMonth, item.Plan.MaxMeetingsPerMonth
	case "recordings":
		if !item.Plan.Features["recording"] {
			return &entitlementError{status: 403, code: "FEATURE_NOT_INCLUDED", message: "recording is not included in this plan"}
		}
		used, limit = item.Usage.RecordingsThisMonth, item.Plan.MaxRecordingsPerMonth
	case "storage":
		used, limit = item.Usage.StorageBytes, item.Plan.MaxStorageBytes
	case "drive":
		if !item.Plan.Features["drive"] {
			return &entitlementError{status: 403, code: "FEATURE_NOT_INCLUDED", message: "drive is not included in this plan"}
		}
		used, limit = item.Usage.StorageBytes, item.Plan.MaxStorageBytes
	case "chatAttachments":
		if !item.Plan.Features["chatAttachments"] {
			return &entitlementError{status: 403, code: "FEATURE_NOT_INCLUDED", message: "chat attachments are not included in this plan"}
		}
		used, limit = item.Usage.StorageBytes, item.Plan.MaxStorageBytes
	default:
		return errors.New("unsupported quota resource")
	}
	if used+increment > limit {
		return &entitlementError{status: 402, code: "QUOTA_EXCEEDED", message: "workspace plan quota exceeded"}
	}
	return nil
}

func respondEntitlementError(writer http.ResponseWriter, err error) bool {
	var limitErr *entitlementError
	if errors.As(err, &limitErr) {
		errorJSON(writer, limitErr.status, limitErr.code, limitErr.message)
		return true
	}
	return false
}
