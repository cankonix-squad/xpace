package httpapi

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBaseRolePermissionDoesNotQueryDatabase(t *testing.T) {
	api, mock := mockAPI(t)
	actor := currentUser{ID: "admin", TenantID: "tenant", Role: roleTenantAdmin}
	if !api.hasPermission(context.Background(), actor, "users.manage") {
		t.Fatal("tenant admin should inherit users.manage")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCustomRolePermissionIsResolvedWithinTenant(t *testing.T) {
	api, mock := mockAPI(t)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("member", "tenant", "audit.read").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	actor := currentUser{ID: "member", TenantID: "tenant", Role: roleMember}
	if !api.hasPermission(context.Background(), actor, "audit.read") {
		t.Fatal("assigned custom permission should be granted")
	}
}

func TestPermissionCatalogRejectsPrivilegeEscalation(t *testing.T) {
	if !validPermissionSet([]string{"users.manage", "audit.read"}) {
		t.Fatal("catalog permissions should be valid")
	}
	if validPermissionSet([]string{"platform.manage"}) || validPermissionSet([]string{"roles.manage"}) {
		t.Fatal("custom roles must not grant platform or role administration")
	}
}
