#!/usr/bin/env bash
# rollback-gcp.sh — one `gcloud run services update-traffic` per service, back to its previous
# revision. Faster than any terraform path out of a bad deploy, and exact: deploys pin digests.
#
#   make rollback                                   # every service, asks first
#   ./scripts/rollback-gcp.sh --service qr-api [--revision qr-api-00007-abc]
#   ./scripts/rollback-gcp.sh --list | --dry-run    # show revisions | print the commands
#
# Flags: --service, --revision, --yes|-y (for CI), --dry-run, --list, --project, --region.
# Terraform keeps pointing at the new revision: fix forward, or re-apply the previous digest.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TFVARS="${ROOT_DIR}/deploy/gcp/terraform.tfvars"

PROJECT_ID="${PROJECT_ID:-}"
REGION="${REGION:-us-central1}"
SERVICES=()
TARGET_REVISION=""
AUTO_APPROVE="${AUTO_APPROVE:-0}"
DRY_RUN=0
LIST_ONLY=0
DEFAULT_SERVICES=(qr-api stats-api web)

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

while [ $# -gt 0 ]; do
  case "$1" in
    --service)     SERVICES+=("${2:?--service needs a name}"); shift 2 ;;
    --revision)    TARGET_REVISION="${2:?--revision needs a name}"; shift 2 ;;
    --project)     PROJECT_ID="${2:?--project needs an id}"; shift 2 ;;
    --region)      REGION="${2:?--region needs a region}"; shift 2 ;;
    --yes | -y)    AUTO_APPROVE=1; shift ;;
    --dry-run)     DRY_RUN=1; shift ;;
    --list)        LIST_ONLY=1; shift ;;
    -h | --help)   usage; exit 0 ;;
    *) printf 'unknown argument: %s\n\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

[ "${#SERVICES[@]}" -gt 0 ] || SERVICES=("${DEFAULT_SERVICES[@]}")

if [ -n "${TARGET_REVISION}" ] && [ "${#SERVICES[@]}" -ne 1 ]; then
  die "--revision pins one revision, so it needs exactly one --service"
fi

if [ "${DRY_RUN}" -eq 0 ]; then
  command -v gcloud >/dev/null 2>&1 || die "gcloud is required and is not on PATH (or use --dry-run)"
fi

if [ -z "${PROJECT_ID}" ] && [ -f "${TFVARS}" ]; then
  PROJECT_ID="$(awk -F'"' '/^[[:space:]]*project_id[[:space:]]*=/{print $2; exit}' "${TFVARS}")"
fi
if [ -z "${PROJECT_ID}" ] && command -v gcloud >/dev/null 2>&1; then
  PROJECT_ID="$(gcloud config get-value project 2>/dev/null || true)"
fi
if [ -z "${PROJECT_ID}" ] || [ "${PROJECT_ID}" = "(unset)" ]; then
  if [ "${DRY_RUN}" -eq 1 ]; then
    PROJECT_ID="<PROJECT_ID>"
  else
    die "no project id: pass --project, export PROJECT_ID or set project_id in deploy/gcp/terraform.tfvars"
  fi
fi

# ---- command runner ---------------------------------------------------------

# show <cmd...> — prints a command the way an operator would paste it.
show() { printf '   %s$ %s%s\n' "${C_DIM}" "$*" "${C_RESET}"; }

# capture <cmd...> — read-only; --dry-run prints and yields nothing, so callers cope with ''.
capture() {
  if [ "${DRY_RUN}" -eq 1 ]; then
    show "$@" >&2
    return 0
  fi
  # Swallowed here, not by the caller: a caller-side `2>/dev/null` would also hide `show`.
  "$@" 2>/dev/null
}

# apply_cmd <cmd...> — the only mutating call; never executed in --dry-run.
apply_cmd() {
  show "$@"
  [ "${DRY_RUN}" -eq 1 ] && return 0
  "$@"
}

