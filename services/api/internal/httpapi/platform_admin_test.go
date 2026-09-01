package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestPlatformPagination(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/v1/platform/tenants?page=2&pageSize=250", nil)
	page, pageSize := platformPagination(request)
	if page != 2 || pageSize != 100 {
		t.Fatalf("unexpected pagination: page=%d pageSize=%d", page, pageSize)
	}
	request = httptest.NewRequest("GET", "/api/v1/platform/tenants?page=-1&pageSize=0", nil)
	page, pageSize = platformPagination(request)
	if page != 1 || pageSize != 20 {
		t.Fatalf("invalid pagination was not normalized: page=%d pageSize=%d", page, pageSize)
	}
}

func TestRequirePlatformAdmin(t *testing.T) {
	writer := httptest.NewRecorder()
	if requirePlatformAdmin(writer, currentUser{Role: roleTenantAdmin}) || writer.Code != 403 {
		t.Fatal("tenant administrators must not receive platform access")
	}
	writer = httptest.NewRecorder()
	if !requirePlatformAdmin(writer, currentUser{Role: roleSuperAdmin}) {
		t.Fatal("super administrators must receive platform access")
	}
}
