package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeGroupInput(t *testing.T) {
	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"Engineering","description":"Product engineering"}`))
	writer := httptest.NewRecorder()
	name, description, ok := decodeGroupInput(writer, request)
	if !ok || name != "Engineering" || description != "Product engineering" {
		t.Fatalf("decodeGroupInput() = %q, %q, %v", name, description, ok)
	}
}

func TestDecodeGroupInputRejectsShortName(t *testing.T) {
	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"X","description":""}`))
	writer := httptest.NewRecorder()
	if _, _, ok := decodeGroupInput(writer, request); ok {
		t.Fatal("short group name must be rejected")
	}
}
