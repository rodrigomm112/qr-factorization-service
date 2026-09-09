# =============================================================================
# What the deploy script and the operator need after an apply.
# =============================================================================

output "qr_api_url" {
  description = "Public URL of qr-api (Swagger UI at /docs). Deterministic form, see cloudrun.tf."
  value       = local.service_urls.qr_api
}

output "stats_api_url" {
  description = "Public URL of stats-api (Swagger UI at /docs). Deterministic form."
  value       = local.service_urls.stats_api
}

output "web_url" {
  description = "Public URL of the frontend. Deterministic form; this is the origin in both APIs' CORS allow-list."
  value       = local.service_urls.web
}

output "legacy_urls" {
  description = "The hash-based hostnames the provider reports as `.uri`; they serve the same services and are informational only."
  value = {
    qr_api    = google_cloud_run_v2_service.qr_api.uri
    stats_api = google_cloud_run_v2_service.stats_api.uri
    web       = google_cloud_run_v2_service.web.uri
  }
}

output "artifact_registry" {
  description = "Docker repository the deploy script pushes to."
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${var.artifact_repo}"
}

output "image_refs" {
  description = "The image reference each service is pinned to. `@sha256:` means the deploy script supplied the digest; a `:tag` means this is a bootstrap apply and the tag was resolved by Cloud Run."
  value       = local.images
}

output "image_tag" {
  description = "Human label of the build (git short SHA), also written to each service's labels. Informational: what runs is image_refs."
  value       = var.image_tag
}

output "deployed_images" {
  description = "What the current revision of each service actually runs, read back from the API. Compare with image_refs after an apply."
  value = {
    qr_api    = google_cloud_run_v2_service.qr_api.template[0].containers[0].image
    stats_api = google_cloud_run_v2_service.stats_api.template[0].containers[0].image
    web       = google_cloud_run_v2_service.web.template[0].containers[0].image
  }
}

output "latest_revisions" {
  description = "Latest ready revision per service as recorded in the state (it can lag one revision behind the live service until the next apply); `scripts/rollback-gcp.sh --list` reads the live list from gcloud."
  value = {
    qr_api    = google_cloud_run_v2_service.qr_api.latest_ready_revision
    stats_api = google_cloud_run_v2_service.stats_api.latest_ready_revision
    web       = google_cloud_run_v2_service.web.latest_ready_revision
  }
}

output "service_accounts" {
  description = "Runtime identities, one per service."
  value = {
    qr_api    = google_service_account.qr_api.email
    stats_api = google_service_account.stats_api.email
    web       = google_service_account.web.email
  }
}
