package httpapi

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type oidcProviderMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

type oidcJWKSet struct {
	Keys []oidcJWK `json:"keys"`
}

type oidcJWK struct {
	KeyType string `json:"kty"`
	KeyID   string `json:"kid"`
	Use     string `json:"use"`
	Alg     string `json:"alg"`
	N       string `json:"n"`
	E       string `json:"e"`
}

type oidcIDTokenClaims struct {
	jwt.RegisteredClaims
	Nonce           string `json:"nonce"`
	AuthorizedParty string `json:"azp"`
	AccessTokenHash string `json:"at_hash"`
}

func validateOIDCIDToken(ctx context.Context, configuration oidcConfiguration, rawIDToken, accessToken, expectedNonceHash string) (string, error) {
	var metadata oidcProviderMetadata
	discoveryEndpoint := strings.TrimRight(configuration.IssuerURL, "/") + "/.well-known/openid-configuration"
	if err := oidcJSONRequest(ctx, http.MethodGet, discoveryEndpoint, nil, "", "", &metadata); err != nil {
		return "", fmt.Errorf("could not load OIDC discovery metadata: %w", err)
	}
	if metadata.Issuer != configuration.IssuerURL || metadata.AuthorizationEndpoint != configuration.AuthorizationEndpoint || metadata.TokenEndpoint != configuration.TokenEndpoint || metadata.UserinfoEndpoint != configuration.UserinfoEndpoint || metadata.JWKSURI == "" {
		return "", fmt.Errorf("OIDC discovery metadata does not match the configured provider")
	}
	if err := validateOIDCEndpoint(metadata.JWKSURI, oidcLocalDevelopmentAllowed()); err != nil {
		return "", fmt.Errorf("invalid OIDC JWKS endpoint: %w", err)
	}

	var keySet oidcJWKSet
	if err := oidcJSONRequest(ctx, http.MethodGet, metadata.JWKSURI, nil, "", "", &keySet); err != nil {
		return "", fmt.Errorf("could not load OIDC signing keys: %w", err)
	}
	claims := &oidcIDTokenClaims{}
	token, err := jwt.ParseWithClaims(rawIDToken, claims, func(token *jwt.Token) (any, error) {
		keyID, _ := token.Header["kid"].(string)
		if keyID == "" {
			return nil, fmt.Errorf("OIDC ID token has no key identifier")
		}
		for _, key := range keySet.Keys {
			if key.KeyID == keyID && key.KeyType == "RSA" && (key.Use == "" || key.Use == "sig") && (key.Alg == "" || key.Alg == "RS256") {
				return oidcRSAPublicKey(key)
			}
		}
		return nil, fmt.Errorf("OIDC signing key was not found")
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(configuration.IssuerURL), jwt.WithAudience(configuration.ClientID), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(30*time.Second))
	if err != nil || !token.Valid {
		return "", fmt.Errorf("invalid OIDC ID token: %w", err)
	}
	if claims.Subject == "" || claims.IssuedAt == nil || claims.Nonce == "" || hashToken(claims.Nonce) != expectedNonceHash {
		return "", fmt.Errorf("OIDC ID token subject, issue time, or nonce is invalid")
	}
	if len(claims.Audience) > 1 && claims.AuthorizedParty != configuration.ClientID {
		return "", fmt.Errorf("OIDC ID token authorized party is invalid")
	}
	if claims.AuthorizedParty != "" && claims.AuthorizedParty != configuration.ClientID {
		return "", fmt.Errorf("OIDC ID token authorized party is invalid")
	}
	if claims.AccessTokenHash != "" && claims.AccessTokenHash != oidcAccessTokenHash(accessToken) {
		return "", fmt.Errorf("OIDC ID token access token hash is invalid")
	}
	return claims.Subject, nil
}

func oidcRSAPublicKey(key oidcJWK) (*rsa.PublicKey, error) {
	modulus, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil || len(modulus) == 0 {
		return nil, fmt.Errorf("OIDC RSA modulus is invalid")
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
		return nil, fmt.Errorf("OIDC RSA exponent is invalid")
	}
	exponent := 0
	for _, value := range exponentBytes {
		exponent = exponent<<8 | int(value)
	}
	publicKey := &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}
	if publicKey.N.BitLen() < 2048 || publicKey.E < 3 {
		return nil, fmt.Errorf("OIDC RSA signing key is too weak")
	}
	return publicKey, nil
}

func oidcAccessTokenHash(accessToken string) string {
	sum := sha256.Sum256([]byte(accessToken))
	return base64.RawURLEncoding.EncodeToString(sum[:len(sum)/2])
}
