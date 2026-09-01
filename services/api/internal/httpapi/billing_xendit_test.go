package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestXenditWebhookCompletesCheckout(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	t.Setenv("XENDIT_WEBHOOK_TOKEN", "callback-token")
	body := []byte(`{"event":"payment_session.completed","created":"2026-08-26T03:00:00Z","data":{"payment_session_id":"ps-661f87c614802d6c402cd82d","status":"COMPLETED"}}`)
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO billing_webhook_events").WithArgs("payment_session.completed:ps-661f87c614802d6c402cd82d:2026-08-26T03:00:00Z", "payment_session.completed", sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("event-1"))
	mock.ExpectExec("UPDATE billing_checkout_sessions").WithArgs("COMPLETED", "ps-661f87c614802d6c402cd82d").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("WITH target AS").WithArgs("ps-661f87c614802d6c402cd82d", "", "", "", sqlmock.AnyArg(), "payment_session.completed:ps-661f87c614802d6c402cd82d:2026-08-26T03:00:00Z").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE billing_webhook_events").WithArgs("event-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/billing/webhooks/xendit/native", bytes.NewReader(body))
	request.Header.Set("x-callback-token", "callback-token")
	writer := httptest.NewRecorder()
	(&API{database: database}).xenditWebhook(writer, request)
	if writer.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", writer.Code, writer.Body.String())
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestXenditWebhookRejectsInvalidToken(t *testing.T) {
	t.Setenv("XENDIT_WEBHOOK_TOKEN", "callback-token")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/billing/webhooks/xendit/native", bytes.NewReader([]byte(`{}`)))
	request.Header.Set("x-callback-token", "wrong-token")
	writer := httptest.NewRecorder()
	(&API{}).xenditWebhook(writer, request)
	if writer.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", writer.Code)
	}
}

func TestCreateXenditSession(t *testing.T) {
	expiresAt := time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second)
	originalClient := xenditHTTPClient
	t.Cleanup(func() { xenditHTTPClient = originalClient })
	xenditHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		username, _, ok := request.BasicAuth()
		if !ok || username != "xnd_test_secret" {
			t.Error("missing Xendit basic authentication")
		}
		if request.Header.Get("api-version") != "2026-01-01" {
			t.Error("unexpected Xendit API version")
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["session_type"] != "SUBSCRIPTION" || body["currency"] != "IDR" {
			t.Fatalf("unexpected checkout payload: %+v", body)
		}
		responseBody, _ := json.Marshal(map[string]any{
			"payment_session_id": "ps-661f87c614802d6c402cd82d",
			"payment_link_url":   "https://dev.xen.to/test-session",
			"customer_id":        "cust-e2878b4c-d57e-4a2c-922d-c0313c2800a3",
			"expires_at":         expiresAt,
		})
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(responseBody))}, nil
	})}
	t.Setenv("XENDIT_API_URL", "https://api.xendit.test")
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	response, err := createXenditSession(request, map[string]any{"session_type": "SUBSCRIPTION", "currency": "IDR"}, "xnd_test_secret")
	if err != nil {
		t.Fatal(err)
	}
	if response.PaymentSessionID == "" || response.PaymentLinkURL != "https://dev.xen.to/test-session" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestXenditCheckoutHelpers(t *testing.T) {
	if !validXenditCheckoutURL("https://checkout.xendit.co/sessions/example") || validXenditCheckoutURL("https://xendit.example/redirect") {
		t.Fatal("checkout URL allowlist is incorrect")
	}
	anchor := nextXenditAnchor(time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC))
	if anchor.Day() != 1 || anchor.Month() != time.September {
		t.Fatalf("unexpected safe anchor: %v", anchor)
	}
	if name := xenditSafeName("Ciko @ Xpace!"); name != "Ciko  Xpace" {
		t.Fatalf("unexpected sanitized name: %q", name)
	}
}

func TestDeactivateXenditPlan(t *testing.T) {
	originalClient := xenditHTTPClient
	t.Cleanup(func() { xenditHTTPClient = originalClient })
	xenditHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/recurring/plans/repl_test/deactivate" {
			t.Fatalf("unexpected deactivation request: %s %s", request.Method, request.URL.Path)
		}
		username, _, ok := request.BasicAuth()
		if !ok || username != "xnd_test_secret" || request.Header.Get("api-version") != "2026-01-01" {
			t.Fatal("missing Xendit authentication or API version")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader([]byte(`{}`)))}, nil
	})}
	t.Setenv("XENDIT_API_URL", "https://api.xendit.test")
	if err := deactivateXenditPlan(httptest.NewRequest(http.MethodPost, "/", nil), "repl_test", "xnd_test_secret"); err != nil {
		t.Fatal(err)
	}
}
