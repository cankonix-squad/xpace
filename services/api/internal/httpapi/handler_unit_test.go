package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	passwordauth "github.com/cankonix/xpace/api/internal/auth"
)

func mockAPI(t *testing.T) (*API, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return &API{database: database}, mock
}

func TestLoginSuccessCreatesSignedSessionAndAudit(t *testing.T) {
	api, mock := mockAPI(t)
	t.Setenv("API_SESSION_SIGNING_KEY", strings.Repeat("s", 32))
	hash, err := passwordauth.HashPassword("Strong!Password#123")
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT u.id,u.tenant_id").WithArgs("cankonix", "admin").WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "slug", "name", "email", "username", "display_name", "role", "password_hash", "status"}).AddRow("user-1", "tenant-1", "cankonix", "Cankonix", "admin@example.com", "admin", "Admin", "SUPER_ADMIN", hash, "ACTIVE"))
	mock.ExpectExec("INSERT INTO sessions").WithArgs("user-1", sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(1, 1))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"Tenant":"cankonix","Identity":"admin","Password":"Strong!Password#123"}`))
	request.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	api.login(writer, request)
	if writer.Code != http.StatusOK {
		t.Fatalf("login returned %d: %s", writer.Code, writer.Body.String())
	}
	cookies := writer.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected session cookie, got %d", len(cookies))
	}
	if _, valid := verifySignedSessionToken(cookies[0].Value); !valid {
		t.Fatal("session cookie must be signed")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLoginUnknownAccountIsGenericAndAuditedWhenTenantExists(t *testing.T) {
	api, mock := mockAPI(t)
	mock.ExpectQuery("SELECT u.id,u.tenant_id").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT t.id").WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "user_id"}).AddRow("tenant-1", ""))
	mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(1, 1))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"Tenant":"cankonix","Identity":"missing@example.com","Password":"wrong"}`))
	request.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	api.login(writer, request)
	if writer.Code != http.StatusUnauthorized || strings.Contains(writer.Body.String(), "missing@example.com") {
		t.Fatalf("unsafe login response: %d %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRequireSessionAcceptsValidSignedCookie(t *testing.T) {
	api, mock := mockAPI(t)
	t.Setenv("API_SESSION_SIGNING_KEY", strings.Repeat("s", 32))
	mock.ExpectQuery("SELECT u.id,u.tenant_id").WithArgs(hashToken("opaque")).WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "slug", "name", "email", "username", "display_name", "role"}).AddRow("user-1", "tenant-1", "cankonix", "Cankonix", "a@example.com", "admin", "Admin", "SUPER_ADMIN"))
	called := false
	handler := api.requireSession(func(writer http.ResponseWriter, _ *http.Request, user currentUser) {
		called = user.ID == "user-1"
		writer.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: signedSessionToken("opaque")})
	writer := httptest.NewRecorder()
	handler(writer, request)
	if writer.Code != http.StatusNoContent || !called {
		t.Fatalf("valid session returned %d", writer.Code)
	}
}

