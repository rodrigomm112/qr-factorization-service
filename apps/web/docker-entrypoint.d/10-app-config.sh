#!/bin/sh
# Writes html/config.js from the environment, before nginx starts (name order puts this ahead
# of 20-envsubst-on-templates.sh). Origins out of the bundle = ONE image for localhost and cloud.
set -eu

target="${APP_CONFIG_PATH:-/usr/share/nginx/html/config.js}"
qr_api_base_url="${QR_API_BASE_URL:-http://localhost:8080}"
stats_api_base_url="${STATS_API_BASE_URL:-http://localhost:3000}"

# envsubst also drops these into the CSP `connect-src` without quoting, so a value with a quote
# or a semicolon would break the nginx grammar or inject a directive. Refuse it loudly here.
require_origin() {
    if ! printf '%s' "$2" | grep -Eq '^https?://[A-Za-z0-9._~-]+(:[0-9]{1,5})?(/[A-Za-z0-9._~-]*)*$'; then
        echo "10-app-config.sh: $1 must be a plain http(s) origin, got: $2" >&2
        exit 1
    fi
}

require_origin QR_API_BASE_URL "$qr_api_base_url"
require_origin STATS_API_BASE_URL "$stats_api_base_url"

# Defence in depth: strip control characters, escape what could end the JS string literal.
escape_json() {
    printf '%s' "$1" | tr -d '\000-\037' | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

cat >"$target" <<CONFIG
// Generated at container start by docker-entrypoint.d/10-app-config.sh. Do not edit.
window.__APP_CONFIG__ = {
  qrApiBaseUrl: "$(escape_json "$qr_api_base_url")",
  statsApiBaseUrl: "$(escape_json "$stats_api_base_url")",
};
CONFIG

echo "10-app-config.sh: qrApiBaseUrl=$qr_api_base_url statsApiBaseUrl=$stats_api_base_url"
