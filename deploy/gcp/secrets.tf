# ---- Secret Manager ---------------------------------------------------------
# JWT_SECRET (HS256, qr-api signs, both verify) and AUTH_CLIENT_SECRET_SHA256,
# reaching the containers through `value_source.secret_key_ref`: never baked into an image,
# never in a revision's plaintext config. They do land in the state, hence TF_VAR_*.

resource "google_secret_manager_secret" "jwt_secret" {
  project   = var.project_id
  secret_id = "proyectot-jwt-secret"

  replication {
    auto {}
  }

  depends_on = [google_project_service.required]
}

resource "google_secret_manager_secret_version" "jwt_secret" {
  secret      = google_secret_manager_secret.jwt_secret.id
  secret_data = var.jwt_secret
}

resource "google_secret_manager_secret" "auth_client_secret_sha256" {
  project   = var.project_id
  secret_id = "proyectot-auth-client-secret-sha256"

  replication {
    auto {}
  }

  depends_on = [google_project_service.required]
}

resource "google_secret_manager_secret_version" "auth_client_secret_sha256" {
  secret      = google_secret_manager_secret.auth_client_secret_sha256.id
  secret_data = var.auth_client_secret_sha256
}

# ---- per-secret accessor bindings -------------------------------------------
# qr-api signs tokens and checks client credentials, stats-api only verifies, web reads none.

resource "google_secret_manager_secret_iam_member" "qr_api_jwt_secret" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.jwt_secret.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.qr_api.email}"
}

resource "google_secret_manager_secret_iam_member" "stats_api_jwt_secret" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.jwt_secret.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.stats_api.email}"
}

resource "google_secret_manager_secret_iam_member" "qr_api_client_secret" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.auth_client_secret_sha256.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.qr_api.email}"
}
