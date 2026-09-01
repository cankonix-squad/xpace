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

func TestGlobalSearchRejectsShortQuery(t *testing.T) {
	api, mock := mockAPI(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=x", nil)
	writer := httptest.NewRecorder()
	api.globalSearch(writer, request, currentUser{ID: "user-1", TenantID: "tenant-1"})
	if writer.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGlobalSearchAggregatesPermissionScopedModules(t *testing.T) {
	api, mock := mockAPI(t)
	updated := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	args := []driver.Value{"tenant-1", "user-1", "%team%"}
	mock.ExpectQuery("FROM meetings").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id", "title", "join_code", "updated_at"}).AddRow("meeting-1", "Team sync", "ABC-DEF-GHI", updated))
	mock.ExpectQuery("FROM users").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id", "title", "subtitle", "updated_at"}))
	mock.ExpectQuery("FROM chat_conversations").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id", "title", "subtitle", "updated_at"}))
	mock.ExpectQuery("FROM workspace_rooms").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id", "title", "subtitle", "updated_at"}))
	mock.ExpectQuery("FROM drive_nodes").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id", "title", "subtitle", "updated_at"}))
	mock.ExpectQuery("FROM calendar_events").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id", "title", "subtitle", "updated_at"}))
	mock.ExpectQuery("FROM recordings").WithArgs("tenant-1", "user-1", "%team%", true).WillReturnRows(sqlmock.NewRows([]string{"id", "title", "subtitle", "updated_at"}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=team", nil)
	writer := httptest.NewRecorder()
	api.globalSearch(writer, request, currentUser{ID: "user-1", TenantID: "tenant-1", Role: roleTenantAdmin})
	if writer.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", writer.Code, writer.Body.String())
	}
	body := writer.Body.String()
	for _, expected := range []string{`"type":"meeting"`, `"title":"Team sync"`, `"href":"/meet/ABC-DEF-GHI/prejoin"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response missing %s: %s", expected, body)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
