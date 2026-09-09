# ---- Terraform and provider pinning -----------------------------------------
# One root module, LOCAL state: a single operator, and a remote backend needs a bucket that
# must itself be created somewhere. The state holds jwt_secret and auth_client_secret_sha256,
# so deploy/gcp/terraform.tfstate is as sensitive as .env and is gitignored.

terraform {
  required_version = ">= 1.16.0"

  # Shared state and locking, once a second applier exists. Create the bucket with object
  # versioning first (the only undo for a corrupted state), then `terraform init
  # -migrate-state`.
  # backend "gcs" {
  #   bucket = "proyectot-tfstate-CHANGE_ME"
  #   prefix = "cloud-run"
  # }

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 8.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# Project number: builds the deterministic Cloud Run URLs (see locals in cloudrun.tf).
data "google_project" "this" {
  project_id = var.project_id
}
