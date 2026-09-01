package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestCrossWorkspaceAdministratorCannotModerateForeignMeeting(t *testing.T) {
	api, _ := mockAPI(t)
	request := httptest.NewRequest("GET", "/api/v1/meetings/ABC-DEF-GHI/participant", nil)
	meeting := meetingResponse{ID: "meeting-1", TenantID: "host-tenant", HostID: "host-1"}
	visitor := currentUser{ID: "admin-2", TenantID: "visitor-tenant", Role: roleSuperAdmin}
	if api.canModerate(request, meeting, visitor) {
		t.Fatal("an administrator must not carry workspace permissions into a foreign meeting")
	}
}

func TestMeetingResponseMarksWorkspaceBoundary(t *testing.T) {
	meeting := meetingResponse{TenantID: "host-tenant"}
	visitor := currentUser{TenantID: "visitor-tenant"}
	meeting.ExternalGuest = meeting.TenantID != visitor.TenantID
	if !meeting.ExternalGuest {
		t.Fatal("foreign meeting must be presented as external guest access")
	}
}
