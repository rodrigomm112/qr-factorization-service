#!/usr/bin/env bash
# demo.sh — the whole Proyecto T flow in eight steps: token, QR of a tall 3x2, the 3x3 identity
# proving anyDiagonal, a 422, a 401, and the statistics recomputed directly on stats-api.
#
#   make demo                                  # docker compose (default)
#   ./scripts/demo.sh --cloud                  # Cloud Run URLs, read with `terraform output`
#   ./scripts/demo.sh --base-qr URL --base-stats URL [--no-color]
#
# Read-only, and needs curl, jq and a stack that is already up. Credentials:
# E2E_CLIENT_ID/E2E_CLIENT_SECRET, else .env.demo-credentials. The secret is never printed.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TF_DIR="${ROOT_DIR}/deploy/gcp"
CREDS_FILE="${ROOT_DIR}/.env.demo-credentials"
FIXTURES="${ROOT_DIR}/contracts/examples"

BASE_QR="${BASE_QR:-http://localhost:8080}"
BASE_STATS="${BASE_STATS:-http://localhost:3000}"
USE_CLOUD=0
STEP=0

TMP_DIR="$(mktemp -d)"
cleanup() { rm -rf "${TMP_DIR}"; }
trap cleanup EXIT

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  C_RESET=$'\033[0m'; C_STEP=$'\033[1;36m'; C_OK=$'\033[32m'
  C_FAIL=$'\033[1;31m'; C_DIM=$'\033[2m'; C_KEY=$'\033[1m'
else
  C_RESET=''; C_STEP=''; C_OK=''; C_FAIL=''; C_DIM=''; C_KEY=''
fi

step() { STEP=$((STEP + 1)); printf '\n%s%d. %s%s\n' "${C_STEP}" "${STEP}" "$*" "${C_RESET}"; }
ok()   { printf '   %s✓%s %s\n' "${C_OK}" "${C_RESET}" "$*"; }
info() { printf '   %s· %s%s\n' "${C_DIM}" "$*" "${C_RESET}"; }
kv()   { printf '   %s%-22s%s %s\n' "${C_KEY}" "$1" "${C_RESET}" "$2"; }
die()  { printf '\n%s[error]%s %s\n' "${C_FAIL}" "${C_RESET}" "$*" >&2; exit 1; }
usage() { awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --cloud)        USE_CLOUD=1; shift ;;
    --base-qr)      BASE_QR="${2:?--base-qr needs a URL}"; shift 2 ;;
    --base-stats)   BASE_STATS="${2:?--base-stats needs a URL}"; shift 2 ;;
    --no-color)     C_RESET=''; C_STEP=''; C_OK=''; C_FAIL=''; C_DIM=''; C_KEY=''; shift ;;
    -h | --help)    usage; exit 0 ;;
    *) printf 'unknown argument: %s\n\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

for tool in curl jq; do
  command -v "${tool}" >/dev/null 2>&1 || die "${tool} is required and is not on PATH"
done

BASE_QR="${BASE_QR%/}"
BASE_STATS="${BASE_STATS%/}"

BODY="${TMP_DIR}/body.json"
STATUS=0

# request <method> <url> [json body] [bearer token] -> $STATUS, body in $BODY
request() {
  local method="$1" url="$2" body="${3:-}" token="${4:-}"
  local -a args=(-sS -o "${BODY}" -w '%{http_code}' -X "${method}" --max-time 30
                 -H 'Accept: application/json'
                 -H "X-Request-ID: $(request_id)")
  [ -n "${token}" ] && args+=(-H "Authorization: Bearer ${token}")
  [ -n "${body}" ] && args+=(-H 'Content-Type: application/json' --data-binary "${body}")
  STATUS="$(curl "${args[@]}" "${url}" || echo 000)"
}

request_id() {
  if command -v uuidgen >/dev/null 2>&1; then uuidgen | tr 'A-Z' 'a-z'; else
    printf '00000000-0000-4000-8000-%012d\n' "$((RANDOM * RANDOM % 1000000000000))"
  fi
}

expect_status() { # expect_status <wanted status> <what>
  if [ "${STATUS}" = "$1" ]; then
    ok "$2 → HTTP ${STATUS}"
  else
    printf '   %s✗ %s → HTTP %s (expected %s)%s\n' "${C_FAIL}" "$2" "${STATUS}" "$1" "${C_RESET}" >&2
    { jq . "${BODY}" 2>/dev/null || cat "${BODY}" 2>/dev/null || true; } | head -n 20 >&2 || true
    die "the demo stopped at step ${STEP}"
  fi
}

