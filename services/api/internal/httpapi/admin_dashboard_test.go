package httpapi

import "testing"

func TestAdminDashboardRoles(t *testing.T) {
	for _, role := range []userRole{roleSuperAdmin, roleTenantAdmin} {
		if !role.isWorkspaceAdmin() {
			t.Fatalf("%s must have admin dashboard access", role)
		}
	}
	for _, role := range []userRole{roleHost, roleCoHost, roleMember, roleGuest} {
		if role.isWorkspaceAdmin() {
			t.Fatalf("%s must not have admin dashboard access", role)
		}
	}
}
