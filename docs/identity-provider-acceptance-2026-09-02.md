# Provider-agnostic OIDC and SCIM acceptance — 2 September 2026

## Outcome

Xpace now has a repeatable local identity acceptance test that does not require a Microsoft Entra ID or Okta tenant. The test uses a disposable standards-compatible OIDC provider with an RSA signing key and the production SCIM HTTP handlers against PostgreSQL.

This is implementation evidence, not certification for a named identity provider. Acceptance with a real customer or trial Entra ID/Okta tenant remains an external launch task.

## OIDC controls exercised

- Authorization Code flow with PKCE S256 and a single-use state value.
- OpenID Provider discovery metadata must match the configured issuer, authorization, token, and UserInfo endpoints.
- JWKS signing-key retrieval through the SSRF-guarded HTTP client.
- RS256 signature, issuer, audience, authorized party, expiry, issued-at, nonce, and optional access-token hash validation.
- ID-token subject must match the UserInfo subject.
- New account linking and JIT provisioning require an explicitly verified email claim.
- Existing SCIM account linking preserves its current role; JIT provisioning applies only the configured `MEMBER` or `GUEST` default role.
- Replayed state, invalid nonce, invalid signature, invalid access-token hash, and unverified email are rejected.

## SCIM controls exercised

- Tenant-scoped bearer token generation.
- User and group provisioning with group membership.
- A valid token cannot access a different tenant path.
- A SCIM-created user can be linked to the matching verified OIDC identity without role escalation.
- User deactivation revokes active sessions immediately.
- Configuration, provisioning, login, group, and deprovisioning audit events are persisted.

## Repeatable verification

Run:

```sh
scripts/integration-test.sh
```

The GitHub Actions `Meeting and identity integration` job runs this acceptance together with the meeting lifecycle and broader tenant-isolation suite.

## Remaining external acceptance

Before claiming compatibility with a named provider, run the same lifecycle using a real Microsoft Entra ID, Okta, Keycloak, Auth0, or customer-managed OIDC/SCIM environment. Record provider metadata, claim mappings, provisioning logs, deactivation latency, administrator consent, conditional-access behavior, and logout/session expectations. SAML is not implemented by this checkpoint.
