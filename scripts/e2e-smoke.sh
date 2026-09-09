#!/usr/bin/env bash
# e2e-smoke.sh — smoke test of the whole stack (web + qr-api + stats-api) over HTTP with
# curl + jq only, so the same script covers compose, CI and the public Cloud Run URLs.
#
#   make e2e                                   # compose up --wait, run, tear down
#   ./scripts/e2e-smoke.sh --keep              # leave the stack running
#   ./scripts/e2e-smoke.sh --no-compose --base-qr URL --base-stats URL --base-web URL
#
# --no-compose is the cloud (and CI) mode: nothing is started or stopped. Credentials:
# E2E_CLIENT_ID/E2E_CLIENT_SECRET, else .env.demo-credentials. Exit 0 only if all pass.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CREDS_FILE="${ROOT_DIR}/.env.demo-credentials"
FIXTURES="${ROOT_DIR}/contracts/examples"

BASE_QR="${BASE_QR:-http://localhost:8080}"
BASE_STATS="${BASE_STATS:-http://localhost:3000}"
BASE_WEB="${BASE_WEB:-http://localhost:8081}"
USE_COMPOSE=1
KEEP=0
READY_DEADLINE_S=90

WE_STARTED_STACK=0
TESTS_RUN=0
TESTS_FAILED=0
CURRENT_STEP="startup"

# Last response body and headers, so a failure can show them.
TMP_DIR="$(mktemp -d)"
BODY="${TMP_DIR}/body"
HEADERS="${TMP_DIR}/headers"
STATUS=0

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  C_RESET=$'\033[0m'; C_STEP=$'\033[1;36m'; C_OK=$'\033[32m'
  C_FAIL=$'\033[1;31m'; C_DIM=$'\033[2m'
else
  C_RESET=''; C_STEP=''; C_OK=''; C_FAIL=''; C_DIM=''
fi

step() { CURRENT_STEP="$*"; printf '\n%s==> %s%s\n' "${C_STEP}" "$*" "${C_RESET}"; }
ok()   { printf '   %s✓%s %s\n' "${C_OK}" "${C_RESET}" "$*"; }
info() { printf '   %s· %s%s\n' "${C_DIM}" "$*" "${C_RESET}"; }
bad()  { printf '   %s✗ %s%s\n' "${C_FAIL}" "$*" "${C_RESET}" >&2; }

