package httpapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestValidBillingSignature(t *testing.T) {
	body := []byte(`{"id":"evt_1"}`)
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !validBillingSignature(body, signature, "test-secret") {
		t.Fatal("expected valid signature")
	}
	if validBillingSignature([]byte(`{"id":"changed"}`), signature, "test-secret") {
		t.Fatal("changed payload must not pass signature verification")
	}
}

func TestBillingWebhookAppliesPaidInvoiceAtomically(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	t.Setenv("BILLING_WEBHOOK_SECRET", "test-secret")
	body := []byte(`{"id":"evt_paid_1","type":"invoice.paid","tenantId":"tenant-1","planKey":"BUSINESS","customerId":"customer-1","subscriptionId":"subscription-1","invoice":{"id":"invoice-1","number":"XP-0001","currency":"IDR","subtotalAmount":499000,"taxAmount":0,"totalAmount":499000}}`)
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write(body)

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO billing_webhook_events").WithArgs("adapter", "evt_paid_1", "invoice.paid", sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("event-1"))
	mock.ExpectQuery("SELECT status FROM tenant_subscriptions").WithArgs("tenant-1").WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("TRIALING"))
	mock.ExpectExec("UPDATE tenant_subscriptions").WithArgs("BUSINESS", "ACTIVE", nil, nil, "customer-1", "subscription-1", "tenant-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO billing_subscription_events").WithArgs("tenant-1", "event-1", "invoice.paid", "TRIALING", "ACTIVE", "BUSINESS").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO billing_invoices").WithArgs("tenant-1", "adapter", "invoice-1", "XP-0001", "PAID", "IDR", int64(499000), int64(0), int64(499000), "", nil, nil, nil, nil).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO email_outbox").WithArgs("tenant-1", "BILLING_NOTICE", sqlmock.AnyArg(), "billing:adapter:evt_paid_1").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE billing_webhook_events").WithArgs("event-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	request := httptest.NewRequest(http.MethodPost, "/api/v1/billing/webhooks/adapter", bytes.NewReader(body))
	request.SetPathValue("provider", "adapter")
	request.Header.Set("X-Xpace-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	writer := httptest.NewRecorder()
	(&API{database: database}).billingWebhook(writer, request)
	if writer.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", writer.Code, writer.Body.String())
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateBillingWebhook(t *testing.T) {
	periodEnd := time.Now().Add(30 * 24 * time.Hour)
	payload := billingWebhookPayload{
		ID:           "evt_paid_1",
		Type:         "invoice.paid",
		TenantID:     "tenant-1",
		PlanKey:      "BUSINESS",
		PeriodEndsAt: &periodEnd,
		Invoice: &billingInvoicePayload{
			ID:             "invoice-1",
			Currency:       "IDR",
			SubtotalAmount: 499000,
			TotalAmount:    499000,
		},
	}
	status, invoiceStatus, err := validateBillingWebhook(payload)
	if err != nil {
		t.Fatalf("expected valid event: %v", err)
	}
	if status != "ACTIVE" || invoiceStatus != "PAID" {
		t.Fatalf("unexpected transitions: subscription=%s invoice=%s", status, invoiceStatus)
	}
	payload.Type = "unknown.event"
	if _, _, err = validateBillingWebhook(payload); err == nil {
		t.Fatal("unsupported event must be rejected")
	}
}