# print_matrix <title> <jq path into $BODY>
print_matrix() {
  printf '   %s%s%s\n' "${C_KEY}" "$1" "${C_RESET}"
  # 6 decimals, Householder noise below the diagonal shown as 0, like the web client.
  jq -r "$2"' | .[]
         | map(if . < 1e-12 and . > -1e-12 then 0 else (. * 1000000 | round / 1000000) end)
         | @tsv' "${BODY}" \
    | while IFS=$'\t' read -r -a row; do
        printf '     '
        printf '%12s' "${row[@]}"
        printf '\n'
      done
}

# print_summary <jq path to the statistics report>
print_summary() {
  jq -r "$1"' | "   máximo   \(.summary.max)\n   mínimo   \(.summary.min)\n   suma     \(.summary.sum)\n   promedio \(.summary.average)\n   elementos \(.summary.count)\n   ¿alguna matriz diagonal?  \(if .summary.anyDiagonal then "sí" else "no" end)"' "${BODY}"
  jq -r "$1"' | .matrices[] | "   · \(.label) \(.rows)×\(.cols)  máx \(.max)  mín \(.min)  suma \(.sum)  diagonal: \(if .isDiagonal then "sí" else "no" end)"' "${BODY}"
}

step "Endpoints"

if [ "${USE_CLOUD}" -eq 1 ]; then
  command -v terraform >/dev/null 2>&1 || die "--cloud needs terraform on PATH"
  [ -f "${TF_DIR}/terraform.tfstate" ] || die "--cloud: no state in ${TF_DIR}; deploy first with \`make deploy\`"
  BASE_QR="$(terraform -chdir="${TF_DIR}" output -raw qr_api_url 2>/dev/null || true)"
  BASE_STATS="$(terraform -chdir="${TF_DIR}" output -raw stats_api_url 2>/dev/null || true)"
  [ -n "${BASE_QR}" ] && [ -n "${BASE_STATS}" ] \
    || die "--cloud: \`terraform output\` gave no qr_api_url/stats_api_url (is the deployment applied?)"
  info "URLs read from deploy/gcp with terraform output"
else
  info "local stack (pass --cloud to run against Cloud Run)"
fi

kv "qr-api" "${BASE_QR}"
kv "stats-api" "${BASE_STATS}"

request GET "${BASE_QR}/health/ready"
expect_status 200 "qr-api readiness"
jq -r '"   estado \(.status) · versión \(.version) · dependencias: " + ((.checks // {}) | keys | join(", "))' "${BODY}"

request GET "${BASE_STATS}/health/ready"
expect_status 200 "stats-api readiness"

step "Token (client credentials)"

CLIENT_ID="${E2E_CLIENT_ID:-}"
CLIENT_SECRET="${E2E_CLIENT_SECRET:-}"
if [ -z "${CLIENT_ID}" ] || [ -z "${CLIENT_SECRET}" ]; then
  [ -f "${CREDS_FILE}" ] \
    || die "no credentials: export E2E_CLIENT_ID/E2E_CLIENT_SECRET or run ./scripts/gen-secrets.sh"
  CLIENT_ID="$(awk -F= '/^AUTH_CLIENT_ID=/{print substr($0, index($0,"=")+1)}' "${CREDS_FILE}" | tail -n 1)"
  CLIENT_SECRET="$(awk -F= '/^AUTH_CLIENT_SECRET=/{print substr($0, index($0,"=")+1)}' "${CREDS_FILE}" | tail -n 1)"
fi
[ -n "${CLIENT_SECRET}" ] || die "empty AUTH_CLIENT_SECRET in ${CREDS_FILE}"

info "POST ${BASE_QR}/api/v1/auth/token   (el secreto no se imprime nunca)"
request POST "${BASE_QR}/api/v1/auth/token" \
  "$(jq -n --arg id "${CLIENT_ID}" --arg secret "${CLIENT_SECRET}" \
        '{clientId: $id, clientSecret: $secret}')"
expect_status 200 "token issued"

TOKEN="$(jq -r '.accessToken' "${BODY}")"
[ -n "${TOKEN}" ] && [ "${TOKEN}" != "null" ] || die "the response carried no accessToken"

