package httpapi

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDirectoryUsersReturnsTenantProfileDetails(t *testing.T) {
	api, mock := mockAPI(t)
	now := time.Now()
	mock.ExpectQuery("FROM users u").WithArgs("tenant-1").WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "email", "role", "timezone", "locale", "bio", "has_avatar", "created_at"}).AddRow("user-2", "ciko", "Ciko", "ciko@example.com", "MEMBER", "Asia/Jakarta", "id-ID", "Product designer", true, now))

	writer := httptest.NewRecorder()
	api.directoryUsers(writer, httptest.NewRequest(http.MethodGet, "/api/v1/directory/users", nil), currentUser{TenantID: "tenant-1"})

	if writer.Code != http.StatusOK || !strings.Contains(writer.Body.String(), `"displayName":"Ciko"`) || !strings.Contains(writer.Body.String(), `"role":"MEMBER"`) || !strings.Contains(writer.Body.String(), `/api/v1/directory/users/user-2/avatar`) {
		t.Fatalf("directory returned %d: %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryAvatarIsTenantScoped(t *testing.T) {
	api, mock := mockAPI(t)
	mock.ExpectQuery("SELECT avatar_url FROM user_profiles").WithArgs("foreign-user", "tenant-1").WillReturnError(sql.ErrNoRows)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/directory/users/foreign-user/avatar", nil)
	request.SetPathValue("userID", "foreign-user")
	writer := httptest.NewRecorder()

	api.directoryUserAvatar(writer, request, currentUser{TenantID: "tenant-1"})

	if writer.Code != http.StatusNotFound {
		t.Fatalf("foreign avatar returned %d: %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
