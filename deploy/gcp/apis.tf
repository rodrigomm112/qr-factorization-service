# ---- APIs every other resource depends on -----------------------------------
# Enabling is idempotent and takes minutes on a fresh project. disable_on_destroy = false:
# `terraform destroy` drops this deployment, not project-wide APIs others may use.

locals {
  required_apis = [
    "cloudresourcemanager.googleapis.com",
    "iam.googleapis.com",
    "artifactregistry.googleapis.com",
    "secretmanager.googleapis.com",
    "run.googleapis.com",
  ]
}

resource "google_project_service" "required" {
  for_each = toset(local.required_apis)

  project = var.project_id
  service = each.value

  disable_on_destroy         = false
  disable_dependent_services = false
}
