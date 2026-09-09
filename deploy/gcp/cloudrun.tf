# ---- URLs -------------------------------------------------------------------
# Services answer at a deterministic hostname, <service>-<project number>.<region>.run.app,
# and at the legacy hash-based one the provider exposes as `.uri`. Outputs, README and smoke
# test use the deterministic form: computable from the project number, so web's origin can go
# in both APIs' CORS and the API origins in web's config with no qr-api <-> web cycle.

# ---- PORT -------------------------------------------------------------------
# Cloud Run injects PORT from `ports.container_port` and rejects an explicit PORT env var.

locals {
  registry = "${var.region}-docker.pkg.dev/${var.project_id}/${var.artifact_repo}"

  image_names = {
    qr_api    = "qr-api"
    stats_api = "stats-api"
    web       = "web"
  }

  # Digest first: deploy-gcp.sh passes the digest of each image it just pushed, so the
  # revision is the exact bytes built and scanned — a tag can be moved between push and
  # apply. `:tag` is the fallback for the bootstrap apply and for `terraform plan`.
  images = {
    for key, name in local.image_names :
    key => lookup(var.image_refs, key, "${local.registry}/${name}:${var.image_tag}")
  }

  # GCP label values accept [a-z0-9_-]{0,63}; fold instead of failing the apply.
  image_tag_label = substr(lower(replace(var.image_tag, "/[^a-zA-Z0-9_-]/", "-")), 0, 63)

  common_labels = {
    app        = "proyectot"
    managed-by = "terraform"
    image-tag  = local.image_tag_label
  }

  service_urls = {
    for key, name in var.service_names :
    key => "https://${name}-${data.google_project.this.number}.${var.region}.run.app"
  }

  # Both web hostnames are allowed, deterministic and `.uri`, so a visitor arriving through
  # either gets a working SPA. Plus whatever the operator adds explicitly. No wildcard.
  cors_allowed_origins = join(",", concat([local.service_urls.web, google_cloud_run_v2_service.web.uri], var.extra_cors_origins))

  # ---- environment, mirroring .env.example ---------------------------------

  # The one `.uri` left is STATS_API_BASE_URL below; it also orders creation (stats-api first).
  qr_api_env = {
    APP_ENV              = var.app_env
    LOG_LEVEL            = var.log_level
    JWT_ISSUER           = var.jwt_issuer
    JWT_TTL              = var.jwt_ttl
    JWT_AUDIENCE         = "${var.service_names.qr_api},${var.service_names.stats_api}"
    AUTH_CLIENT_ID       = var.auth_client_id
    STATS_API_BASE_URL   = google_cloud_run_v2_service.stats_api.uri
    STATS_API_TIMEOUT    = var.stats_api_timeout
    STATS_API_RETRIES    = tostring(var.stats_api_retries)
    REQUEST_BUDGET       = var.request_budget
    DEMO_TOKEN_ENABLED   = tostring(var.demo_token_enabled)
    MAX_MATRIX_ROWS      = tostring(var.max_matrix_rows)
    MAX_MATRIX_COLS      = tostring(var.max_matrix_cols)
    MAX_BODY_BYTES       = tostring(var.qr_max_body_bytes)
    RATE_LIMIT_MAX       = tostring(var.qr_rate_limit_max)
    RATE_LIMIT_WINDOW    = var.qr_rate_limit_window
    AUTH_RATE_LIMIT_MAX  = tostring(var.auth_rate_limit_max)
    CORS_ALLOWED_ORIGINS = local.cors_allowed_origins
    SHUTDOWN_TIMEOUT     = var.shutdown_timeout
    # Cloud Run sets X-Forwarded-For; without this the rate limiter sees one front-end IP.
    TRUST_PROXY = "true"
  }

  # Plain names; docker-compose maps STATS_* -> these, one .env feeding both services.
  stats_api_env = {
    APP_ENV                = var.app_env
    LOG_LEVEL              = var.log_level
    JWT_ISSUER             = var.jwt_issuer
    JWT_AUDIENCE           = var.service_names.stats_api
    CORS_ALLOWED_ORIGINS   = local.cors_allowed_origins
    MAX_BODY_BYTES         = tostring(var.stats_max_body_bytes)
    RATE_LIMIT_MAX         = tostring(var.stats_rate_limit_max)
    RATE_LIMIT_WINDOW_MS   = tostring(var.stats_rate_limit_window_ms)
    DIAGONAL_ABS_TOLERANCE = var.diagonal_abs_tolerance
    DIAGONAL_REL_TOLERANCE = var.diagonal_rel_tolerance
    MAX_MATRICES           = tostring(var.max_matrices)
    MAX_TOTAL_ELEMENTS     = tostring(var.max_total_elements)
    SHUTDOWN_TIMEOUT_MS    = tostring(var.shutdown_timeout_ms)
    MAX_MATRIX_ROWS        = tostring(var.max_matrix_rows)
    MAX_MATRIX_COLS        = tostring(var.max_matrix_cols)
    TRUST_PROXY            = "true"
  }

  # Resolved by the browser, so public API URLs; nginx bakes them into config.js at start.
  web_env = {
    QR_API_BASE_URL    = local.service_urls.qr_api
    STATS_API_BASE_URL = local.service_urls.stats_api
  }
}

