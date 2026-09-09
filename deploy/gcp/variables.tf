# Inputs. Only project_id and the two secrets have no default; the rest mirrors
# .env.example, so the same image behaves the same under compose and under Cloud Run.

variable "project_id" {
  description = "GCP project that owns every resource. Billing must be enabled."
  type        = string
}

variable "region" {
  description = "Cloud Run and Artifact Registry region."
  type        = string
  default     = "us-central1"
}

variable "image_tag" {
  description = "HUMAN LABEL of the build (the deploy script uses the short git SHA). It is written to each service's `labels` and used to build the image reference ONLY when image_refs is empty; what actually runs is image_refs."
  type        = string
  default     = "latest"
}

variable "image_refs" {
  description = <<-EOT
    Digest-pinned image references keyed by application (qr_api, stats_api, web),
    e.g. "us-central1-docker.pkg.dev/<project>/proyectot/qr-api@sha256:<64 hex>".
    scripts/deploy-gcp.sh fills this from `docker inspect` right after the push, so
    the revision Cloud Run creates is byte-for-byte the image that was built and
    scanned — a tag can be moved under our feet, a digest cannot.
    Empty (the default) falls back to `<registry>/<app>:<image_tag>`, the reference
    the bootstrap apply and `terraform plan` need before any image exists.
  EOT
  type        = map(string)
  default     = {}

  validation {
    condition     = alltrue([for key in keys(var.image_refs) : contains(["qr_api", "stats_api", "web"], key)])
    error_message = "image_refs may only use the keys qr_api, stats_api and web."
  }

  validation {
    condition     = alltrue([for reference in values(var.image_refs) : can(regex("^[^@]+@sha256:[0-9a-f]{64}$", reference))])
    error_message = "every image_refs value must be a full digest reference: <repo>/<image>@sha256:<64 lowercase hex>."
  }
}

variable "artifact_repo" {
  description = "Artifact Registry Docker repository id."
  type        = string
  default     = "proyectot"
}

variable "service_names" {
  description = "Cloud Run service names, keyed by application."
  type        = map(string)
  default = {
    qr_api    = "qr-api"
    stats_api = "stats-api"
    web       = "web"
  }

  validation {
    condition     = alltrue([for k in ["qr_api", "stats_api", "web"] : contains(keys(var.service_names), k)])
    error_message = "service_names must define the keys qr_api, stats_api and web."
  }
}

# ---- secrets: passed as TF_VAR_*, never written to a .tfvars file -----------

variable "jwt_secret" {
  description = "HS256 signing key shared by both APIs (>= 32 bytes). Pass it as TF_VAR_jwt_secret, never in a .tfvars file."
  type        = string
  sensitive   = true
}

variable "auth_client_id" {
  description = "Client id accepted by POST /api/v1/auth/token."
  type        = string
  default     = "demo-client"
}

variable "auth_client_secret_sha256" {
  description = "Lowercase hex SHA-256 of the demo client secret. The plaintext is never stored anywhere."
  type        = string
  sensitive   = true

  validation {
    condition     = can(regex("^[0-9a-f]{64}$", var.auth_client_secret_sha256))
    error_message = "auth_client_secret_sha256 must be 64 lowercase hexadecimal characters."
  }
}

# ---- scaling and limits -----------------------------------------------------

variable "min_instances" {
  description = "Minimum Cloud Run instances. 0 = scale to zero (free tier, cold starts)."
  type        = number
  default     = 0
}

variable "max_instances" {
  description = "Maximum Cloud Run instances. A low ceiling is the cost guardrail of a demo."
  type        = number
  default     = 3
}

variable "cpu_limit" {
  description = "CPU limit per container."
  type        = string
  default     = "1"
}

variable "memory_limit" {
  description = "Memory limit per container."
  type        = string
  default     = "512Mi"
}

variable "max_concurrency" {
  description = "Maximum concurrent requests per instance."
  type        = number
  default     = 80
}

