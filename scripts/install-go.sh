#!/usr/bin/env bash
# install-go.sh — toolchain installer, no sudo, everything under ~/.local. Idempotent.
#
#   Go            -> ~/.local/opt/go        (sha256 pinned below)
#   Terraform     -> ~/.local/bin           (verified against the published SHA256SUMS)
#   golangci-lint -> ~/.local/go/bin        (go install, version pinned)
#   govulncheck   -> ~/.local/go/bin        (go install, version pinned)
#
# Also writes ~/.local/etc/dev-toolchain.sh with the GOROOT/GOPATH/PATH exports.
set -euo pipefail

GO_VERSION="1.27.1"
GO_SHA256_LINUX_AMD64="63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445"
TERRAFORM_VERSION="1.16.1"
GOLANGCI_LINT_VERSION="v2.13.2"
GOVULNCHECK_VERSION="v1.8.0"

LOCAL_PREFIX="${HOME}/.local"
GOROOT_DIR="${LOCAL_PREFIX}/opt/go"
GOPATH_DIR="${LOCAL_PREFIX}/go"
BIN_DIR="${LOCAL_PREFIX}/bin"
ETC_DIR="${LOCAL_PREFIX}/etc"
TOOLCHAIN_FILE="${ETC_DIR}/dev-toolchain.sh"
WORK_DIR="$(mktemp -d)"

trap 'rm -rf "${WORK_DIR}"' EXIT

log()  { printf '\033[36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[warn]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[31m[error]\033[0m %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "required tool not found: $1"; }

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) echo "amd64" ;;
    aarch64 | arm64) echo "arm64" ;;
    *) die "unsupported architecture: $(uname -m)" ;;
  esac
}

download() { # download <url> <dest>
  if command -v curl >/dev/null 2>&1; then
    curl --fail --location --silent --show-error --output "$2" "$1"
  else
    wget --quiet --output-document "$2" "$1"
  fi
}

need uname
need mktemp
need sha256sum
command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 || die "need curl or wget"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(detect_arch)"
mkdir -p "${GOROOT_DIR%/go}" "${GOPATH_DIR}" "${BIN_DIR}" "${ETC_DIR}"

# ---- 1. Go ----
install_go() {
  if [ -x "${GOROOT_DIR}/bin/go" ] && "${GOROOT_DIR}/bin/go" version | grep -q "go${GO_VERSION} "; then
    log "Go ${GO_VERSION} already installed at ${GOROOT_DIR}"
    return 0
  fi

  local tarball="go${GO_VERSION}.${OS}-${ARCH}.tar.gz"
  local url="https://go.dev/dl/${tarball}"
  local dest="${WORK_DIR}/${tarball}"
  local expected="${GO_SHA256:-}"

  if [ -z "${expected}" ]; then
    if [ "${OS}" = "linux" ] && [ "${ARCH}" = "amd64" ]; then
      expected="${GO_SHA256_LINUX_AMD64}"
    else
      die "no pinned sha256 for ${OS}-${ARCH}; export GO_SHA256=<sha256 from https://go.dev/dl/> and re-run"
    fi
  fi

  log "downloading ${url}"
  download "${url}" "${dest}"

  log "verifying sha256"
  printf '%s  %s\n' "${expected}" "${dest}" | sha256sum --check --status \
    || die "sha256 mismatch for ${tarball} (expected ${expected})"

  log "extracting into ${GOROOT_DIR}"
  rm -rf "${GOROOT_DIR}"
  tar -C "${GOROOT_DIR%/go}" -xzf "${dest}"
  "${GOROOT_DIR}/bin/go" version
}

# ---- 2. Terraform ----
install_terraform() {
  if [ -x "${BIN_DIR}/terraform" ] && "${BIN_DIR}/terraform" version | head -n 1 | grep -q "v${TERRAFORM_VERSION}"; then
    log "Terraform ${TERRAFORM_VERSION} already installed at ${BIN_DIR}/terraform"
    return 0
  fi
  if ! command -v unzip >/dev/null 2>&1 && ! command -v python3 >/dev/null 2>&1; then
    warn "neither unzip nor python3 available; skipping Terraform (only needed for 'make deploy')"
    return 0
  fi

  local base="https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}"
  local zip="terraform_${TERRAFORM_VERSION}_${OS}_${ARCH}.zip"
  local sums="terraform_${TERRAFORM_VERSION}_SHA256SUMS"

  log "downloading ${base}/${zip}"
  download "${base}/${zip}" "${WORK_DIR}/${zip}"
  download "${base}/${sums}" "${WORK_DIR}/${sums}"

  log "verifying sha256 against ${sums}"
  ( cd "${WORK_DIR}" && grep " ${zip}\$" "${sums}" | sha256sum --check --status ) \
    || die "sha256 mismatch for ${zip}"

  log "installing terraform into ${BIN_DIR}"
  if command -v unzip >/dev/null 2>&1; then
    unzip -o -q "${WORK_DIR}/${zip}" terraform -d "${WORK_DIR}"
  else
    python3 -c 'import sys,zipfile; zipfile.ZipFile(sys.argv[1]).extract("terraform", sys.argv[2])' \
      "${WORK_DIR}/${zip}" "${WORK_DIR}"
  fi
  install -m 0755 "${WORK_DIR}/terraform" "${BIN_DIR}/terraform"
  "${BIN_DIR}/terraform" version | head -n 1
}

# ---- 3. Go-based linters ----
install_go_tools() {
  export GOROOT="${GOROOT_DIR}"
  export GOPATH="${GOPATH_DIR}"
  export GOBIN="${GOPATH_DIR}/bin"
  export PATH="${GOROOT}/bin:${GOBIN}:${PATH}"

  if [ -x "${GOBIN}/golangci-lint" ] && "${GOBIN}/golangci-lint" --version | grep -q "${GOLANGCI_LINT_VERSION#v}"; then
    log "golangci-lint ${GOLANGCI_LINT_VERSION} already installed"
  else
    log "go install golangci-lint ${GOLANGCI_LINT_VERSION}"
    go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"
  fi

  if [ -x "${GOBIN}/govulncheck" ]; then
    log "govulncheck already installed"
  else
    log "go install govulncheck ${GOVULNCHECK_VERSION}"
    go install "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}"
  fi
}

# ---- 4. Environment file, sourced by `make setup` and by ~/.bashrc ----
write_toolchain_file() {
  if [ -f "${TOOLCHAIN_FILE}" ]; then
    log "${TOOLCHAIN_FILE} already exists, leaving it untouched"
    return 0
  fi
  log "writing ${TOOLCHAIN_FILE}"
  cat > "${TOOLCHAIN_FILE}" <<'PROFILE'
# Written by proyectoT/scripts/install-go.sh — source it from ~/.bashrc:
#   [ -f "$HOME/.local/etc/dev-toolchain.sh" ] && . "$HOME/.local/etc/dev-toolchain.sh"
export GOROOT="$HOME/.local/opt/go"
export GOPATH="$HOME/.local/go"
export PATH="$GOROOT/bin:$GOPATH/bin:$HOME/.local/bin:$PATH"
PROFILE
  chmod 0644 "${TOOLCHAIN_FILE}"
}

install_go
install_terraform
install_go_tools
write_toolchain_file

log "done. Add this to your shell profile if it is not there yet:"
printf '    [ -f "%s" ] && . "%s"\n' "${TOOLCHAIN_FILE}" "${TOOLCHAIN_FILE}"
