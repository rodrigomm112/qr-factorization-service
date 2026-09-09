#!/usr/bin/env bash
# deploy-gcp.sh — build, push and deploy the three services to Cloud Run. Idempotent.
#
#   make deploy-plan / make deploy   ->  ./scripts/deploy-gcp.sh plan | apply
#   ./scripts/deploy-gcp.sh destroy [--yes]
#
# Flags: --yes (non-interactive, for CI), --skip-e2e, --skip-billing-check, --project <id>,
#        --tag <tag>. Secrets come from ./.env as TF_VAR_*, never through terraform.tfvars.
#
# Services are pinned BY DIGEST, read back after each push: a tag can move between the push
# and the apply, a digest cannot.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TF_DIR="${ROOT_DIR}/deploy/gcp"
ENV_FILE="${ROOT_DIR}/.env"
CREDS_FILE="${ROOT_DIR}/.env.demo-credentials"
TFVARS="${TF_DIR}/terraform.tfvars"

MODE=""
AUTO_APPROVE="${AUTO_APPROVE:-0}"
SKIP_E2E=0
SKIP_BILLING_CHECK=0
PROJECT_ID="${PROJECT_ID:-}"
REGION="${REGION:-us-central1}"
ARTIFACT_REPO="${ARTIFACT_REPO:-proyectot}"
IMAGE_TAG=""
CLOUD_BUILD=0   # --cloud-build: images are built by Cloud Build, nothing runs on this machine
APPS=(qr-api stats-api web)

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  C_RESET=$'\033[0m'; C_STEP=$'\033[1;36m'; C_OK=$'\033[32m'; C_FAIL=$'\033[1;31m'; C_DIM=$'\033[2m'
else
  C_RESET=''; C_STEP=''; C_OK=''; C_FAIL=''; C_DIM=''
fi
step() { printf '\n%s==> %s%s\n' "${C_STEP}" "$*" "${C_RESET}"; }
ok()   { printf '   %s✓%s %s\n' "${C_OK}" "${C_RESET}" "$*"; }
info() { printf '   %s· %s%s\n' "${C_DIM}" "$*" "${C_RESET}"; }
die()  { printf '\n%s[error]%s %s\n' "${C_FAIL}" "${C_RESET}" "$*" >&2; exit 1; }
usage() { awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"; }

# ---- arguments --------------------------------------------------------------

while [ $# -gt 0 ]; do
  case "$1" in
    plan | --plan)               MODE="plan"; shift ;;
    apply | deploy | --apply)    MODE="apply"; shift ;;
    destroy | --destroy)         MODE="destroy"; shift ;;
    --yes | -y)                  AUTO_APPROVE=1; shift ;;
    --skip-e2e)                  SKIP_E2E=1; shift ;;
    --skip-billing-check)        SKIP_BILLING_CHECK=1; shift ;;
    --project)                   PROJECT_ID="$2"; shift 2 ;;
    --tag)                       IMAGE_TAG="$2"; shift 2 ;;
    --cloud-build)               CLOUD_BUILD=1; shift ;;
    -h | --help)                 usage; exit 0 ;;
    *) printf 'unknown argument: %s\n\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done
[ -n "${MODE}" ] || { usage >&2; exit 2; }

for tool in gcloud terraform docker jq; do
  command -v "${tool}" >/dev/null 2>&1 || die "${tool} is required and is not on PATH"
done

step "1. resolving configuration"

if [ -z "${PROJECT_ID}" ] && [ -f "${TFVARS}" ]; then
  PROJECT_ID="$(awk -F'"' '/^[[:space:]]*project_id[[:space:]]*=/{print $2; exit}' "${TFVARS}")"
fi
if [ -z "${PROJECT_ID}" ]; then
  PROJECT_ID="$(gcloud config get-value project 2>/dev/null || true)"
fi
[ -n "${PROJECT_ID}" ] && [ "${PROJECT_ID}" != "(unset)" ] \
  || die "no project id: pass --project, export PROJECT_ID or set project_id in deploy/gcp/terraform.tfvars"

