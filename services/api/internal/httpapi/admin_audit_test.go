package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestAuditFilters(t *testing.T) {
	request := httptest.NewRequest("GET", "/?limit=25&offset=5&action=user.&resource=user&actorId=abc", nil)
	limit, offset, action, resource, actorID, err := auditFilters(request)
	if err != nil || limit != 25 || offset != 5 || action != "user." || resource != "user" || actorID != "abc" {
		t.Fatalf("unexpected filters: %d %d %q %q %q %v", limit, offset, action, resource, actorID, err)
	}
}

func TestAuditFiltersRejectInvalidLimit(t *testing.T) {
	request := httptest.NewRequest("GET", "/?limit=500", nil)
	if _, _, _, _, _, err := auditFilters(request); err == nil {
		t.Fatal("oversized limit must be rejected")
	}
}
