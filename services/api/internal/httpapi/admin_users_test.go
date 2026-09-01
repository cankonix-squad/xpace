package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestValidManagedRole(t *testing.T) {
	if validManagedRole(roleTenantAdmin, roleSuperAdmin) {
		t.Fatal("tenant admin must not assign super admin")
	}
	if !validManagedRole(roleSuperAdmin, roleSuperAdmin) {
		t.Fatal("super admin must be able to assign super admin")
	}
	if !validManagedRole(roleTenantAdmin, roleMember) {
		t.Fatal("tenant admin must be able to assign member")
	}
	if validManagedRole(roleSuperAdmin, userRole("UNKNOWN")) {
		t.Fatal("unknown role must be rejected")
	}
}

func TestCreateActiveUserRejectsPasswordMismatch(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	api := &API{database: database}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewBufferString(`{"email":"ciko@example.com","username":"ciko","displayName":"Ciko","role":"MEMBER","status":"ACTIVE","password":"StrongPassword!2026","passwordConfirm":"DifferentPassword!2026"}`))
	request.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	api.createAdminUser(writer, request, currentUser{TenantID: "tenant-1", Role: roleSuperAdmin})
	if writer.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, received %d: %s", writer.Code, writer.Body.String())
	}
	if !bytes.Contains(writer.Body.Bytes(), []byte(`"code":"PASSWORD_MISMATCH"`)) {
		t.Fatalf("expected PASSWORD_MISMATCH response: %s", writer.Body.String())
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValidUserStatus(t *testing.T) {
	for _, status := range []string{"ACTIVE", "INVITED", "SUSPENDED", "DEACTIVATED"} {
		if !validUserStatus(status) {
			t.Fatalf("%s must be valid", status)
		}
	}
	if validUserStatus("UNKNOWN") {
		t.Fatal("unknown status must be rejected")
	}
}

func TestUserDeletionEligibility(t *testing.T) {
	if !userDeletionEligible(roleMember, "DEACTIVATED") {
		t.Fatal("deactivated member must be eligible for permanent deletion")
	}
	if !userDeletionEligible(roleMember, "INVITED") {
		t.Fatal("invited member must be eligible for permanent deletion")
	}
	if userDeletionEligible(roleMember, "ACTIVE") || userDeletionEligible(roleMember, "SUSPENDED") {
		t.Fatal("active and suspended users must be deactivated before permanent deletion")
	}
	if userDeletionEligible(roleSuperAdmin, "DEACTIVATED") {
		t.Fatal("super administrators must be protected from permanent deletion")
	}
}
