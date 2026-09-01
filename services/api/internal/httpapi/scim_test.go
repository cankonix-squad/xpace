package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeSCIMUser(t *testing.T) {
	input := scimUserInput{UserName: " Ciko ", DisplayName: "Ciko Member"}
	input.Emails = make([]scimEmail, 1)
	input.Emails[0].Value, input.Emails[0].Type, input.Emails[0].Primary = "CIKO@EXAMPLE.COM", "work", true
	email, display, active, message := normalizeSCIMUser(input)
	if message != "" || email != "ciko@example.com" || display != "Ciko Member" || !active {
		t.Fatalf("unexpected normalized user: %q %q %v %q", email, display, active, message)
	}
}

func TestSCIMPaginationIsBounded(t *testing.T) {
	request := httptest.NewRequest("GET", "/?startIndex=-2&count=9999", nil)
	start, count := scimPagination(request)
	if start != 1 || count != 100 {
		t.Fatalf("unexpected pagination: %d %d", start, count)
	}
}

func TestSCIMBaseURLUsesPublicURL(t *testing.T) {
	t.Setenv("XPACE_PUBLIC_URL", "https://xpace.example.com/")
	if got := scimBaseURL("acme"); got != "https://xpace.example.com/api/v1/scim/v2/acme" {
		t.Fatal(got)
	}
}

func TestDecodeSCIMRejectsRegularText(t *testing.T) {
	request := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "text/plain")
	writer := httptest.NewRecorder()
	if decodeSCIM(writer, request, &scimUserInput{}) || writer.Code != 415 {
		t.Fatalf("unexpected response %d", writer.Code)
	}
}

func TestSCIMPatchMemberParsing(t *testing.T) {
	members := scimPatchMembers([]byte(`{"members":[{"value":"user-1"},{"value":"user-2"}]}`))
	if len(members) != 2 || members[0] != "user-1" || members[1] != "user-2" {
		t.Fatalf("unexpected members: %v", members)
	}
	if value := memberFromFilter(`members[value eq "user-3"]`); value != "user-3" {
		t.Fatalf("unexpected filter member: %q", value)
	}
}
