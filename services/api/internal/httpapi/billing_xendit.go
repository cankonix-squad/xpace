package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"
)

type xenditSessionResponse struct {
	PaymentSessionID string    `json:"payment_session_id"`
	PaymentLinkURL   string    `json:"payment_link_url"`
	CustomerID       string    `json:"customer_id"`
	ExpiresAt        time.Time `json:"expires_at"`
}

var xenditHTTPClient = &http.Client{Timeout: 15 * time.Second}

func (api *API) adminBillingCheckout(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "tenant.manage") {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	secret := strings.TrimSpace(os.Getenv("XENDIT_SECRET_KEY"))
	if secret == "" {
		errorJSON(writer, http.StatusServiceUnavailable, "BILLING_NOT_CONFIGURED", "Xendit checkout is not configured")
		return
	}
	var input struct {
		PlanKey string `json:"planKey"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	input.PlanKey = strings.ToUpper(strings.TrimSpace(input.PlanKey))
	var planName, customerID string
	var amount int64
	err := api.database.QueryRowContext(request.Context(), `SELECT p.name,p.price_monthly_idr,COALESCE(s.billing_customer_id,'') FROM saas_plans p JOIN tenant_subscriptions s ON s.tenant_id=$1 WHERE p.key=$2 AND p.active=TRUE AND p.price_monthly_idr>0`, actor.TenantID, input.PlanKey).Scan(&planName, &amount, &customerID)
	if errors.Is(err, sql.ErrNoRows) {
		errorJSON(writer, http.StatusBadRequest, "PLAN_NOT_AVAILABLE", "the selected self-service plan is not available")
		return
	}
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load selected plan")
		return
	}
	var existingID, existingURL string
	var existingExpiry time.Time
	err = api.database.QueryRowContext(request.Context(), `SELECT id,checkout_url,expires_at FROM billing_checkout_sessions WHERE tenant_id=$1 AND plan_key=$2 AND provider='xendit' AND status='OPEN' AND expires_at>NOW() ORDER BY created_at DESC LIMIT 1`, actor.TenantID, input.PlanKey).Scan(&existingID, &existingURL, &existingExpiry)
	if err == nil {
		respondJSON(writer, http.StatusOK, map[string]any{"id": existingID, "checkoutUrl": existingURL, "expiresAt": existingExpiry, "reused": true})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not inspect checkout state")
		return
	}
	var checkoutID string
	err = api.database.QueryRowContext(request.Context(), `INSERT INTO billing_checkout_sessions(tenant_id,requested_by,plan_key,provider) VALUES($1,$2,$3,'xendit') RETURNING id`, actor.TenantID, actor.ID, input.PlanKey).Scan(&checkoutID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not initialize checkout")
		return
	}
	payload := xenditCheckoutPayload(checkoutID, input.PlanKey, planName, amount, customerID, actor)
	response, err := createXenditSession(request, payload, secret)
	if err != nil {
		_, _ = api.database.ExecContext(request.Context(), `UPDATE billing_checkout_sessions SET status='CANCELED',updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, checkoutID, actor.TenantID)
		errorJSON(writer, http.StatusBadGateway, "CHECKOUT_PROVIDER_ERROR", "Xendit could not create a checkout session")
		return
	}
	if _, err = api.database.ExecContext(request.Context(), `UPDATE billing_checkout_sessions SET provider_session_id=$1,status='OPEN',checkout_url=$2,expires_at=$3,updated_at=NOW() WHERE id=$4 AND tenant_id=$5`, response.PaymentSessionID, response.PaymentLinkURL, response.ExpiresAt, checkoutID, actor.TenantID); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "checkout was created but could not be stored")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "billing.checkout.create", "billing_checkout", checkoutID, map[string]any{"provider": "xendit", "planKey": input.PlanKey})
	respondJSON(writer, http.StatusCreated, map[string]any{"id": checkoutID, "checkoutUrl": response.PaymentLinkURL, "expiresAt": response.ExpiresAt, "reused": false})
}