usage() { awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"; }

# ---- assertions -------------------------------------------------------------

# assert_eq <description> <expected> <actual>
assert_eq() {
  TESTS_RUN=$((TESTS_RUN + 1))
  if [ "$2" = "$3" ]; then
    ok "$1"
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    bad "$1: expected '$2', got '$3'"
    fail "assertion failed in step: ${CURRENT_STEP}"
  fi
}

# assert_status <description> <expected status> — reads $STATUS from the last call
assert_status() { assert_eq "$1 → HTTP $2" "$2" "${STATUS}"; }

# assert_jq <description> <jq filter> — the filter must be true on $BODY
assert_jq() {
  local desc="$1" filter="$2" result
  result="$(jq -r "${filter}" "${BODY}" 2>&1 || true)"
  assert_eq "${desc}" "true" "${result}"
}

assert_jq_eq() {
  local desc="$1" filter="$2" expected="$3" actual
  actual="$(jq -r "${filter}" "${BODY}" 2>&1 || true)"
  assert_eq "${desc}" "${expected}" "${actual}"
}

# assert_contains <description> <needle> <file>
assert_contains() {
  TESTS_RUN=$((TESTS_RUN + 1))
  if grep -qF -- "$2" "$3"; then
    ok "$1"
  else
    TESTS_FAILED=$((TESTS_FAILED + 1))
    bad "$1: '$2' not found"
    fail "assertion failed in step: ${CURRENT_STEP}"
  fi
}

assert_header() {
  local desc="$1" name="$2" prefix="${3:-}" value
  value="$(tr -d '\r' < "${HEADERS}" | awk -v h="$(printf '%s' "$2" | tr 'A-Z' 'a-z')" \
    'BEGIN{FS=": "} {k=tolower($1)} k==h {sub(/^[^:]*: /,""); print; exit}')"
  if [ -z "${prefix}" ]; then
    TESTS_RUN=$((TESTS_RUN + 1))
    if [ -n "${value}" ]; then ok "${desc} (${name}: ${value})"; else
      TESTS_FAILED=$((TESTS_FAILED + 1)); bad "${desc}: header ${name} absent"
      fail "assertion failed in step: ${CURRENT_STEP}"
    fi
  else
    assert_eq "${desc}" "${prefix}" "${value:0:${#prefix}}"
  fi
}

fail() {
  bad "$*"
  printf '\n%s--- last response (status %s) ---%s\n' "${C_DIM}" "${STATUS}" "${C_RESET}" >&2
  head -c 2000 "${BODY}" >&2 2>/dev/null || true
  printf '\n' >&2
  if [ "${USE_COMPOSE}" -eq 1 ]; then
    printf '%s--- docker compose logs --tail=50 ---%s\n' "${C_DIM}" "${C_RESET}" >&2
    (cd "${ROOT_DIR}" && docker compose logs --tail=50 2>&1 | tail -n 200) >&2 || true
  fi
  exit 1
}

cleanup() {
  local code=$?
  rm -rf "${TMP_DIR}"
  if [ "${USE_COMPOSE}" -eq 1 ] && [ "${KEEP}" -eq 0 ] && [ "${WE_STARTED_STACK}" -eq 1 ]; then
    printf '\n%s==> docker compose down%s\n' "${C_STEP}" "${C_RESET}"
    (cd "${ROOT_DIR}" && docker compose down --volumes --remove-orphans >/dev/null 2>&1) || true
  fi
  exit "${code}"
}
trap cleanup EXIT

# ---- HTTP -------------------------------------------------------------------

# request <method> <url> [body file or "-"] [curl args...] -> $BODY, $HEADERS, $STATUS
request() {
  local method="$1" url="$2" data="${3:--}"; shift 3 || shift 2
  local args=(-sS -o "${BODY}" -D "${HEADERS}" -w '%{http_code}' -X "${method}" --max-time 60)
  if [ "${data}" != "-" ]; then
    args+=(-H 'Content-Type: application/json' --data-binary "@${data}")
  fi
  STATUS="$(curl "${args[@]}" "$@" "${url}")" || {
    STATUS=0
    fail "curl could not reach ${method} ${url}"
  }
}

auth_request() {
  request "$1" "$2" "${3:--}" -H "Authorization: Bearer ${ACCESS_TOKEN}"
}

wait_ready() {
  local name="$1" base="$2" started="${SECONDS}" code
  while true; do
    code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 5 "${base}/health/ready" 2>/dev/null || echo 000)"
    [ "${code}" = "200" ] && { ok "${name} ready after $((SECONDS - started))s"; return 0; }
    if [ $((SECONDS - started)) -ge "${READY_DEADLINE_S}" ]; then
      STATUS="${code}"
      fail "${name} did not become ready in ${READY_DEADLINE_S}s (last status ${code})"
    fi
    sleep 2
  done
}

# ---- arguments --------------------------------------------------------------

while [ $# -gt 0 ]; do
  case "$1" in
    --base-qr)    BASE_QR="$2"; shift 2 ;;
    --base-stats) BASE_STATS="$2"; shift 2 ;;
    --base-web)   BASE_WEB="$2"; shift 2 ;;
    --no-compose) USE_COMPOSE=0; shift ;;
    --keep)       KEEP=1; shift ;;
    -h | --help)  usage; exit 0 ;;
    *) printf 'unknown argument: %s\n\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

