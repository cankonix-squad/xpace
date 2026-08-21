package httpapi

import "testing"

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