func xenditCheckoutPayload(checkoutID, planKey, planName string, amount int64, customerID string, actor currentUser) map[string]any {
	publicURL := strings.TrimRight(strings.TrimSpace(os.Getenv("XPACE_PUBLIC_URL")), "/")
	if publicURL == "" {
		publicURL = "https://xspace.cankonix.com"
	}
	payload := map[string]any{
		"reference_id": "xpace_" + checkoutID,
		"session_type": "SUBSCRIPTION",
		"mode":         "PAYMENT_LINK",
		"amount":       amount,
		"currency":     "IDR",
		"country":      "ID",
		"locale":       "id",
		"description":  "Xspace " + planName + " monthly subscription",
		"subscription": map[string]any{
			"schedule": map[string]any{
				"interval":                     "MONTH",
				"interval_count":               1,
				"anchor_date":                  nextXenditAnchor(time.Now()).Format(time.RFC3339),
				"retry_interval":               "DAY",
				"retry_interval_count":         1,
				"total_retry":                  3,
				"failed_attempt_notifications": []int{1, 2, 3},
			},
			"failed_cycle_action":             "RESUME",
			"payment_link_for_failed_attempt": true,
		},
		"notification_channels": []string{"EMAIL"},
		"success_return_url":    publicURL + "/admin/billing?checkout=success",
		"cancel_return_url":     publicURL + "/admin/billing?checkout=canceled",
		"metadata": map[string]string{
			"tenant_id":   actor.TenantID,
			"plan_key":    planKey,
			"checkout_id": checkoutID,
		},
	}
	if customerID != "" {
		payload["customer_id"] = customerID
	} else {
		payload["customer"] = map[string]any{
			"reference_id": "xpace" + strings.ReplaceAll(actor.TenantID, "-", ""),
			"type":         "INDIVIDUAL",
			"email":        actor.Email,
			"individual_detail": map[string]string{
				"given_names": xenditSafeName(actor.DisplayName),
			},
		}
	}
	return payload
}

func createXenditSession(request *http.Request, payload map[string]any, secret string) (xenditSessionResponse, error) {
	var result xenditSessionResponse
	body, err := json.Marshal(payload)
	if err != nil {
		return result, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("XENDIT_API_URL")), "/")
	if baseURL == "" {
		baseURL = "https://api.xendit.co"
	}
	providerRequest, err := http.NewRequestWithContext(request.Context(), http.MethodPost, baseURL+"/sessions", bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	providerRequest.SetBasicAuth(secret, "")
	providerRequest.Header.Set("Content-Type", "application/json")
	providerRequest.Header.Set("api-version", "2026-01-01")
	providerResponse, err := xenditHTTPClient.Do(providerRequest)
	if err != nil {
		return result, err
	}
	defer providerResponse.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(providerResponse.Body, 1<<20))
	if err != nil || providerResponse.StatusCode < 200 || providerResponse.StatusCode >= 300 {
		return result, errors.New("xendit session request failed")
	}
	if err = json.Unmarshal(responseBody, &result); err != nil || result.PaymentSessionID == "" || result.PaymentLinkURL == "" || result.ExpiresAt.IsZero() || !validXenditCheckoutURL(result.PaymentLinkURL) {
		return result, errors.New("xendit session response is invalid")
	}
	return result, nil
}

func deactivateXenditPlan(request *http.Request, planID, secret string) error {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("XENDIT_API_URL")), "/")
	if baseURL == "" {
		baseURL = "https://api.xendit.co"
	}
	providerRequest, err := http.NewRequestWithContext(request.Context(), http.MethodPost, baseURL+"/recurring/plans/"+url.PathEscape(planID)+"/deactivate", nil)
	if err != nil {
		return err
	}
	providerRequest.SetBasicAuth(secret, "")
	providerRequest.Header.Set("api-version", "2026-01-01")
	providerResponse, err := xenditHTTPClient.Do(providerRequest)
	if err != nil {
		return err
	}
	defer providerResponse.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(providerResponse.Body, 1<<20))
	if providerResponse.StatusCode < 200 || providerResponse.StatusCode >= 300 {
		return errors.New("xendit plan deactivation failed")
	}
	return nil
}

func nextXenditAnchor(now time.Time) time.Time {
	anchor := now.Add(15 * time.Minute)
	if anchor.Day() > 28 {
		anchor = time.Date(anchor.Year(), anchor.Month()+1, 1, 9, 0, 0, 0, anchor.Location())
	}
	return anchor
}

func xenditSafeName(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsNumber(character) || unicode.IsSpace(character) {
			return character
		}
		return -1
	}, strings.TrimSpace(value))
	if value == "" {
		return "Xspace Owner"
	}
	return value
}

func validXenditCheckoutURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "xen.to" || host == "dev.xen.to" || host == "checkout.xendit.co" || host == "checkout-staging.xendit.co"
}
