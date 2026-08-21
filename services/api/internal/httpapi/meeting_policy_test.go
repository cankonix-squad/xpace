package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMeetingPolicyJSONRequiresAllKnownFields(t *testing.T) {
	request := httptest.NewRequest("PUT", "/", strings.NewReader(`{"guestAccessEnabled":false,"waitingRoomDefault":true,"recordingEnabled":false,"screenShareEnabled":true}`))
	writer := httptest.NewRecorder()
	var policy meetingPolicy
	if err := decodeJSON(writer, request, &policy); err != nil {
		t.Fatal(err)
	}
	if policy.GuestAccessEnabled || policy.RecordingEnabled || !policy.WaitingRoomDefault || !policy.ScreenShareEnabled {
		t.Fatalf("unexpected policy: %+v", policy)
	}
}
