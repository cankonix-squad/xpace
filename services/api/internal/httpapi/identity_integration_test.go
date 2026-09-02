//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	passwordauth "github.com/cankonix/xpace/api/internal/auth"
	"github.com/cankonix/xpace/api/internal/httpapi"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestIntegrationOIDCSCIMAcceptance(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err = database.PingContext(ctx); err != nil {
		t.Fatalf("postgres unavailable: %v", err)
	}

	provider := newLocalOIDCProvider(t)
	defer provider.Close()
	apiServer := httptest.NewServer(httpapi.NewRouter(database, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	defer apiServer.Close()
	publicAPIURL := strings.Replace(apiServer.URL, "127.0.0.1", "localhost", 1)
	t.Setenv("XPACE_PUBLIC_URL", publicAPIURL)
	t.Setenv("COOKIE_SECURE", "false")

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantSlug := "identity-" + suffix
	foreignSlug := "identity-foreign-" + suffix
	tenantID, adminID := createIdentityTenant(t, ctx, database, tenantSlug, "Identity Acceptance", suffix)
	foreignTenantID, _ := createIdentityTenant(t, ctx, database, foreignSlug, "Foreign Identity", "foreign-"+suffix)
	t.Cleanup(func() { cleanupTenant(t, database, foreignTenantID) })
	t.Cleanup(func() { cleanupTenant(t, database, tenantID) })

	admin := newAPIClient(t)
	doJSON(t, admin, http.MethodPost, apiServer.URL+"/api/v1/auth/login", map[string]any{"tenant": tenantSlug, "identity": "admin", "password": "Identity!Pass#2026"}, http.StatusOK, nil)

	var scimConfiguration struct {
		Token   string `json:"token"`
		BaseURL string `json:"baseUrl"`
	}
	doJSON(t, admin, http.MethodPost, apiServer.URL+"/api/v1/admin/identity/scim", nil, http.StatusCreated, &scimConfiguration)
	if scimConfiguration.Token == "" || !strings.HasSuffix(scimConfiguration.BaseURL, "/"+tenantSlug) {
		t.Fatalf("invalid SCIM configuration: %+v", scimConfiguration)
	}

	verifiedEmail := "scim-oidc-" + suffix + "@test.invalid"
	var scimUser map[string]any
	doSCIM(t, http.MethodPost, scimConfiguration.BaseURL+"/Users", scimConfiguration.Token, map[string]any{
		"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"externalId":  "external-user-" + suffix,
		"userName":    "scim-user-" + suffix,
		"displayName": "SCIM OIDC User",
		"active":      true,
		"emails":      []map[string]any{{"value": verifiedEmail, "primary": true, "type": "work"}},
	}, http.StatusCreated, &scimUser)
	userID, _ := scimUser["id"].(string)
	if userID == "" {
		t.Fatal("SCIM user ID was not returned")
	}

	var scimGroup map[string]any
	doSCIM(t, http.MethodPost, scimConfiguration.BaseURL+"/Groups", scimConfiguration.Token, map[string]any{
		"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
		"externalId":  "external-group-" + suffix,
		"displayName": "Identity Acceptance Group " + suffix,
		"members":     []map[string]any{{"value": userID}},
	}, http.StatusCreated, &scimGroup)
	if scimGroup["id"] == "" {
		t.Fatal("SCIM group ID was not returned")
	}
	doSCIM(t, http.MethodGet, apiServer.URL+"/api/v1/scim/v2/"+foreignSlug+"/Users", scimConfiguration.Token, nil, http.StatusUnauthorized, nil)

	oidcInput := map[string]any{
		"issuerUrl":             provider.URL,
		"authorizationEndpoint": provider.URL + "/authorize",
		"tokenEndpoint":         provider.URL + "/token",
		"userinfoEndpoint":      provider.URL + "/userinfo",
		"clientId":              localOIDCClientID,
		"clientSecret":          localOIDCClientSecret,
		"enabled":               true,
		"autoProvision":         false,
		"defaultRole":           "GUEST",
	}
	doJSON(t, admin, http.MethodPut, apiServer.URL+"/api/v1/admin/identity/oidc", oidcInput, http.StatusOK, nil)

	provider.SetIdentity(localOIDCIdentity{Subject: "linked-subject", Email: verifiedEmail, Name: "Linked User", EmailVerified: true})
	linkedClient, callbackURL, callbackLocation := performOIDCLogin(t, publicAPIURL, tenantSlug)
	if callbackLocation != "/" {
		t.Fatalf("OIDC login failed: %s", callbackLocation)
	}
	var me struct {
		User struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"user"`
	}
	doJSON(t, linkedClient, http.MethodGet, publicAPIURL+"/api/v1/auth/me", nil, http.StatusOK, &me)
	if me.User.ID != userID || me.User.Role != "MEMBER" {
		t.Fatalf("OIDC linked wrong SCIM account or changed its role: %+v", me.User)
	}
	assertRedirectLocation(t, linkedClient, callbackURL, "/login?error=sso_state")

	provider.SetIdentity(localOIDCIdentity{Subject: "unverified-subject", Email: "unverified-" + suffix + "@test.invalid", Name: "Unverified User", EmailVerified: false})
	_, _, callbackLocation = performOIDCLogin(t, publicAPIURL, tenantSlug)
	if callbackLocation != "/login?error=sso_identity" {
		t.Fatalf("unverified identity was not rejected: %s", callbackLocation)
	}

	provider.SetIdentity(localOIDCIdentity{Subject: "bad-nonce-subject", Email: "nonce-" + suffix + "@test.invalid", Name: "Bad Nonce", EmailVerified: true, BadNonce: true})
	_, _, callbackLocation = performOIDCLogin(t, publicAPIURL, tenantSlug)
	if callbackLocation != "/login?error=sso_token" {
		t.Fatalf("invalid ID-token nonce was not rejected: %s", callbackLocation)
	}
	provider.SetIdentity(localOIDCIdentity{Subject: "bad-signature-subject", Email: "signature-" + suffix + "@test.invalid", Name: "Bad Signature", EmailVerified: true, BadSignature: true})
	_, _, callbackLocation = performOIDCLogin(t, publicAPIURL, tenantSlug)
	if callbackLocation != "/login?error=sso_token" {
		t.Fatalf("invalid ID-token signature was not rejected: %s", callbackLocation)
	}
	provider.SetIdentity(localOIDCIdentity{Subject: "bad-hash-subject", Email: "hash-" + suffix + "@test.invalid", Name: "Bad Access Hash", EmailVerified: true, BadAccessTokenHash: true})
	_, _, callbackLocation = performOIDCLogin(t, publicAPIURL, tenantSlug)
	if callbackLocation != "/login?error=sso_token" {
		t.Fatalf("invalid ID-token access-token hash was not rejected: %s", callbackLocation)
	}

	oidcInput["clientSecret"] = ""
	oidcInput["autoProvision"] = true
	doJSON(t, admin, http.MethodPut, apiServer.URL+"/api/v1/admin/identity/oidc", oidcInput, http.StatusOK, nil)
	provider.SetIdentity(localOIDCIdentity{Subject: "guest-subject", Email: "guest-" + suffix + "@test.invalid", Name: "OIDC Guest", EmailVerified: true})
	guestClient, _, callbackLocation := performOIDCLogin(t, publicAPIURL, tenantSlug)
	if callbackLocation != "/" {
		t.Fatalf("OIDC JIT provisioning failed: %s", callbackLocation)
	}
	doJSON(t, guestClient, http.MethodGet, publicAPIURL+"/api/v1/auth/me", nil, http.StatusOK, &me)
	if me.User.Role != "GUEST" || me.User.ID == userID {
		t.Fatalf("OIDC default role mapping failed: %+v", me.User)
	}

	doSCIM(t, http.MethodPatch, scimConfiguration.BaseURL+"/Users/"+userID, scimConfiguration.Token, map[string]any{
		"schemas":    []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []map[string]any{{"op": "replace", "path": "active", "value": false}},
	}, http.StatusOK, nil)
	doJSON(t, linkedClient, http.MethodGet, publicAPIURL+"/api/v1/auth/me", nil, http.StatusUnauthorized, nil)

	var auditCount int
	err = database.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE tenant_id=$1 AND action IN ('identity.scim.token.rotate','scim.user.create','scim.group.create','identity.oidc.configuration.update','auth.login.oidc','scim.user.patch')`, tenantID).Scan(&auditCount)
	if err != nil || auditCount < 7 {
		t.Fatalf("identity audit coverage = %d, error = %v", auditCount, err)
	}
	var foreignIdentityCount int
	if err = database.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_oidc_identities WHERE tenant_id=$1`, foreignTenantID).Scan(&foreignIdentityCount); err != nil || foreignIdentityCount != 0 {
		t.Fatalf("foreign tenant identity count = %d, error = %v", foreignIdentityCount, err)
	}
	_ = adminID
}

const localOIDCClientID = "xpace-local-acceptance"
const localOIDCClientSecret = "local-provider-secret"

type localOIDCIdentity struct {
	Subject, Email, Name string
	EmailVerified        bool
	BadNonce             bool
	BadSignature         bool
	BadAccessTokenHash   bool
}

type localOIDCProvider struct {
	*httptest.Server
	key      *rsa.PrivateKey
	badKey   *rsa.PrivateKey
	mu       sync.Mutex
	identity localOIDCIdentity
	codes    map[string]localOIDCCode
}

type localOIDCCode struct{ Challenge, Nonce string }

func newLocalOIDCProvider(t *testing.T) *localOIDCProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	badKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := &localOIDCProvider{key: key, badKey: badKey, codes: map[string]localOIDCCode{}}
	provider.Server = httptest.NewServer(http.HandlerFunc(provider.ServeHTTP))
	provider.URL = strings.Replace(provider.URL, "127.0.0.1", "localhost", 1)
	return provider
}

func (provider *localOIDCProvider) SetIdentity(identity localOIDCIdentity) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.identity = identity
}

func (provider *localOIDCProvider) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/.well-known/openid-configuration":
		writeTestJSON(writer, map[string]any{"issuer": provider.URL, "authorization_endpoint": provider.URL + "/authorize", "token_endpoint": provider.URL + "/token", "userinfo_endpoint": provider.URL + "/userinfo", "jwks_uri": provider.URL + "/jwks"})
	case "/jwks":
		exponent := big.NewInt(int64(provider.key.PublicKey.E)).Bytes()
		writeTestJSON(writer, map[string]any{"keys": []map[string]any{{"kty": "RSA", "kid": "local-key", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(provider.key.PublicKey.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(exponent)}}})
	case "/authorize":
		query := request.URL.Query()
		if query.Get("client_id") != localOIDCClientID || query.Get("response_type") != "code" || query.Get("code_challenge_method") != "S256" || !strings.Contains(query.Get("scope"), "openid") || query.Get("state") == "" || query.Get("nonce") == "" || query.Get("code_challenge") == "" {
			http.Error(writer, "invalid authorization request", http.StatusBadRequest)
			return
		}
		code := fmt.Sprintf("code-%d", time.Now().UnixNano())
		provider.mu.Lock()
		provider.codes[code] = localOIDCCode{Challenge: query.Get("code_challenge"), Nonce: query.Get("nonce")}
		provider.mu.Unlock()
		callback, _ := url.Parse(query.Get("redirect_uri"))
		parameters := callback.Query()
		parameters.Set("state", query.Get("state"))
		parameters.Set("code", code)
		callback.RawQuery = parameters.Encode()
		http.Redirect(writer, request, callback.String(), http.StatusFound)
	case "/token":
		if request.Method != http.MethodPost || request.ParseForm() != nil || request.Form.Get("client_id") != localOIDCClientID || request.Form.Get("client_secret") != localOIDCClientSecret || request.Form.Get("grant_type") != "authorization_code" {
			http.Error(writer, "invalid token request", http.StatusBadRequest)
			return
		}
		provider.mu.Lock()
		code, exists := provider.codes[request.Form.Get("code")]
		delete(provider.codes, request.Form.Get("code"))
		identity := provider.identity
		provider.mu.Unlock()
		challenge := sha256.Sum256([]byte(request.Form.Get("code_verifier")))
		if !exists || base64.RawURLEncoding.EncodeToString(challenge[:]) != code.Challenge {
			http.Error(writer, "invalid authorization code", http.StatusBadRequest)
			return
		}
		nonce := code.Nonce
		if identity.BadNonce {
			nonce = "wrong-nonce"
		}
		accessToken := "access-" + identity.Subject
		now := time.Now()
		accessHash := accessTokenHash(accessToken)
		if identity.BadAccessTokenHash {
			accessHash = "invalid-access-token-hash"
		}
		claims := jwt.MapClaims{"iss": provider.URL, "sub": identity.Subject, "aud": localOIDCClientID, "exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix(), "nonce": nonce, "at_hash": accessHash}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = "local-key"
		signingKey := provider.key
		if identity.BadSignature {
			signingKey = provider.badKey
		}
		signed, err := token.SignedString(signingKey)
		if err != nil {
			http.Error(writer, "could not sign token", http.StatusInternalServerError)
			return
		}
		writeTestJSON(writer, map[string]any{"access_token": accessToken, "token_type": "Bearer", "expires_in": 300, "id_token": signed})
	case "/userinfo":
		provider.mu.Lock()
		identity := provider.identity
		provider.mu.Unlock()
		if request.Header.Get("Authorization") != "Bearer access-"+identity.Subject {
			http.Error(writer, "invalid access token", http.StatusUnauthorized)
			return
		}
		writeTestJSON(writer, map[string]any{"sub": identity.Subject, "email": identity.Email, "name": identity.Name, "email_verified": identity.EmailVerified})
	default:
		http.NotFound(writer, request)
	}
}

func createIdentityTenant(t *testing.T, ctx context.Context, database *sql.DB, slug, name, suffix string) (string, string) {
	t.Helper()
	hash, err := passwordauth.HashPassword("Identity!Pass#2026")
	if err != nil {
		t.Fatal(err)
	}
	var tenantID, adminID string
	if err = database.QueryRowContext(ctx, `INSERT INTO tenants(slug,name) VALUES($1,$2) RETURNING id`, slug, name).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO tenant_subscriptions(tenant_id,plan_key,status,current_period_started_at,current_period_ends_at) VALUES($1,'ENTERPRISE','ACTIVE',NOW(),NOW()+INTERVAL '1 day')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if err = database.QueryRowContext(ctx, `INSERT INTO users(tenant_id,email,username,display_name,password_hash,role,status) VALUES($1,$2,'admin','Identity Admin',$3,'TENANT_ADMIN','ACTIVE') RETURNING id`, tenantID, "admin-"+suffix+"@test.invalid", hash).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	return tenantID, adminID
}

func performOIDCLogin(t *testing.T, apiURL, tenantSlug string) (*http.Client, string, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	authorizationLocation := redirectLocation(t, client, apiURL+"/api/v1/auth/sso/oidc/start?tenant="+url.QueryEscape(tenantSlug))
	callbackURL := redirectLocation(t, client, authorizationLocation)
	callbackLocation := redirectLocation(t, client, callbackURL)
	return client, callbackURL, callbackLocation
}

func redirectLocation(t *testing.T, client *http.Client, endpoint string) string {
	t.Helper()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("GET %s returned %d", endpoint, response.StatusCode)
	}
	return response.Header.Get("Location")
}

func assertRedirectLocation(t *testing.T, client *http.Client, endpoint, expected string) {
	t.Helper()
	if actual := redirectLocation(t, client, endpoint); actual != expected {
		t.Fatalf("redirect = %s, want %s", actual, expected)
	}
}

func doSCIM(t *testing.T, method, endpoint, token string, input any, expectedStatus int, output any) {
	t.Helper()
	var body []byte
	var err error
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequest(method, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if input != nil {
		request.Header.Set("Content-Type", "application/scim+json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		var failure any
		_ = json.NewDecoder(response.Body).Decode(&failure)
		t.Fatalf("%s %s returned %d, want %d: %#v", method, endpoint, response.StatusCode, expectedStatus, failure)
	}
	if output != nil {
		if err = json.NewDecoder(response.Body).Decode(output); err != nil {
			t.Fatal(err)
		}
	}
}

func writeTestJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func accessTokenHash(accessToken string) string {
	sum := sha256.Sum256([]byte(accessToken))
	return base64.RawURLEncoding.EncodeToString(sum[:len(sum)/2])
}
