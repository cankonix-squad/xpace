package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestApplyLimitOverride(t *testing.T) {
	plan := planEntitlements{MaxUsers: 5, MaxStorageBytes: 100}
	applyLimitOverride(&plan, "users.max", 12)
	applyLimitOverride(&plan, "storage.bytes", 2048)
	if plan.MaxUsers != 12 || plan.MaxStorageBytes != 2048 {
		t.Fatalf("overrides were not applied: %+v", plan)
	}
}

func TestMeetingQuotaExceeded(t *testing.T) {
	api, mock := mockAPI(t)
	expectSubscriptionQueries(mock, 5, 100, 0)
	err := api.enforceTenantQuota(context.Background(), "tenant-1", "meetings", 1)
	var quota *entitlementError
	if !errors.As(err, &quota) || quota.code != "QUOTA_EXCEEDED" || quota.status != 402 {
		t.Fatalf("expected quota error, got %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExpiredTrialBlocksWrites(t *testing.T) {
	api, mock := mockAPI(t)
	mock.ExpectQuery("SELECT p.key,p.name").WithArgs("tenant-1").WillReturnRows(planRow("TRIALING", time.Now().Add(-time.Hour), 100))
	mock.ExpectQuery("SELECT entitlement_key,enabled,limit_value").WithArgs("tenant-1").WillReturnRows(sqlmock.NewRows([]string{"key", "enabled", "limit"}))
	mock.ExpectQuery("FROM users WHERE tenant_id=\\$1").WithArgs("tenant-1").WillReturnRows(sqlmock.NewRows([]string{"users", "meetings", "storage", "recordings"}).AddRow(1, 0, 0, 0))
	err := api.enforceTenantQuota(context.Background(), "tenant-1", "meetings", 1)
	var quota *entitlementError
	if !errors.As(err, &quota) || quota.code != "SUBSCRIPTION_INACTIVE" {
		t.Fatalf("expected inactive subscription, got %v", err)
	}
}

func expectSubscriptionQueries(mock sqlmock.Sqlmock, users, meetings, storage int64) {
	mock.ExpectQuery("SELECT p.key,p.name").WithArgs("tenant-1").WillReturnRows(planRow("ACTIVE", time.Now().Add(24*time.Hour), meetings))
	mock.ExpectQuery("SELECT entitlement_key,enabled,limit_value").WithArgs("tenant-1").WillReturnRows(sqlmock.NewRows([]string{"key", "enabled", "limit"}))
	mock.ExpectQuery("FROM users WHERE tenant_id=\\$1").WithArgs("tenant-1").WillReturnRows(sqlmock.NewRows([]string{"users", "meetings", "storage", "recordings"}).AddRow(users, meetings, storage, 0))
}

func planRow(status string, end time.Time, maxMeetings int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"key", "name", "description", "price", "trial_days", "max_users", "max_meetings", "max_duration", "max_storage", "max_recordings", "features", "status", "trial_ends", "period_ends", "cancel"}).AddRow("STARTER", "Starter", "Starter plan", 149000, 14, 5, maxMeetings, 60, int64(5368709120), 10, []byte(`{"recording":true,"drive":true,"chatAttachments":true}`), status, end, end, false)
}
