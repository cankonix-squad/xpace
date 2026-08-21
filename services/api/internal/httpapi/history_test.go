package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestHistoryPage(t *testing.T) {
	tests := []struct {
		query       string
		limit       int
		offset      int
		shouldError bool
	}{
		{"", 25, 0, false},
		{"?limit=50&offset=10", 50, 10, false},
		{"?limit=0", 0, 0, true},
		{"?limit=101", 0, 0, true},
		{"?offset=-1", 0, 0, true},
		{"?limit=invalid", 0, 0, true},
	}
	for _, test := range tests {
		request := httptest.NewRequest("GET", "/api/v1/meetings/history"+test.query, nil)
		limit, offset, err := historyPage(request)
		if (err != nil) != test.shouldError {
			t.Fatalf("historyPage(%q) error = %v", test.query, err)
		}
		if err == nil && (limit != test.limit || offset != test.offset) {
			t.Fatalf("historyPage(%q) = (%d,%d), want (%d,%d)", test.query, limit, offset, test.limit, test.offset)
		}
	}
}
