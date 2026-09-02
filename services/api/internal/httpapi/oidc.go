package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	passwordauth "github.com/cankonix/xpace/api/internal/auth"
)

type oidcConfiguration struct {
	IssuerURL              string `json:"issuerUrl"`
	AuthorizationEndpoint  string `json:"authorizationEndpoint"`
	TokenEndpoint          string `json:"tokenEndpoint"`
	UserinfoEndpoint       string `json:"userinfoEndpoint"`
	ClientID               string `json:"clientId"`
	ClientSecret           string `json:"clientSecret,omitempty"`
	ClientSecretConfigured bool   `json:"clientSecretConfigured"`
	Enabled                bool   `json:"enabled"`
	AutoProvision          bool   `json:"autoProvision"`
	DefaultRole            string `json:"defaultRole"`
}

func (api *API) adminOIDCConfiguration(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "identity.manage") {
		errorJSON(writer, 403, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	if request.Method == http.MethodGet {
		configuration, err := api.loadOIDCConfiguration(request.Context(), actor.TenantID)
		if err != nil {
			respondJSON(writer, 200, map[string]any{"configuration": oidcConfiguration{DefaultRole: "MEMBER"}})
			return
		}
		configuration.ClientSecret = ""
		respondJSON(writer, 200, map[string]any{"configuration": configuration})
		return
	}
	var input oidcConfiguration
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	input.normalize()
	if message := input.validate(); message != "" {
		errorJSON(writer, 400, "INVALID_OIDC_CONFIGURATION", message)
		return
	}
	secretEncrypted := ""
	if input.ClientSecret != "" {
		var err error
		secretEncrypted, err = encryptMFASecret(input.ClientSecret)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not protect OIDC client secret")
			return
		}
	} else {
		_ = api.database.QueryRowContext(request.Context(), `SELECT client_secret_encrypted FROM tenant_oidc_configurations WHERE tenant_id=$1`, actor.TenantID).Scan(&secretEncrypted)
	}
	if secretEncrypted == "" {
		errorJSON(writer, 400, "OIDC_CLIENT_SECRET_REQUIRED", "clientSecret is required for initial setup")
		return
	}
	_, err := api.database.ExecContext(request.Context(), `INSERT INTO tenant_oidc_configurations(tenant_id,issuer_url,authorization_endpoint,token_endpoint,userinfo_endpoint,client_id,client_secret_encrypted,enabled,auto_provision,default_role,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(tenant_id) DO UPDATE SET issuer_url=EXCLUDED.issuer_url,authorization_endpoint=EXCLUDED.authorization_endpoint,token_endpoint=EXCLUDED.token_endpoint,userinfo_endpoint=EXCLUDED.userinfo_endpoint,client_id=EXCLUDED.client_id,client_secret_encrypted=EXCLUDED.client_secret_encrypted,enabled=EXCLUDED.enabled,auto_provision=EXCLUDED.auto_provision,default_role=EXCLUDED.default_role,updated_by=EXCLUDED.updated_by,updated_at=NOW()`, actor.TenantID, input.IssuerURL, input.AuthorizationEndpoint, input.TokenEndpoint, input.UserinfoEndpoint, input.ClientID, secretEncrypted, input.Enabled, input.AutoProvision, input.DefaultRole, actor.ID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not save OIDC configuration")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "identity.oidc.configuration.update", "tenant", actor.TenantID, map[string]any{"issuer": input.IssuerURL, "enabled": input.Enabled, "autoProvision": input.AutoProvision})
	input.ClientSecret = ""
	input.ClientSecretConfigured = true
	respondJSON(writer, 200, map[string]any{"configuration": input})
}

func (api *API) oidcStart(writer http.ResponseWriter, request *http.Request) {
	tenantSlug := strings.ToLower(strings.TrimSpace(request.URL.Query().Get("tenant")))
	var tenantID string
	if err := api.database.QueryRowContext(request.Context(), `SELECT id FROM tenants WHERE slug=$1`, tenantSlug).Scan(&tenantID); err != nil {
		errorJSON(writer, 404, "SSO_NOT_AVAILABLE", "SSO is not configured for this workspace")
		return
	}
	configuration, err := api.loadOIDCConfiguration(request.Context(), tenantID)
	if err != nil || !configuration.Enabled {
		errorJSON(writer, 404, "SSO_NOT_AVAILABLE", "SSO is not configured for this workspace")
		return
	}
	state, _ := randomToken(32)
	nonce, _ := randomToken(32)
	verifier, _ := randomToken(48)
	encryptedVerifier, err := encryptMFASecret(verifier)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not start SSO")
		return
	}
	_, _ = api.database.ExecContext(request.Context(), `DELETE FROM oidc_login_states WHERE expires_at<NOW()`)
	_, err = api.database.ExecContext(request.Context(), `INSERT INTO oidc_login_states(state_hash,tenant_id,nonce_hash,verifier_encrypted,expires_at) VALUES($1,$2,$3,$4,NOW()+INTERVAL '10 minutes')`, hashToken(state), tenantID, hashToken(nonce), encryptedVerifier)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not start SSO")
		return
	}
	challengeSum := sha256.Sum256([]byte(verifier))
	parameters := url.Values{"client_id": {configuration.ClientID}, "redirect_uri": {oidcRedirectURI()}, "response_type": {"code"}, "scope": {"openid email profile"}, "state": {state}, "nonce": {nonce}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challengeSum[:])}, "code_challenge_method": {"S256"}}
	http.Redirect(writer, request, configuration.AuthorizationEndpoint+"?"+parameters.Encode(), http.StatusFound)
}

