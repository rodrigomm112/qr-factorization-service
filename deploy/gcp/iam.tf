# ---- runtime identities -----------------------------------------------------
# One service account per service instead of the default Compute Engine one, which carries
# roles/editor project-wide. None holds a project-level role: their only grant is the
# per-secret accessor binding in secrets.tf, and web has none at all.

resource "google_service_account" "qr_api" {
  project      = var.project_id
  account_id   = "qr-api-sa"
  display_name = "Proyecto T — qr-api runtime"
  description  = "Runtime identity of the qr-api Cloud Run service. Reads JWT_SECRET and AUTH_CLIENT_SECRET_SHA256."

  depends_on = [google_project_service.required]
}

resource "google_service_account" "stats_api" {
  project      = var.project_id
  account_id   = "stats-api-sa"
  display_name = "Proyecto T — stats-api runtime"
  description  = "Runtime identity of the stats-api Cloud Run service. Reads JWT_SECRET only (it verifies, it never signs)."

  depends_on = [google_project_service.required]
}

resource "google_service_account" "web" {
  project      = var.project_id
  account_id   = "web-sa"
  display_name = "Proyecto T — web runtime"
  description  = "Runtime identity of the static frontend. Holds no permission at all: nginx only serves files."

  depends_on = [google_project_service.required]
}

# ---- public invocation ------------------------------------------------------
# allUsers on all three: web is a public SPA, qr-api the public API entry point, and
# stats-api is called directly by the browser ("recompute in Node", health badge), so it
# cannot be ingress-internal. The boundary is the JWT, not the network.

resource "google_cloud_run_v2_service_iam_member" "public" {
  for_each = {
    qr_api    = google_cloud_run_v2_service.qr_api.name
    stats_api = google_cloud_run_v2_service.stats_api.name
    web       = google_cloud_run_v2_service.web.name
  }

  project  = var.project_id
  location = var.region
  name     = each.value
  role     = "roles/run.invoker"
  member   = "allUsers"
}
