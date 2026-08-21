package httpapi

type userRole string

const (
	roleSuperAdmin  userRole = "SUPER_ADMIN"
	roleTenantAdmin userRole = "TENANT_ADMIN"
	roleHost        userRole = "HOST"
	roleCoHost      userRole = "CO_HOST"
	roleMember      userRole = "MEMBER"
	roleGuest       userRole = "GUEST"
)

var rolePermissions = map[userRole][]string{
	roleSuperAdmin:  {"platform.manage", "tenant.manage", "users.manage", "meeting.create", "meeting.moderate", "meeting.join"},
	roleTenantAdmin: {"tenant.manage", "users.manage", "meeting.create", "meeting.moderate", "meeting.join"},
	roleHost:        {"meeting.create", "meeting.moderate.owned", "meeting.join"},
	roleCoHost:      {"meeting.create", "meeting.moderate.assigned", "meeting.join"},
	roleMember:      {"meeting.create", "meeting.join"},
	roleGuest:       {"meeting.join"},
}

func (role userRole) permissions() []string {
	return append([]string(nil), rolePermissions[role]...)
}

func (role userRole) canCreateMeeting() bool {
	return role != roleGuest
}

func (role userRole) isWorkspaceAdmin() bool {
	return role == roleSuperAdmin || role == roleTenantAdmin
}