func (api *API) oidcCallback(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Query().Get("error") != "" {
		http.Redirect(writer, request, "/login?error=sso_denied", http.StatusFound)
		return
	}
	state, code := request.URL.Query().Get("state"), request.URL.Query().Get("code")
	var tenantID, encryptedVerifier, nonceHash string
	err := api.database.QueryRowContext(request.Context(), `DELETE FROM oidc_login_states WHERE state_hash=$1 AND expires_at>NOW() RETURNING tenant_id,verifier_encrypted,nonce_hash`, hashToken(state)).Scan(&tenantID, &encryptedVerifier, &nonceHash)
	if err != nil || code == "" {
		http.Redirect(writer, request, "/login?error=sso_state", http.StatusFound)
		return
	}
	configuration, err := api.loadOIDCConfiguration(request.Context(), tenantID)
	if err != nil || !configuration.Enabled {
		http.Redirect(writer, request, "/login?error=sso_configuration", http.StatusFound)
		return
	}
	verifier, err := decryptMFASecret(encryptedVerifier)
	if err != nil {
		http.Redirect(writer, request, "/login?error=sso_state", http.StatusFound)
		return
	}
	secret, err := decryptMFASecret(configuration.ClientSecret)
	if err != nil {
		http.Redirect(writer, request, "/login?error=sso_configuration", http.StatusFound)
		return
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {oidcRedirectURI()}, "client_id": {configuration.ClientID}, "client_secret": {secret}, "code_verifier": {verifier}}
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	}
	if err = oidcJSONRequest(request.Context(), http.MethodPost, configuration.TokenEndpoint, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", "", &tokenResponse); err != nil || tokenResponse.AccessToken == "" || tokenResponse.IDToken == "" {
		slog.Warn("OIDC token exchange failed", "error", err, "issuer", configuration.IssuerURL)
		http.Redirect(writer, request, "/login?error=sso_token", http.StatusFound)
		return
	}
	idTokenSubject, err := validateOIDCIDToken(request.Context(), configuration, tokenResponse.IDToken, tokenResponse.AccessToken, nonceHash)
	if err != nil {
		slog.Warn("OIDC ID token validation failed", "error", err, "issuer", configuration.IssuerURL)
		http.Redirect(writer, request, "/login?error=sso_token", http.StatusFound)
		return
	}
	var identity struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		Name          string `json:"name"`
		EmailVerified *bool  `json:"email_verified"`
	}
	if err = oidcJSONRequest(request.Context(), http.MethodGet, configuration.UserinfoEndpoint, nil, "", tokenResponse.AccessToken, &identity); err != nil || identity.Subject == "" || identity.Subject != idTokenSubject || identity.Email == "" || (identity.EmailVerified != nil && !*identity.EmailVerified) {
		http.Redirect(writer, request, "/login?error=sso_identity", http.StatusFound)
		return
	}
	emailVerified := identity.EmailVerified != nil && *identity.EmailVerified
	user, err := api.resolveOIDCUser(request.Context(), tenantID, configuration, identity.Subject, identity.Email, identity.Name, emailVerified)
	if err != nil {
		http.Redirect(writer, request, "/login?error=sso_user", http.StatusFound)
		return
	}
	if err = api.createSession(request.Context(), writer, request, user); err != nil {
		http.Redirect(writer, request, "/login?error=sso_session", http.StatusFound)
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, "auth.login.oidc", "session", user.ID, map[string]any{"issuer": configuration.IssuerURL})
	http.Redirect(writer, request, "/", http.StatusFound)
}

