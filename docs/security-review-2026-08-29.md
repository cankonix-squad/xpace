# Xspace dependency and vulnerability review — 29 August 2026

## Result

- Web production dependency audit: `npm audit --audit-level=high` — **0 vulnerabilities**.
- API reachable-code audit: `govulncheck ./...` — **0 reachable vulnerabilities**.
- `github.com/google/cel-go` was upgraded from `v0.29.0` to `v0.30.0`, resolving GO-2026-6094 in an imported transitive package. All API tests pass after the upgrade.
- CI rejects new high-severity dependency changes on pull requests with the official `actions/dependency-review-action@v5.0.0`.
- Existing CI retains npm audit, govulncheck, Dependabot, and HIGH/CRITICAL Trivy runtime-image gates.

## Accepted residual finding

Govulncheck reports GO-2026-5932 at module level for the deprecated `golang.org/x/crypto/openpgp` package. Xspace does not import or call `openpgp`; it directly uses `golang.org/x/crypto/argon2` for password hashing. The advisory has no fixed module version and is not reachable from Xspace code. The dependency remains monitored through govulncheck and Dependabot.

## Re-run commands

```bash
cd apps/web && npm audit --audit-level=high
cd services/api && go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

Re-run this review before a paid-beta release and after material authentication, media, billing, storage, or tenant-isolation dependency changes.