if [ -z "${IMAGE_TAG}" ]; then
  if git -C "${ROOT_DIR}" rev-parse --short HEAD >/dev/null 2>&1; then
    IMAGE_TAG="$(git -C "${ROOT_DIR}" rev-parse --short=12 HEAD)"
    if [ -n "$(git -C "${ROOT_DIR}" status --porcelain 2>/dev/null)" ]; then
      IMAGE_TAG="${IMAGE_TAG}-dirty"
      info "working tree is dirty: tagging images as ${IMAGE_TAG}"
    fi
  else
    IMAGE_TAG="$(date -u +%Y%m%d%H%M%S)"
    info "not a git repository: falling back to a timestamp tag"
  fi
fi

REGISTRY="${REGION}-docker.pkg.dev/${PROJECT_ID}/${ARTIFACT_REPO}"
ok "project ${PROJECT_ID} · region ${REGION} · tag ${IMAGE_TAG}"

# Read .env without sourcing it: the file holds shell-hostile values.
read_env() { awk -F= -v k="$1" '$1 == k { print substr($0, index($0, "=") + 1) }' "$2" | tail -n 1; }

[ -f "${ENV_FILE}" ] || die "missing ${ENV_FILE} — run: cp .env.example .env && ./scripts/gen-secrets.sh"
JWT_SECRET="$(read_env JWT_SECRET "${ENV_FILE}")"
AUTH_CLIENT_ID="$(read_env AUTH_CLIENT_ID "${ENV_FILE}")"
AUTH_CLIENT_SECRET_SHA256="$(read_env AUTH_CLIENT_SECRET_SHA256 "${ENV_FILE}")"

case "${JWT_SECRET}" in CHANGE_ME_* | "") die "JWT_SECRET is not set in .env — run ./scripts/gen-secrets.sh" ;; esac
case "${AUTH_CLIENT_SECRET_SHA256}" in CHANGE_ME_* | "") die "AUTH_CLIENT_SECRET_SHA256 is not set in .env — run ./scripts/gen-secrets.sh" ;; esac
[ "${#JWT_SECRET}" -ge 32 ] || die "JWT_SECRET must be at least 32 bytes"
ok "secrets loaded from .env (values never printed)"

export TF_VAR_project_id="${PROJECT_ID}"
export TF_VAR_region="${REGION}"
export TF_VAR_artifact_repo="${ARTIFACT_REPO}"
export TF_VAR_image_tag="${IMAGE_TAG}"   # also passed with -var: terraform.tfvars would win over the env var
export TF_VAR_jwt_secret="${JWT_SECRET}"
export TF_VAR_auth_client_id="${AUTH_CLIENT_ID:-demo-client}"
export TF_VAR_auth_client_secret_sha256="${AUTH_CLIENT_SECRET_SHA256}"

step "2. preflight checks"

ACTIVE_ACCOUNT="$(gcloud auth list --filter=status:ACTIVE --format='value(account)' 2>/dev/null | head -n 1)"
[ -n "${ACTIVE_ACCOUNT}" ] || die "no active gcloud account — run: gcloud auth login"
ok "gcloud account: ${ACTIVE_ACCOUNT}"

if ! gcloud auth application-default print-access-token >/dev/null 2>&1; then
  die "no Application Default Credentials — run: gcloud auth application-default login
       (Terraform authenticates with ADC, not with the gcloud account above)"
fi
ok "application default credentials present"

if [ "${SKIP_BILLING_CHECK}" -eq 0 ]; then
  BILLING="$(gcloud billing projects describe "${PROJECT_ID}" --format='value(billingEnabled)' 2>/dev/null || true)"
  case "${BILLING}" in
    True | true) ok "billing enabled on ${PROJECT_ID}" ;;
    False | false) die "billing is NOT enabled on ${PROJECT_ID}.
       Cloud Run, Artifact Registry and Secret Manager all require a billing account
       (the free tier still applies and this demo costs ~0). Link one at
       https://console.cloud.google.com/billing/linkedaccount?project=${PROJECT_ID}
       and re-run. Alternative without billing: any container host that runs the three images." ;;
    *) die "could not read the billing status of ${PROJECT_ID} (missing permission or
       cloudbilling.googleapis.com disabled). Fix it, or re-run with --skip-billing-check
       if you are sure billing is linked." ;;
  esac
