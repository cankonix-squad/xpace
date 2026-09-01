package httpapi

import (
	"database/sql/driver"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestWorkspaceActivityRejectsInvalidCursor(t *testing.T) {
	api, mock := mockAPI(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/activity?before=not-a-date", nil)
	writer := httptest.NewRecorder()
	api.workspaceActivity(writer, request, currentUser{ID: "user-1", TenantID: "tenant-1"})
	if writer.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceActivityAggregatesAccessibleModules(t *testing.T) {
	api, mock := mockAPI(t)
	created := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
	standardArgs := []driver.Value{"tenant-1", "user-1", sqlmock.AnyArg(), 13}
	mock.ExpectQuery("FROM meetings").WithArgs(standardArgs...).WillReturnRows(sqlmock.NewRows([]string{"id", "title", "description", "actor_name", "link", "created_at"}).AddRow("meeting-1", "Product sync", "Meeting scheduled", "Admin", "ABC-DEF-GHI", created))
	mock.ExpectQuery("FROM chat_messages").WithArgs(standardArgs...).WillReturnRows(activityRows())
	mock.ExpectQuery("FROM workspace_room_activity").WithArgs(standardArgs...).WillReturnRows(activityRows())
	mock.ExpectQuery("FROM drive_nodes").WithArgs(standardArgs...).WillReturnRows(activityRows())
	mock.ExpectQuery("FROM calendar_events").WithArgs(standardArgs...).WillReturnRows(activityRows())
	mock.ExpectQuery("FROM recordings").WithArgs("tenant-1", "user-1", true, sqlmock.AnyArg(), 13).WillReturnRows(activityRows())

	request := httptest.NewRequest(http.MethodGet, "/api/v1/activity?limit=12", nil)
	writer := httptest.NewRecorder()
	api.workspaceActivity(writer, request, currentUser{ID: "user-1", TenantID: "tenant-1", Role: roleTenantAdmin})
	if writer.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", writer.Code, writer.Body.String())
	}
	body := writer.Body.String()
	for _, expected := range []string{`"type":"meeting"`, `"title":"Product sync"`, `"href":"/meet/ABC-DEF-GHI/prejoin"`, `"hasMore":false`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response missing %s: %s", expected, body)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func activityRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "title", "description", "actor_name", "link", "created_at"})
}