func TestRequireSessionRejectsTamperedCookieWithoutDatabase(t *testing.T) {
	api, mock := mockAPI(t)
	t.Setenv("API_SESSION_SIGNING_KEY", strings.Repeat("s", 32))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: signedSessionToken("opaque") + "x"})
	writer := httptest.NewRecorder()
	api.requireSession(func(http.ResponseWriter, *http.Request, currentUser) { t.Fatal("handler must not run") })(writer, request)
	if writer.Code != http.StatusUnauthorized {
		t.Fatalf("tampered session returned %d", writer.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateMeetingPersistsTenantPolicyAndAudit(t *testing.T) {
	api, mock := mockAPI(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT guest_access_enabled,waiting_room_default,recording_enabled,screen_share_enabled FROM tenant_meeting_policies WHERE tenant_id=$1")).WithArgs("tenant-1").WillReturnRows(sqlmock.NewRows([]string{"guest", "waiting", "recording", "screen"}).AddRow(true, true, true, true))
	mock.ExpectQuery("INSERT INTO meetings").WithArgs("tenant-1", "user-1", sqlmock.AnyArg(), sqlmock.AnyArg(), "Planning", nil, "WAITING", true).WillReturnRows(sqlmock.NewRows([]string{"id", "status", "created_at"}).AddRow("meeting-1", "WAITING", time.Now()))
	mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(1, 1))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/meetings", strings.NewReader(`{"title":"Planning"}`))
	request.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	api.createMeeting(writer, request, currentUser{ID: "user-1", TenantID: "tenant-1", Role: roleMember})
	if writer.Code != http.StatusCreated {
		t.Fatalf("create meeting returned %d: %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListMeetingsAndTenantScopedLookup(t *testing.T) {
	api, mock := mockAPI(t)
	now := time.Now()
	mock.ExpectQuery("FROM meetings WHERE tenant_id=\\$1").WithArgs("tenant-1").WillReturnRows(sqlmock.NewRows([]string{"id", "title", "join_code", "room_name", "status", "scheduled_at", "waiting_room_enabled", "host_id", "created_at"}).AddRow("meeting-1", "Planning", "ABC-DEF-GHI", "room-1", "WAITING", nil, true, "user-1", now))
	writer := httptest.NewRecorder()
	api.listMeetings(writer, httptest.NewRequest(http.MethodGet, "/api/v1/meetings", nil), currentUser{TenantID: "tenant-1"})
	if writer.Code != http.StatusOK || !strings.Contains(writer.Body.String(), "ABC-DEF-GHI") {
		t.Fatalf("list meetings returned %d: %s", writer.Code, writer.Body.String())
	}

	mock.ExpectQuery("FROM meetings WHERE tenant_id=\\$1 AND join_code=\\$2").WithArgs("tenant-1", "ZZZ-ZZZ-ZZZ").WillReturnError(sql.ErrNoRows)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/meetings/ZZZ-ZZZ-ZZZ", nil)
	request.SetPathValue("joinCode", "ZZZ-ZZZ-ZZZ")
	writer = httptest.NewRecorder()
	api.getMeeting(writer, request, currentUser{TenantID: "tenant-1"})
	if writer.Code != http.StatusNotFound {
		t.Fatalf("missing meeting returned %d", writer.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLogoutRevokesSessionAndAudits(t *testing.T) {
	api, mock := mockAPI(t)
	t.Setenv("API_SESSION_SIGNING_KEY", strings.Repeat("s", 32))
	mock.ExpectQuery("UPDATE sessions s SET revoked_at").WithArgs(hashToken("opaque")).WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "user_id"}).AddRow("tenant-1", "user-1"))
	mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(1, 1))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: signedSessionToken("opaque")})
	writer := httptest.NewRecorder()
	api.logout(writer, request)
	if writer.Code != http.StatusNoContent || len(writer.Result().Cookies()) != 1 || writer.Result().Cookies()[0].MaxAge != -1 {
		t.Fatalf("logout returned %d", writer.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMeReturnsRolePermissions(t *testing.T) {
	writer := httptest.NewRecorder()
	(&API{}).me(writer, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil), currentUser{ID: "user-1", TenantID: "tenant-1", Role: roleMember})
	if writer.Code != http.StatusOK || !strings.Contains(writer.Body.String(), "meeting.create") {
		t.Fatalf("me returned %d: %s", writer.Code, writer.Body.String())
	}
}

func TestCoreAuthorizationDenialsDoNotTouchDatabase(t *testing.T) {
	api, mock := mockAPI(t)
	member := currentUser{ID: "user-1", TenantID: "tenant-1", Role: roleMember}
	guest := currentUser{ID: "guest-1", TenantID: "tenant-1", Role: roleGuest}
	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request, currentUser)
		user    currentUser
	}{
		{"admin dashboard", api.adminDashboard, member},
		{"admin users", api.adminUsers, member},
		{"admin groups", api.adminGroups, member},
		{"admin meetings", api.adminMeetings, member},
		{"admin audit", api.adminAuditLog, member},
		{"meeting policy", api.adminMeetingPolicy, member},
		{"system configuration", api.adminSystemConfiguration, member},
		{"guest create meeting", api.createMeeting, guest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			test.handler(writer, request, test.user)
			if writer.Code != http.StatusForbidden {
				t.Fatalf("got %d", writer.Code)
			}
		})
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadinessAndRouterHealth(t *testing.T) {
	database, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectPing()
	writer := httptest.NewRecorder()
	readiness(database)(writer, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if writer.Code != http.StatusOK {
		t.Fatalf("readiness returned %d", writer.Code)
	}

	mock.ExpectPing().WillReturnError(errors.New("offline"))
	writer = httptest.NewRecorder()
	readiness(database)(writer, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if writer.Code != http.StatusServiceUnavailable {
		t.Fatalf("degraded readiness returned %d", writer.Code)
	}

	writer = httptest.NewRecorder()
	NewRouter(database, nil).ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if writer.Code != http.StatusOK || writer.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("router health returned %d", writer.Code)
	}
}

func TestUtilityDomainBranches(t *testing.T) {
	for _, value := range []string{"xp", "xpace-1", "tenant"} {
		if !validSlug(value) {
			t.Fatalf("valid slug rejected: %s", value)
		}
	}
	for _, value := range []string{"x", "Upper", "bad_slug", strings.Repeat("a", 49)} {
		if validSlug(value) {
			t.Fatalf("invalid slug accepted: %s", value)
		}
	}
	for index := 0; index < 20; index++ {
		code, err := meetingCode()
		if err != nil || !regexp.MustCompile(`^[A-Z0-9]{3}-[A-Z0-9]{3}-[A-Z0-9]{3}$`).MatchString(code) {
			t.Fatalf("invalid meeting code %q: %v", code, err)
		}
	}
	if len(randomTokenMust(t, 24)) < 24 {
		t.Fatal("random token is unexpectedly short")
	}
}

func randomTokenMust(t *testing.T, size int) string {
	t.Helper()
	value, err := randomToken(size)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
