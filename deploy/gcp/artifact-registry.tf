# ---- one Docker repository for the three images -----------------------------
# Free tier is 0.5 GB and every deploy pushes three tags, so cleanup keeps the two most
# recent versions per package (current + rollback target) and drops the rest after 30 days;
# KEEP outranks DELETE. Durations in seconds: "30d" comes back normalized, as a forever diff.

resource "google_artifact_registry_repository" "docker" {
  project       = var.project_id
  location      = var.region
  repository_id = var.artifact_repo
  description   = "Proyecto T container images (qr-api, stats-api, web)"
  format        = "DOCKER"

  cleanup_policy_dry_run = false

  cleanup_policies {
    id     = "keep-two-most-recent"
    action = "KEEP"

    most_recent_versions {
      keep_count = 2
    }
  }

  cleanup_policies {
    id     = "delete-untagged"
    action = "DELETE"

    condition {
      tag_state = "UNTAGGED"
    }
  }

  cleanup_policies {
    id     = "delete-older-than-30d"
    action = "DELETE"

    condition {
      older_than = "2592000s"
    }
  }

  depends_on = [google_project_service.required]
}