else
  info "billing check skipped (--skip-billing-check)"
fi

confirm() {
  [ "${AUTO_APPROVE}" -eq 1 ] && return 0
  printf '\n%s? %s [yes/NO] %s' "${C_STEP}" "$1" "${C_RESET}"
  local answer; read -r answer
  [ "${answer}" = "yes" ] || die "aborted by the operator"
}

step "3. terraform init"
terraform -chdir="${TF_DIR}" init -input=false
ok "initialised"

if [ "${MODE}" = "plan" ]; then
  step "4. terraform plan (no changes applied)"
  terraform -chdir="${TF_DIR}" plan -input=false -var "image_tag=${IMAGE_TAG}"
  printf '\n%sPlan only. Run `make deploy` to apply.%s\n' "${C_OK}" "${C_RESET}"
  exit 0
fi

if [ "${MODE}" = "destroy" ]; then
  step "4. terraform destroy"
  confirm "destroy every Cloud Run service, secret and the image repository of ${PROJECT_ID}?"
  terraform -chdir="${TF_DIR}" destroy -input=false -auto-approve
  printf '\n%sDestroyed. Enabled APIs are kept (disable_on_destroy = false).%s\n' "${C_OK}" "${C_RESET}"
  exit 0
fi

# The registry must exist before `docker push`, and the APIs before the registry.
confirm "deploy Proyecto T to ${PROJECT_ID} (${REGION}) with tag ${IMAGE_TAG}?"

step "4. terraform apply — project APIs and Artifact Registry"
terraform -chdir="${TF_DIR}" apply -input=false -auto-approve -var "image_tag=${IMAGE_TAG}" \
  -target=google_project_service.required \
  -target=google_artifact_registry_repository.docker
ok "registry ready at ${REGISTRY}"

step "5. build + push (linux/amd64)"
declare -A IMAGE_DIGESTS=()

if [ "${CLOUD_BUILD}" -eq 1 ]; then
  # Cloud Build receives each app directory (its .dockerignore applies) and pushes the image;
  # the digest comes back in the build's results, so the local docker daemon is never used.
  for app in "${APPS[@]}"; do
    image="${REGISTRY}/${app}:${IMAGE_TAG}"
    info "cloud-building ${app} -> ${image}"
    substitutions="_IMAGE=${image},_VERSION=${IMAGE_TAG}"
    build_id="$(gcloud builds submit "${ROOT_DIR}/apps/${app}" \
      --project "${PROJECT_ID}" --region "${REGION}" \
      --config "${ROOT_DIR}/deploy/gcp/cloudbuild.yaml" \
      --substitutions "${substitutions}" --format 'value(id)' --quiet 2>&1 | tail -n 1)" \
      || die "cloud build failed for ${app}"
    digest="$(gcloud builds describe "${build_id}" --project "${PROJECT_ID}" --region "${REGION}" \
      --format 'value(results.images[0].digest)' 2>/dev/null || true)"
    [ -n "${digest}" ] || die "could not read the digest of ${app} from build ${build_id}"
    IMAGE_DIGESTS["${app//-/_}"]="${REGISTRY}/${app}@${digest}"
    ok "${app} built in Cloud Build · ${digest}"
  done
else
  gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet
  for app in "${APPS[@]}"; do
    image="${REGISTRY}/${app}:${IMAGE_TAG}"
    info "building ${app} -> ${image}"
    build_args=()
    # Both APIs stamp VERSION so /health/live reports the deployed git SHA.
    case "${app}" in
      qr-api|stats-api) build_args+=(--build-arg "VERSION=${IMAGE_TAG}") ;;
    esac
    docker build --platform linux/amd64 "${build_args[@]}" \
      -t "${image}" "${ROOT_DIR}/apps/${app}" || die "build failed for ${app}"
    docker push "${image}" || die "push failed for ${app} (is Artifact Registry reachable?)"

    # Known only once the registry accepted the manifest, hence after the push; RepoDigests
    # lists one entry per registry the image was ever pushed to, so filter by ours.
    reference="$(docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "${image}" \
      | grep -m 1 "^${REGISTRY}/${app}@sha256:" || true)"
    [ -n "${reference}" ] \
      || die "could not read the digest of ${app} (docker image inspect ${image} → RepoDigests)"
    IMAGE_DIGESTS["${app//-/_}"]="${reference}"
    ok "${app} pushed · ${reference##*@}"
  done