# ---- stats-api — no dependency of its own, so it is created first -----------
resource "google_cloud_run_v2_service" "stats_api" {
  project             = var.project_id
  name                = var.service_names.stats_api
  location            = var.region
  description         = "Proyecto T — descriptive statistics over N matrices (Node 24 + Express 5)"
  ingress             = "INGRESS_TRAFFIC_ALL" # the browser calls it directly; see iam.tf
  deletion_protection = false
  labels              = merge(local.common_labels, { component = "stats-api" })

  template {
    service_account                  = google_service_account.stats_api.email
    timeout                          = var.request_timeout
    max_instance_request_concurrency = var.max_concurrency

    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    containers {
      image = local.images.stats_api

      ports {
        container_port = 8080
      }

      resources {
        limits = {
          cpu    = var.cpu_limit
          memory = var.memory_limit
        }
        # Bill CPU only while a request is in flight: with min_instances = 0, idle is free.
        cpu_idle          = true
        startup_cpu_boost = true
      }

      dynamic "env" {
        for_each = local.stats_api_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.jwt_secret.secret_id
            version = "latest"
          }
        }
      }

      startup_probe {
        initial_delay_seconds = 0
        period_seconds        = 3
        timeout_seconds       = 3
        failure_threshold     = 10

        http_get {
          path = "/health/live"
          port = 8080
        }
      }
    }
  }

  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }

  depends_on = [
    google_project_service.required,
    google_secret_manager_secret_iam_member.stats_api_jwt_secret,
    google_secret_manager_secret_version.jwt_secret,
  ]
}

# ---- qr-api — public entry point; needs stats-api's real URL ----------------
resource "google_cloud_run_v2_service" "qr_api" {
  project             = var.project_id
  name                = var.service_names.qr_api
  location            = var.region
  description         = "Proyecto T — Householder QR factorization + orchestration (Go 1.27 + Fiber v3)"
  ingress             = "INGRESS_TRAFFIC_ALL"
  deletion_protection = false
  labels              = merge(local.common_labels, { component = "qr-api" })

  template {
    service_account                  = google_service_account.qr_api.email
    timeout                          = var.request_timeout
    max_instance_request_concurrency = var.max_concurrency

    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    containers {
      image = local.images.qr_api

      ports {
        container_port = 8080
      }

      resources {
        limits = {
          cpu    = var.cpu_limit
          memory = var.memory_limit
        }
        cpu_idle          = true
        startup_cpu_boost = true
      }

      dynamic "env" {
        for_each = local.qr_api_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.jwt_secret.secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "AUTH_CLIENT_SECRET_SHA256"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.auth_client_secret_sha256.secret_id
            version = "latest"
          }
        }
      }

      # /health/live never touches stats-api, so a cold downstream cannot block startup.
      # Readiness, which does probe it, is what the e2e script polls.
      startup_probe {
        initial_delay_seconds = 0
        period_seconds        = 3
        timeout_seconds       = 3
        failure_threshold     = 10

        http_get {
          path = "/health/live"
          port = 8080
        }
      }
    }
  }

  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }

  depends_on = [
    google_project_service.required,
    google_secret_manager_secret_iam_member.qr_api_jwt_secret,
    google_secret_manager_secret_iam_member.qr_api_client_secret,
    google_secret_manager_secret_version.jwt_secret,
    google_secret_manager_secret_version.auth_client_secret_sha256,
  ]
}

# ---- web — static SPA, the same image as `docker compose` runs --------------
resource "google_cloud_run_v2_service" "web" {
  project             = var.project_id
  name                = var.service_names.web
  location            = var.region
  description         = "Proyecto T — React frontend served by nginx-unprivileged"
  ingress             = "INGRESS_TRAFFIC_ALL"
  deletion_protection = false
  labels              = merge(local.common_labels, { component = "web" })

  template {
    service_account                  = google_service_account.web.email
    timeout                          = var.request_timeout
    max_instance_request_concurrency = var.max_concurrency

    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    containers {
      image = local.images.web

      ports {
        container_port = 8080
      }

      resources {
        limits = {
          cpu    = var.cpu_limit
          memory = "256Mi"
        }
        cpu_idle          = true
        startup_cpu_boost = true
      }

      dynamic "env" {
        for_each = local.web_env
        content {
          name  = env.key
          value = env.value
        }
      }

      # A static site has no /health/live; "/" also proves the entrypoint wrote config.js.
      startup_probe {
        initial_delay_seconds = 0
        period_seconds        = 3
        timeout_seconds       = 3
        failure_threshold     = 10

        http_get {
          path = "/"
          port = 8080
        }
      }
    }
  }

  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }

  depends_on = [google_project_service.required]
}
