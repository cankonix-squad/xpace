package httpapi

import "testing"

func TestRoleCapabilities(t *testing.T) {
	tests := []struct {
		role      userRole
		admin     bool
		canCreate bool
	}{
		{roleSuperAdmin, true, true},
		{roleTenantAdmin, true, true},
		{roleHost, false, true},
		{roleCoHost, false, true},
		{roleMember, false, true},
		{roleGuest, false, false},
	}
	for _, test := range tests {
		t.Run(string(test.role), func(t *testing.T) {
			if got := test.role.isWorkspaceAdmin(); got != test.admin {
				t.Fatalf("isWorkspaceAdmin() = %v, want %v", got, test.admin)
			}
			if got := test.role.canCreateMeeting(); got != test.canCreate {
				t.Fatalf("canCreateMeeting() = %v, want %v", got, test.canCreate)
			}
			if len(test.role.permissions()) == 0 {
				t.Fatal("role must expose at least one permission")
			}
		})
	}
}

func TestPermissionsReturnsCopy(t *testing.T) {
	permissions := roleMember.permissions()
	permissions[0] = "changed"
	if roleMember.permissions()[0] == "changed" {
		t.Fatal("permissions must not expose mutable shared role state")
	}
}