fi

# Terraform reads a complex -var as HCL, and a JSON object is valid HCL.
IMAGE_REFS_JSON="$(jq -nc \
  --arg qr_api "${IMAGE_DIGESTS[qr_api]}" \
  --arg stats_api "${IMAGE_DIGESTS[stats_api]}" \
  --arg web "${IMAGE_DIGESTS[web]}" \
  '{qr_api: $qr_api, stats_api: $stats_api, web: $web}')"

step "6. terraform apply — Cloud Run services, secrets and IAM"
info "services pinned by digest; ${IMAGE_TAG} stays as the human label"
terraform -chdir="${TF_DIR}" apply -input=false -auto-approve \
  -var "image_tag=${IMAGE_TAG}" -var "image_refs=${IMAGE_REFS_JSON}"
ok "infrastructure converged"

QR_URL="$(terraform -chdir="${TF_DIR}" output -raw qr_api_url)"
STATS_URL="$(terraform -chdir="${TF_DIR}" output -raw stats_api_url)"
WEB_URL="$(terraform -chdir="${TF_DIR}" output -raw web_url)"

step "7. deployed URLs"
printf '   web        %s\n' "${WEB_URL}"
printf '   qr-api     %s   (docs: %s/docs)\n' "${QR_URL}" "${QR_URL}"
printf '   stats-api  %s   (docs: %s/docs)\n' "${STATS_URL}" "${STATS_URL}"
terraform -chdir="${TF_DIR}" output -json deployed_images \
  | jq -r 'to_entries[] | "   \(.key | (. + "          ")[0:10]) \(.value)"'

# The module publishes the deterministic hostnames (cloudrun.tf), which CORS and the smoke test
# both trust; a service needs a few seconds after apply before it serves the first request.
wait_for_url() { # wait_for_url <url> <expected status>
  local url="$1" want="$2" code="" i
  for i in $(seq 1 30); do
    code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 "${url}" || true)"
    [ "${code}" = "${want}" ] && return 0
    sleep 3
  done
  die "${url} answered ${code:-nothing} instead of ${want}: the deterministic URL is not serving this service.
       Compare with \`terraform output legacy_urls\` (deploy/gcp/cloudrun.tf explains the URL formats)."
}
wait_for_url "${QR_URL}/health/live" 200
wait_for_url "${STATS_URL}/health/live" 200
wait_for_url "${WEB_URL}/config.js" 200
ok "the three deterministic URLs are serving (CORS allow-list and web config are consistent)"

if [ "${SKIP_E2E}" -eq 1 ]; then
  info "smoke test skipped (--skip-e2e)"
  exit 0
fi

step "8. end-to-end smoke test against the public URLs"
if [ -f "${CREDS_FILE}" ]; then
  E2E_CLIENT_ID="$(read_env AUTH_CLIENT_ID "${CREDS_FILE}")"
  E2E_CLIENT_SECRET="$(read_env AUTH_CLIENT_SECRET "${CREDS_FILE}")"
  export E2E_CLIENT_ID E2E_CLIENT_SECRET
else
  info "no .env.demo-credentials: the smoke test will use E2E_CLIENT_ID/E2E_CLIENT_SECRET from the environment"
fi

bash "${ROOT_DIR}/scripts/e2e-smoke.sh" --no-compose \
  --base-qr "${QR_URL}" --base-stats "${STATS_URL}" --base-web "${WEB_URL}"

printf '\n%sDeployment complete. Open %s%s\n' "${C_OK}" "${WEB_URL}" "${C_RESET}"