kv "clientId" "${CLIENT_ID}"
kv "tokenType" "$(jq -r '.tokenType' "${BODY}")"
kv "scope" "$(jq -r '.scope' "${BODY}")"
kv "expiresIn" "$(jq -r '.expiresIn' "${BODY}") s"
# Masked: enough to recognise in a log, useless to replay.
kv "accessToken" "${TOKEN:0:12}…${TOKEN: -6} (${#TOKEN} bytes)"

step "QR de una matriz alta 3×2 (modo completo)"

TALL="$(jq -c '. + {mode: "full"}' "${FIXTURES}/qr.request.tall-3x2.json")"
info "POST ${BASE_QR}/api/v1/qr   $(jq -c '.matrix' <<<"${TALL}")"
request POST "${BASE_QR}/api/v1/qr" "${TALL}" "${TOKEN}"
expect_status 200 "QR de A (3×2)"

cp "${BODY}" "${TMP_DIR}/qr-tall.json"
print_matrix "Q  $(jq -r '"\(.meta.qShape[0])×\(.meta.qShape[1])  (ortogonal)"' "${BODY}")" '.q'
print_matrix "R  $(jq -r '"\(.meta.rShape[0])×\(.meta.rShape[1])  (triangular superior)"' "${BODY}")" '.r'
printf '   %sEstadísticas de Q y R (qr-api → stats-api)%s\n' "${C_KEY}" "${C_RESET}"
print_summary '.statistics'
jq -r '"   \(.meta.algorithm) · descomposición \(.meta.decompositionMs) ms · estadísticas \(.meta.statisticsMs) ms · requestId \(.requestId)"' "${BODY}"

step "La identidad 3×3 ⇒ anyDiagonal = true"

info "Q y R son ambas diagonales, así que el resumen lo marca."
request POST "${BASE_QR}/api/v1/qr" \
  "$(jq -c '. + {mode: "full"}' "${FIXTURES}/qr.request.identity-3x3.json")" "${TOKEN}"
expect_status 200 "QR de I₃"

ANY_DIAGONAL="$(jq -r '.statistics.summary.anyDiagonal' "${BODY}")"
[ "${ANY_DIAGONAL}" = "true" ] || die "anyDiagonal is ${ANY_DIAGONAL}, expected true"
print_summary '.statistics'

step "Matriz irregular ⇒ 422 problem+json"

info "POST ${BASE_QR}/api/v1/qr con filas de distinta longitud"
request POST "${BASE_QR}/api/v1/qr" \
  "$(jq -c '.' "${FIXTURES}/qr.request.invalid-ragged.json")" "${TOKEN}"
expect_status 422 "matriz irregular rechazada"
jq -r '"   \(.status) \(.title)\n   \(.detail)"' "${BODY}"
jq -r '.errors[]? | "   · \(.pointer)  [\(.code)]  \(.message)"' "${BODY}"

step "Sin token ⇒ 401 problem+json"

request POST "${BASE_QR}/api/v1/qr" "$(jq -c '.' "${FIXTURES}/qr.request.tall-3x2.json")"
expect_status 401 "petición sin Authorization rechazada"
jq -r '"   \(.status) \(.title)\n   \(.detail)\n   type: \(.type)"' "${BODY}"

step "El navegador también llama a stats-api directamente"

info "POST ${BASE_STATS}/api/v1/statistics con la Q y la R del paso 3 y el MISMO token"
request POST "${BASE_STATS}/api/v1/statistics" \
  "$(jq -c '{matrices: [{label: "Q", values: .q}, {label: "R", values: .r}]}' "${TMP_DIR}/qr-tall.json")" \
  "${TOKEN}"
expect_status 200 "estadísticas recalculadas"

print_summary '.'
if [ "$(jq -c '.summary' "${BODY}")" = "$(jq -c '.statistics.summary' "${TMP_DIR}/qr-tall.json")" ]; then
  ok "coincide, campo a campo, con el resumen que devolvió qr-api"
else
  die "the direct call disagrees with what qr-api reported"
fi

step "Listo"
kv "web" "$([ "${USE_CLOUD}" -eq 1 ] && terraform -chdir="${TF_DIR}" output -raw web_url 2>/dev/null || echo 'http://localhost:8081')"
kv "swagger qr-api" "${BASE_QR}/docs"
kv "swagger stats-api" "${BASE_STATS}/docs"
printf '\n%sTodo el flujo respondió como dice el contrato.%s\n' "${C_OK}" "${C_RESET}"
