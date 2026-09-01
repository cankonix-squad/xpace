package httpapi

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestOIDCConfigurationValidation(t *testing.T) {
	valid := oidcConfiguration{IssuerURL: "https://identity.example.com", AuthorizationEndpoint: "https://identity.example.com/authorize", TokenEndpoint: "https://identity.example.com/token", UserinfoEndpoint: "https://identity.example.com/userinfo", ClientID: "xpace", DefaultRole: "MEMBER"}
	if message := valid.validate(); message != "" {
		t.Fatalf("valid configuration rejected: %s", message)
	}
	invalid := valid
	invalid.TokenEndpoint = "http://identity.example.com/token"
	if message := invalid.validate(); !strings.Contains(message, "HTTPS") {
		t.Fatalf("insecure endpoint should be rejected: %s", message)
	}
	invalid = valid
	invalid.DefaultRole = "SUPER_ADMIN"
	if message := invalid.validate(); !strings.Contains(message, "MEMBER or GUEST") {
		t.Fatalf("privileged automatic role should be rejected: %s", message)
	}
}

func TestOIDCUsernameIsSafeAndUniqueShape(t *testing.T) {
	username := oidcUsername("First+Last@Example.com")
	if !strings.HasPrefix(username, "first-last-") || strings.ContainsAny(username, "+@") {
		t.Fatalf("unsafe OIDC username: %s", username)
	}
}

func TestOIDCEndpointBlocksServerSideRequestForgery(t *testing.T) {
	t.Setenv("XPACE_PUBLIC_URL", "https://xspace.example.com")
	for _, endpoint := range []string{
		"http://identity.example.com/token",
		"https://localhost/token",
		"https://127.0.0.1/token",
		"https://10.0.0.10/token",
		"https://169.254.169.254/latest/meta-data",
		"https://identity.internal/token",
		"https://user:password@identity.example.com/token",
		"https://identity.example.com/token#secret",
	} {
		if err := validateOIDCEndpoint(endpoint, false); err == nil {
			t.Fatalf("unsafe endpoint accepted: %s", endpoint)
		}
	}
	if err := validateOIDCEndpoint("https://identity.example.com/token", false); err != nil {
		t.Fatalf("public HTTPS endpoint rejected: %v", err)
	}
}

func TestOIDCLocalHTTPRequiresLocalDevelopment(t *testing.T) {
	t.Setenv("XPACE_PUBLIC_URL", "http://localhost:3300")
	if !oidcLocalDevelopmentAllowed() {
		t.Fatal("local development should allow localhost OIDC")
	}
	if err := validateOIDCEndpoint("http://localhost:8080/token", true); err != nil {
		t.Fatalf("local endpoint rejected in development: %v", err)
	}
	t.Setenv("XPACE_PUBLIC_URL", "https://xspace.example.com")
	if oidcLocalDevelopmentAllowed() {
		t.Fatal("production URL must not allow localhost OIDC")
	}
}

func TestOIDCRedirectValidationRejectsCrossHostSecrets(t *testing.T) {
	client := oidcHTTPClient(false)
	previousURL, _ := url.Parse("https://identity.example.com/token")
	nextURL, _ := url.Parse("https://accounts.example.net/token")
	previous := &http.Request{URL: previousURL}
	next := &http.Request{URL: nextURL, Header: http.Header{"Authorization": {"Bearer secret"}}}
	if err := client.CheckRedirect(next, []*http.Request{previous}); err == nil {
		t.Fatal("cross-host redirect must not receive bearer tokens or client-secret request bodies")
	}
	privateURL, _ := url.Parse("https://127.0.0.1/token")
	if err := client.CheckRedirect(&http.Request{URL: privateURL, Header: make(http.Header)}, []*http.Request{previous}); err == nil {
		t.Fatal("redirect to private network should be rejected")
	}
}
