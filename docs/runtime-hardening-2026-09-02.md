# Enterprise runtime hardening — 2 September 2026

## Outcome

The production web path now uses a per-request nonce Content Security Policy and contains no browser HTML `style` attributes. API and web application containers run with read-only root filesystems, all Linux capabilities dropped, and `no-new-privileges`; only explicit temporary/cache paths remain writable through memory-backed filesystems.

This closes the CSP and application-container findings from the 30 August and 1 September engineering pentests. It is an engineering acceptance gate, not a substitute for the independent professional pentest required before enterprise general availability.

## Controls implemented

- Next.js `proxy.ts` generates a cryptographically random nonce, forwards it as `x-nonce`, and returns the matching CSP response header. Production uses nonce-bound `script-src` and `style-src`, `strict-dynamic`, `script-src-attr 'none'`, and `style-src-attr 'none'` with no `unsafe-inline` allowance. Development retains the documented allowances needed by the local Next.js toolchain.
- The root theme bootstrap receives the request nonce. Dynamic page styling was moved from JSX style attributes to classes, semantic progress elements, or SVG attributes.
- A CSP-safe image wrapper replaces `next/image` because the latter emits inline style attributes even when application source does not specify them.
- API and web Compose services use `read_only: true`, `cap_drop: [ALL]`, and `security_opt: [no-new-privileges:true]`. API receives a `/tmp` tmpfs; web receives `/tmp` and `/app/.next/cache` tmpfs mounts.
- LiveKit Egress was upgraded from `v1.9.0` to `v1.13.0`. The recording smoke test now waits for Egress to become active and validates its signed download through the internal object-store endpoint.
- LiveKit external-IP discovery remains enabled by default for deployments. `LIVEKIT_USE_EXTERNAL_IP=false` provides an explicit Docker Desktop/local smoke mode, where public NAT hairpinning is unavailable.

The nonce implementation follows the [official Next.js CSP guidance](https://nextjs.org/docs/app/guides/content-security-policy). Egress configuration and runtime behavior follow the [official LiveKit Egress documentation](https://github.com/livekit/egress).

## Verification evidence

- `npm run lint`: passed with zero warnings.
- `npm run build`: passed; all application pages requiring nonce propagation are dynamically rendered and the Next.js Proxy is registered.
- Production container `/login`: HTTP 200, one consistent request nonce across CSP and generated scripts/styles, `html_style_attributes=0`, and no production `unsafe-inline` directive.
- Web runtime inspection: `ReadonlyRootfs=true`, `CapDrop=[ALL]`, `SecurityOpt=[no-new-privileges]`, tmpfs at `/tmp` and `/app/.next/cache`.
- API runtime inspection and readiness: healthy with the same read-only/capability/security settings and `/tmp` tmpfs.
- `scripts/integration-test.sh`: authenticated meeting lifecycle and tenant-isolation suites passed.
- `go test ./cmd/recording-smoke ./internal/httpapi`: passed.
- `LIVEKIT_USE_EXTERNAL_IP=false docker compose run --rm --entrypoint /usr/local/bin/recording-smoke api`: passed create room, publish audio, record MP4, signed partial download, and object deletion.
- Proxy and edge Compose overlays both passed `docker compose config --quiet`.

The Open Graph image route still uses React style objects by design; it renders a PNG through `ImageResponse` and does not emit browser HTML governed by `style-src-attr`.