BASE_QR="${BASE_QR%/}"; BASE_STATS="${BASE_STATS%/}"; BASE_WEB="${BASE_WEB%/}"

for tool in curl jq; do
  command -v "${tool}" >/dev/null 2>&1 || { echo "e2e: ${tool} is required" >&2; exit 2; }
done
[ "${USE_COMPOSE}" -eq 1 ] && { command -v docker >/dev/null 2>&1 || { echo "e2e: docker is required (or use --no-compose)" >&2; exit 2; }; }

printf '%sProyecto T — end-to-end smoke test%s\n' "${C_STEP}" "${C_RESET}"
info "qr-api    ${BASE_QR}"
info "stats-api ${BASE_STATS}"
info "web       ${BASE_WEB}"
info "compose   $([ "${USE_COMPOSE}" -eq 1 ] && echo enabled || echo 'disabled (--no-compose)')"

if [ "${USE_COMPOSE}" -eq 1 ]; then
  step "1. docker compose up"
  [ -f "${ROOT_DIR}/.env" ] || fail ".env not found — run: cp .env.example .env && ./scripts/gen-secrets.sh"
  running="$( (cd "${ROOT_DIR}" && docker compose ps --status running --format '{{.Service}}' 2>/dev/null) | wc -l || true)"
  if [ "${running}" -ge 3 ]; then
    info "stack already running: reusing it and leaving it up at the end"
  else
    WE_STARTED_STACK=1
    (cd "${ROOT_DIR}" && docker compose up -d --build --wait) || fail "docker compose up failed"
  fi
  ok "compose stack up"
else
  step "1. compose skipped (--no-compose)"
fi

step "2. readiness (deadline ${READY_DEADLINE_S}s)"
wait_ready "stats-api" "${BASE_STATS}"
wait_ready "qr-api" "${BASE_QR}"

step "3. client credentials → access token"
CLIENT_ID="${E2E_CLIENT_ID:-}"
CLIENT_SECRET="${E2E_CLIENT_SECRET:-}"
if [ -z "${CLIENT_ID}" ] || [ -z "${CLIENT_SECRET}" ]; then
  [ -f "${CREDS_FILE}" ] || fail "no credentials: export E2E_CLIENT_ID/E2E_CLIENT_SECRET or run ./scripts/gen-secrets.sh"
  CLIENT_ID="$(awk -F= '/^AUTH_CLIENT_ID=/{print substr($0, index($0,"=")+1)}' "${CREDS_FILE}" | tail -n 1)"
  CLIENT_SECRET="$(awk -F= '/^AUTH_CLIENT_SECRET=/{print substr($0, index($0,"=")+1)}' "${CREDS_FILE}" | tail -n 1)"
  info "credentials read from .env.demo-credentials"
fi
[ -n "${CLIENT_ID}" ] && [ -n "${CLIENT_SECRET}" ] || fail "empty clientId/clientSecret"

jq -n --arg id "${CLIENT_ID}" --arg secret "${CLIENT_SECRET}" \
  '{clientId: $id, clientSecret: $secret}' > "${TMP_DIR}/token.json"
request POST "${BASE_QR}/api/v1/auth/token" "${TMP_DIR}/token.json"
assert_status "POST /api/v1/auth/token" 200
assert_jq "tokenType is Bearer" '.tokenType == "Bearer"'
assert_jq "accessToken is a JWT (three dot-separated parts)" '(.accessToken | split(".") | length) == 3'
assert_jq "expiresIn is a positive number" '.expiresIn > 0'
assert_header "Cache-Control: no-store on the token response" "cache-control" "no-store"
ACCESS_TOKEN="$(jq -r '.accessToken' "${BODY}")"