func (api *API) loadOIDCConfiguration(ctx context.Context, tenantID string) (oidcConfiguration, error) {
	var item oidcConfiguration
	err := api.database.QueryRowContext(ctx, `SELECT issuer_url,authorization_endpoint,token_endpoint,userinfo_endpoint,client_id,client_secret_encrypted,enabled,auto_provision,default_role FROM tenant_oidc_configurations WHERE tenant_id=$1`, tenantID).Scan(&item.IssuerURL, &item.AuthorizationEndpoint, &item.TokenEndpoint, &item.UserinfoEndpoint, &item.ClientID, &item.ClientSecret, &item.Enabled, &item.AutoProvision, &item.DefaultRole)
	item.ClientSecretConfigured = item.ClientSecret != ""
	return item, err
}

func (api *API) resolveOIDCUser(ctx context.Context, tenantID string, configuration oidcConfiguration, subject, email, name string, emailVerified bool) (currentUser, error) {
	var user currentUser
	err := api.database.QueryRowContext(ctx, `SELECT u.id,u.tenant_id,t.slug,t.name,u.email,u.username,u.display_name,u.role FROM user_oidc_identities i JOIN users u ON u.id=i.user_id AND u.tenant_id=i.tenant_id JOIN tenants t ON t.id=u.tenant_id WHERE i.tenant_id=$1 AND i.issuer_url=$2 AND i.subject=$3 AND u.status='ACTIVE'`, tenantID, configuration.IssuerURL, subject).Scan(&user.ID, &user.TenantID, &user.TenantSlug, &user.TenantName, &user.Email, &user.Username, &user.DisplayName, &user.Role)
	if err == nil {
		return user, nil
	}
	// Linking or creating an account by email is only safe when the provider
	// explicitly attests that it verified the address. Existing subject links do
	// not depend on a mutable email claim and are handled above.
	if !emailVerified {
		return user, fmt.Errorf("OIDC email is not verified")
	}
	err = api.database.QueryRowContext(ctx, `SELECT u.id,u.tenant_id,t.slug,t.name,u.email,u.username,u.display_name,u.role FROM users u JOIN tenants t ON t.id=u.tenant_id WHERE u.tenant_id=$1 AND LOWER(u.email)=LOWER($2) AND u.status='ACTIVE'`, tenantID, strings.TrimSpace(email)).Scan(&user.ID, &user.TenantID, &user.TenantSlug, &user.TenantName, &user.Email, &user.Username, &user.DisplayName, &user.Role)
	if err != nil && !configuration.AutoProvision {
		return user, fmt.Errorf("OIDC account is not provisioned")
	}
	if err != nil {
		if quotaErr := api.enforceTenantQuota(ctx, tenantID, "users", 1); quotaErr != nil {
			return user, quotaErr
		}
		username := oidcUsername(email)
		password, _ := randomToken(32)
		hash, hashErr := passwordauth.HashPassword(password)
		if hashErr != nil {
			return user, hashErr
		}
		if strings.TrimSpace(name) == "" {
			name = strings.Split(email, "@")[0]
		}
		err = api.database.QueryRowContext(ctx, `INSERT INTO users(tenant_id,email,username,display_name,password_hash,role,status) VALUES($1,LOWER($2),$3,$4,$5,$6,'ACTIVE') RETURNING id`, tenantID, email, username, strings.TrimSpace(name), hash, configuration.DefaultRole).Scan(&user.ID)
		if err != nil {
			return user, err
		}
		err = api.database.QueryRowContext(ctx, `SELECT u.id,u.tenant_id,t.slug,t.name,u.email,u.username,u.display_name,u.role FROM users u JOIN tenants t ON t.id=u.tenant_id WHERE u.id=$1`, user.ID).Scan(&user.ID, &user.TenantID, &user.TenantSlug, &user.TenantName, &user.Email, &user.Username, &user.DisplayName, &user.Role)
	}
	if err == nil {
		_, err = api.database.ExecContext(ctx, `INSERT INTO user_oidc_identities(user_id,tenant_id,issuer_url,subject) VALUES($1,$2,$3,$4) ON CONFLICT(tenant_id,issuer_url,subject) DO NOTHING`, user.ID, tenantID, configuration.IssuerURL, subject)
	}
	return user, err
}

