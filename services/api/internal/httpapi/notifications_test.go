package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNotificationReadAllUpdatesEveryUnreadNotification(t *testing.T) {
	api, mock := mockAPI(t)
	mock.ExpectExec("UPDATE notifications SET read_at=NOW\\(\\) WHERE tenant_id=\\$1 AND recipient_id=\\$2 AND read_at IS NULL").WithArgs("tenant-1", "user-1").WillReturnResult(sqlmock.NewResult(0, 3))
	writer := httptest.NewRecorder()
	api.notificationRead(writer, httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read-all", nil), currentUser{ID: "user-1", TenantID: "tenant-1"})
	if writer.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", writer.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationReadUpdatesOnlyRequestedNotification(t *testing.T) {
	api, mock := mockAPI(t)
	mock.ExpectExec("UPDATE notifications SET read_at=NOW\\(\\) WHERE id=\\$1").WithArgs("notification-1", "tenant-1", "user-1").WillReturnResult(sqlmock.NewResult(0, 1))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/notification-1/read", nil)
	request.SetPathValue("notificationID", "notification-1")
	writer := httptest.NewRecorder()
	api.notificationRead(writer, request, currentUser{ID: "user-1", TenantID: "tenant-1"})
	if writer.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", writer.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