step "4. POST /api/v1/qr — tall 3x2 (full QR)"
auth_request POST "${BASE_QR}/api/v1/qr" "${FIXTURES}/qr.request.tall-3x2.json"
assert_status "3x2 factorization" 200
assert_jq "Q has 3 rows"    '(.q | length) == 3'
assert_jq "Q has 3 columns" '(.q[0] | length) == 3'
assert_jq "R has 3 rows"    '(.r | length) == 3'
assert_jq "R has 2 columns" '(.r[0] | length) == 2'
assert_jq "statistics.summary.count == 15 (9 of Q + 6 of R)" '.statistics.summary.count == 15'
assert_jq "statistics.summary.anyDiagonal == false"          '.statistics.summary.anyDiagonal == false'
assert_jq_eq "meta.algorithm == householder" '.meta.algorithm' 'householder'
assert_jq_eq "meta.mode == full"             '.meta.mode' 'full'
assert_jq "meta.qShape == [3,3]" '.meta.qShape == [3,3]'
assert_jq "meta.rShape == [3,2]" '.meta.rShape == [3,2]'
assert_jq "R is upper triangular (exact zeros below the diagonal)" \
  '[.r | to_entries[] | .key as $i | .value | to_entries[] | select(.key < $i) | .value] | all(. == 0)'
assert_header "X-Request-ID echoed on success" "x-request-id"

step "5. POST /api/v1/qr — identity 3x3 (Q = R = I, anyDiagonal = true)"
auth_request POST "${BASE_QR}/api/v1/qr" "${FIXTURES}/qr.request.identity-3x3.json"
assert_status "identity factorization" 200
assert_jq "statistics.summary.anyDiagonal == true" '.statistics.summary.anyDiagonal == true'
assert_jq "statistics.summary.count == 18"         '.statistics.summary.count == 18'
IDENTITY_DEV_FILTER='
  def dev($m): [ $m | to_entries[] | .key as $i | .value | to_entries[]
                 | ((.value - (if .key == $i then 1 else 0 end)) | if . < 0 then -. else . end) ];
  (dev(.q) + dev(.r)) | max'
assert_jq "max |Q-I| and |R-I| <= 1e-12" "(${IDENTITY_DEV_FILTER}) <= 1e-12"

step "6. POST /api/v1/qr — mode \"reduced\" on a 5x2 (Q becomes 5x2)"
jq -nc '{matrix: [[1,2],[3,4],[5,6],[7,8],[9,11]], mode: "reduced"}' > "${TMP_DIR}/reduced.json"
auth_request POST "${BASE_QR}/api/v1/qr" "${TMP_DIR}/reduced.json"
assert_status "reduced factorization" 200
assert_jq "Q has 5 rows"            '(.q | length) == 5'
assert_jq "Q has 2 columns (k = min(m,n))" '(.q[0] | length) == 2'
assert_jq "R is 2x2"                '(.r | length) == 2 and (.r[0] | length) == 2'
assert_jq_eq "meta.mode == reduced" '.meta.mode' 'reduced'

step "7. limit coherence — 100x100 (qr-api's worst case fits in stats-api)"
jq -nc '{matrix: [range(100) as $i | [range(100) as $j
          | (if $i == $j then 200 else 0 end) + (($i * $j) % 7) + 1]]}' > "${TMP_DIR}/big.json"
auth_request POST "${BASE_QR}/api/v1/qr" "${TMP_DIR}/big.json"
assert_status "100x100 factorization + statistics" 200
assert_jq "statistics.summary.count == 20000 (two 100x100 matrices)" '.statistics.summary.count == 20000'
info "response size: $(wc -c < "${BODY}") bytes"

step "8. validation — ragged matrix ⇒ 422 problem+json"
auth_request POST "${BASE_QR}/api/v1/qr" "${FIXTURES}/qr.request.invalid-ragged.json"
assert_status "ragged matrix" 422
assert_header "Content-Type is problem+json" "content-type" "application/problem+json"
assert_jq_eq "type is the validation-error URN" '.type' 'urn:proyectot:problem:validation-error'
assert_jq_eq "errors[0].code == ragged_row" '.errors[0].code' 'ragged_row'
assert_jq "errors[0].pointer points at the offending row" '(.errors[0].pointer | startswith("/matrix"))'

