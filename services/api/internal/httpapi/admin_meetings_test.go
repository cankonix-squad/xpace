package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestAdminMeetingFilters(t *testing.T) {
	request := httptest.NewRequest("GET", "/?limit=40&offset=20&status=ended&search=roadmap", nil)
	limit, offset, status, search, err := adminMeetingFilters(request)
	if err != nil || limit != 40 || offset != 20 || status != "ENDED" || search != "roadmap" {
		t.Fatalf("unexpected filters: %d %d %q %q %v", limit, offset, status, search, err)
	}
}

func TestAdminMeetingFiltersRejectInvalidStatus(t *testing.T) {
	request := httptest.NewRequest("GET", "/?status=unknown", nil)
	if _, _, _, _, err := adminMeetingFilters(request); err == nil {
		t.Fatal("unknown meeting status must be rejected")
	}
}