func oidcJSONRequest(ctx context.Context, method, endpoint string, body io.Reader, contentType, bearer string, output any) error {
	allowLocal := oidcLocalDevelopmentAllowed()
	if err := validateOIDCEndpoint(endpoint, allowLocal); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := oidcHTTPClient(allowLocal).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("OIDC endpoint returned %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output)
}

func (configuration *oidcConfiguration) normalize() {
	configuration.IssuerURL = strings.TrimRight(strings.TrimSpace(configuration.IssuerURL), "/")
	configuration.AuthorizationEndpoint = strings.TrimSpace(configuration.AuthorizationEndpoint)
	configuration.TokenEndpoint = strings.TrimSpace(configuration.TokenEndpoint)
	configuration.UserinfoEndpoint = strings.TrimSpace(configuration.UserinfoEndpoint)
	configuration.ClientID = strings.TrimSpace(configuration.ClientID)
	configuration.ClientSecret = strings.TrimSpace(configuration.ClientSecret)
	configuration.DefaultRole = strings.ToUpper(strings.TrimSpace(configuration.DefaultRole))
}

func (configuration oidcConfiguration) validate() string {
	allowLocal := oidcLocalDevelopmentAllowed()
	for _, endpoint := range []string{configuration.IssuerURL, configuration.AuthorizationEndpoint, configuration.TokenEndpoint, configuration.UserinfoEndpoint} {
		if err := validateOIDCEndpoint(endpoint, allowLocal); err != nil {
			return err.Error()
		}
	}
	if configuration.ClientID == "" {
		return "clientId is required"
	}
	if configuration.DefaultRole != "MEMBER" && configuration.DefaultRole != "GUEST" {
		return "defaultRole must be MEMBER or GUEST"
	}
	return ""
}

func oidcLocalDevelopmentAllowed() bool {
	publicURL, err := url.Parse(strings.TrimSpace(os.Getenv("XPACE_PUBLIC_URL")))
	if err != nil || publicURL.Scheme != "http" {
		return false
	}
	host := strings.ToLower(publicURL.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func validateOIDCEndpoint(endpoint string, allowLocal bool) error {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("OIDC issuer and endpoints must be valid HTTPS URLs without credentials or fragments")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	localHost := host == "localhost" || strings.HasSuffix(host, ".localhost")
	if parsed.Scheme != "https" && !(allowLocal && parsed.Scheme == "http" && localHost) {
		return fmt.Errorf("OIDC issuer and endpoints must use HTTPS (HTTP localhost is allowed only in local development)")
	}
	if localHost && !allowLocal {
		return fmt.Errorf("OIDC issuer and endpoints must not target local or private networks")
	}
	if strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("OIDC issuer and endpoints must not target local or private networks")
	}
	if ip := net.ParseIP(host); ip != nil && !isPublicNetworkIP(ip) && !(allowLocal && ip.IsLoopback()) {
		return fmt.Errorf("OIDC issuer and endpoints must not target local or private networks")
	}
	return nil
}

func oidcHTTPClient(allowLocal bool) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid OIDC endpoint address: %w", err)
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(addresses) == 0 {
			return nil, fmt.Errorf("could not resolve OIDC endpoint")
		}
		for _, address := range addresses {
			if !isPublicNetworkIP(address.IP) && !(allowLocal && address.IP.IsLoopback()) {
				return nil, fmt.Errorf("OIDC endpoint resolved to a local or private network")
			}
		}
		var dialErr error
		for _, resolved := range addresses {
			connection, attemptErr := dialer.DialContext(ctx, network, net.JoinHostPort(resolved.IP.String(), port))
			if attemptErr == nil {
				return connection, nil
			}
			dialErr = attemptErr
		}
		return nil, fmt.Errorf("could not connect to OIDC endpoint: %w", dialErr)
	}
	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many OIDC endpoint redirects")
			}
			if err := validateOIDCEndpoint(request.URL.String(), allowLocal); err != nil {
				return err
			}
			if len(via) > 0 && !strings.EqualFold(request.URL.Hostname(), via[0].URL.Hostname()) {
				return fmt.Errorf("cross-host OIDC endpoint redirects are not allowed")
			}
			return nil
		},
	}
}

func oidcRedirectURI() string {
	base := strings.TrimRight(os.Getenv("XPACE_PUBLIC_URL"), "/")
	if base == "" {
		base = "http://localhost:3300"
	}
	return base + "/api/v1/auth/sso/oidc/callback"
}

var invalidUsername = regexp.MustCompile(`[^a-z0-9._-]+`)

func oidcUsername(email string) string {
	local := strings.ToLower(strings.Split(strings.TrimSpace(email), "@")[0])
	local = strings.Trim(invalidUsername.ReplaceAllString(local, "-"), "-._")
	if len(local) < 2 {
		local = "oidc-user"
	}
	suffix, _ := randomToken(4)
	return local + "-" + strings.ToLower(suffix)
}