step "9. auth — POST /api/v1/qr without a token ⇒ 401"
request POST "${BASE_QR}/api/v1/qr" "${FIXTURES}/qr.request.tall-3x2.json"
assert_status "unauthenticated /qr" 401
assert_header "Content-Type is problem+json" "content-type" "application/problem+json"
assert_header "WWW-Authenticate present" "www-authenticate"
assert_jq_eq "type is the unauthorized URN" '.type' 'urn:proyectot:problem:unauthorized'
assert_jq "no errors[] on a non-validation problem" 'has("errors") == false'

step "10. stats-api called DIRECTLY with the same token (browser path)"
request POST "${BASE_STATS}/api/v1/statistics" "${FIXTURES}/statistics.request.mixed-2x2.json" \
  -H "Authorization: Bearer ${ACCESS_TOKEN}"
assert_status "direct /api/v1/statistics" 200
EXPECTED_SUMMARY="$(jq -Sc '.summary' "${FIXTURES}/statistics.response.mixed-2x2.json")"
ACTUAL_SUMMARY="$(jq -Sc '.summary' "${BODY}")"
assert_eq "summary matches the golden fixture (mixed-2x2)" "${EXPECTED_SUMMARY}" "${ACTUAL_SUMMARY}"
assert_jq_eq "meta.summationAlgorithm == neumaier" '.meta.summationAlgorithm' 'neumaier'

step "11. web — index references config.js and config.js defines __APP_CONFIG__"
request GET "${BASE_WEB}/"
assert_status "GET /" 200
assert_contains "index.html references config.js" "config.js" "${BODY}"
request GET "${BASE_WEB}/config.js"
assert_status "GET /config.js" 200
assert_contains "config.js defines window.__APP_CONFIG__" "__APP_CONFIG__" "${BODY}"

if [ "${USE_COMPOSE}" -eq 1 ]; then
  step "12. fail-fast — stats-api stopped ⇒ 502 downstream-unavailable"
  (cd "${ROOT_DIR}" && docker compose stop stats-api >/dev/null 2>&1) || fail "could not stop stats-api"
  auth_request POST "${BASE_QR}/api/v1/qr" "${FIXTURES}/qr.request.tall-3x2.json"
  assert_status "/qr with stats-api down" 502
  assert_header "Content-Type is problem+json" "content-type" "application/problem+json"
  assert_jq_eq "type is the downstream-unavailable URN" '.type' 'urn:proyectot:problem:downstream-unavailable'
  assert_jq "no partial 200: the body carries no q/r" 'has("q") == false and has("r") == false'

  info "restarting stats-api"
  (cd "${ROOT_DIR}" && docker compose start stats-api >/dev/null 2>&1) || fail "could not restart stats-api"
  wait_ready "stats-api" "${BASE_STATS}"
  wait_ready "qr-api" "${BASE_QR}"
  auth_request POST "${BASE_QR}/api/v1/qr" "${FIXTURES}/qr.request.tall-3x2.json"
  assert_status "/qr recovers after stats-api is back" 200
else
  step "12. fail-fast check skipped (--no-compose: cannot stop a remote service)"
fi

# ---- summary ----------------------------------------------------------------

printf '\n%s%d assertions passed, %d failed%s\n' \
  "$([ "${TESTS_FAILED}" -eq 0 ] && printf '%s' "${C_OK}" || printf '%s' "${C_FAIL}")" \
  "$((TESTS_RUN - TESTS_FAILED))" "${TESTS_FAILED}" "${C_RESET}"
[ "${TESTS_FAILED}" -eq 0 ] || exit 1
printf '%se2e smoke test PASSED%s\n' "${C_OK}" "${C_RESET}"
