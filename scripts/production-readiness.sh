#!/bin/sh
set -eu

web_domain=${XPACE_DOMAIN:-xpace.cankonix.com}
livekit_domain=${LIVEKIT_DOMAIN:-livekit.xpace.cankonix.com}
failed=0

check_dns() {
  domain=$1
  addresses=$(dig +time=4 +tries=1 +short A "$domain"; dig +time=4 +tries=1 +short AAAA "$domain")
  if [ -z "$addresses" ]; then
    echo "FAIL: $domain has no resolvable A or AAAA record"
    failed=1
    return
  fi
  echo "PASS: $domain resolves"
}

check_https() {
  domain=$1
  if curl --connect-timeout 5 --max-time 15 -fsSI "https://$domain/" >/dev/null 2>&1; then
    echo "PASS: https://$domain has a trusted certificate and responds"
  else
    echo "FAIL: https://$domain did not pass trusted HTTPS validation"
    failed=1
  fi
}

check_dns "$web_domain"
check_dns "$livekit_domain"
check_https "$web_domain"
if [ "$failed" -ne 0 ]; then
  echo "Production readiness failed. Fix DNS, Traefik routing, and ACME before deployment approval."
  exit 1
fi

echo "Production DNS and TLS readiness passed."