variable "request_timeout" {
  description = "Cloud Run request timeout. Must exceed 2 x STATS_API_TIMEOUT (the worst case with one retry)."
  type        = string
  default     = "60s"
}

# ---- application configuration (mirrors .env.example) -----------------------

variable "app_env" {
  description = "APP_ENV for both APIs."
  type        = string
  default     = "production"
}

variable "log_level" {
  description = "LOG_LEVEL for both APIs."
  type        = string
  default     = "info"
}

variable "jwt_issuer" {
  description = "`iss` claim minted by qr-api and required by both verifiers."
  type        = string
  default     = "qr-api"
}

variable "jwt_ttl" {
  description = "Access-token lifetime (Go duration)."
  type        = string
  default     = "1h"
}

variable "stats_api_timeout" {
  description = "Per-attempt timeout of the qr-api -> stats-api call (Go duration)."
  type        = string
  default     = "5s"
}

variable "stats_api_retries" {
  description = "Extra idempotent retries after the first attempt."
  type        = number
  default     = 1
}

variable "max_matrix_rows" {
  description = "MAX_MATRIX_ROWS accepted by qr-api."
  type        = number
  default     = 100
}

variable "max_matrix_cols" {
  description = "MAX_MATRIX_COLS accepted by qr-api."
  type        = number
  default     = 100
}

variable "qr_max_body_bytes" {
  description = "qr-api request body limit in bytes (1 MiB)."
  type        = number
  default     = 1048576
}

variable "stats_max_body_bytes" {
  description = "stats-api request body limit in bytes (4 MiB)."
  type        = number
  default     = 4194304
}

variable "qr_rate_limit_max" {
  description = "Requests per window per IP on qr-api /api/v1/*."
  type        = number
  default     = 60
}

variable "qr_rate_limit_window" {
  description = "Rate-limit window for qr-api (Go duration)."
  type        = string
  default     = "1m"
}

variable "auth_rate_limit_max" {
  description = "Requests per window per IP on POST /api/v1/auth/token."
  type        = number
  default     = 10
}

variable "stats_rate_limit_max" {
  description = "Requests per window per IP on stats-api."
  type        = number
  default     = 120
}

variable "stats_rate_limit_window_ms" {
  description = "Rate-limit window for stats-api, in milliseconds."
  type        = number
  default     = 60000
}

variable "diagonal_abs_tolerance" {
  description = "Absolute term of the hybrid isDiagonal tolerance."
  type        = string
  default     = "1e-12"
}

variable "diagonal_rel_tolerance" {
  description = "Relative term of the hybrid isDiagonal tolerance."
  type        = string
  default     = "1e-9"
}

variable "max_matrices" {
  description = "Maximum matrices per stats-api request."
  type        = number
  default     = 8
}

variable "max_total_elements" {
  description = "Maximum sum of rows*cols across a stats-api request."
  type        = number
  default     = 100000
}

variable "shutdown_timeout" {
  description = "Grace period for in-flight requests on SIGTERM (Go duration)."
  type        = string
  default     = "10s"
}

variable "extra_cors_origins" {
  description = "Additional origins allowed by both APIs on top of the deployed web URL (e.g. http://localhost:5173 while developing against the cloud)."
  type        = list(string)
  default     = []
}

variable "request_budget" {
  description = "qr-api: total per-request deadline (decomposition + stats call + retry). Must be >= STATS_API_TIMEOUT x (1 + STATS_API_RETRIES) and <= the 15 s write timeout."
  type        = string
  default     = "12s"
}

variable "shutdown_timeout_ms" {
  description = "stats-api: drain deadline on SIGTERM before the forced exit, in milliseconds."
  type        = number
  default     = 10000
}

variable "demo_token_enabled" {
  description = "qr-api: expose POST /api/v1/auth/demo-token so the public SPA runs without a client secret."
  type        = bool
  default     = true
}