confirm() {
  # --dry-run executes nothing, so there is nothing to confirm.
  [ "${DRY_RUN}" -eq 1 ] && return 0
  [ "${AUTO_APPROVE}" -eq 1 ] && return 0
  printf '\n%s? %s [yes/NO] %s' "${C_STEP}" "$1" "${C_RESET}"
  local answer=""
  read -r answer || true
  [ "${answer}" = "yes" ] || die "aborted by the operator"
}

# ---- per service ------------------------------------------------------------

# With the LATEST allocation Terraform uses, `traffic[0].revisionName` is empty and the
# serving revision is latestReadyRevisionName.
serving_revision() { # serving_revision <service>
  local named=""
  named="$(capture gcloud run services describe "$1" \
    --region "${REGION}" --project "${PROJECT_ID}" \
    --format 'value(status.traffic[0].revisionName)' || true)"
  if [ -z "${named}" ]; then
    named="$(capture gcloud run services describe "$1" \
      --region "${REGION}" --project "${PROJECT_ID}" \
      --format 'value(status.latestReadyRevisionName)' || true)"
  fi
  printf '%s\n' "${named}"
}

# Newest first: [0] is (or is about to be) serving, [1] is where a rollback goes.
list_revisions() { # list_revisions <service>
  capture gcloud run revisions list \
    --service "$1" --region "${REGION}" --project "${PROJECT_ID}" \
    --sort-by '~metadata.creationTimestamp' --limit 10 \
    --format 'value(metadata.name)' || true
}

rolled_back=0
skipped=0

for service in "${SERVICES[@]}"; do
  step "${service} (project ${PROJECT_ID}, region ${REGION})"

  mapfile -t revisions < <(list_revisions "${service}")
  current="$(serving_revision "${service}")"

  if [ "${#revisions[@]}" -eq 0 ]; then
    if [ "${DRY_RUN}" -eq 1 ]; then
      info "dry run: the commands above are what reads the revision list"
      revisions=("<latest-revision>" "<previous-revision>")
      current="${revisions[0]}"
    else
      info "no revisions found — is ${service} deployed in ${REGION}?"
      skipped=$((skipped + 1))
      continue
    fi
  fi

  printf '   %-34s %s\n' "revision" "state"
  for revision in "${revisions[@]}"; do
    if [ "${revision}" = "${current}" ]; then
      printf '   %-34s %sserving 100%%%s\n' "${revision}" "${C_OK}" "${C_RESET}"
    else
      printf '   %-34s %s\n' "${revision}" "-"
    fi
  done

  target="${TARGET_REVISION}"
  if [ -z "${target}" ]; then
    for revision in "${revisions[@]}"; do
      if [ "${revision}" != "${current}" ]; then
        target="${revision}"
        break
      fi
    done
  fi

  if [ -z "${target}" ]; then
    info "only one revision: there is nothing to roll back to"
    skipped=$((skipped + 1))
    continue
  fi

  if [ "${LIST_ONLY}" -eq 1 ]; then
    info "would roll back to ${target} (--list, nothing changed)"
    continue
  fi

  confirm "send 100% of ${service} traffic from ${current:-latest} to ${target}?"
  apply_cmd gcloud run services update-traffic "${service}" \
    "--to-revisions=${target}=100" \
    --region "${REGION}" --project "${PROJECT_ID}"
  rolled_back=$((rolled_back + 1))
  if [ "${DRY_RUN}" -eq 1 ]; then
    info "dry run: the command above was NOT executed"
  else
    ok "${service} now serves ${target}"
  fi
done

step "summary"
if [ "${LIST_ONLY}" -eq 1 ]; then
  info "listing only, nothing was changed"
elif [ "${DRY_RUN}" -eq 1 ]; then
  info "dry run: ${rolled_back} service(s) would have been rolled back, ${skipped} skipped"
else
  ok "${rolled_back} service(s) rolled back, ${skipped} skipped"
  info "traffic is now pinned to an explicit revision: the next \`make deploy\` puts it"
  info "back on the newest revision (TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST)."
fi
