provider "google" {
  project = var.gcp_project_id
  region  = var.gcp_region
  zone    = var.gcp_zone
}

resource "random_pet" "suffix" {}

locals {
  name = "flux-test-${random_pet.suffix.id}"
}

module "gke" {
  source = "git::https://github.com/somtochiama/test-infra.git//tf-modules/gcp/gke?ref=gcp-workload-id"

  name = local.name
  tags = var.tags
  workload_id = true
  oauth_scopes = []
}

module "gcr" {
  source = "git::https://github.com/fluxcd/test-infra.git//tf-modules/gcp/gcr"

  name = local.name
  tags = var.tags
}

resource "google_service_account" "workload-identity-user-sa" {
  account_id   = "workload-id"
  display_name = "Service Account For Workload Identity"
}

resource "google_project_iam_member" "acr-storage-role" {
  project = var.gcp_project_id
  role = "roles/artifactregistry.admin"
  member = "serviceAccount:${google_service_account.workload-identity-user-sa.email}"
}

resource "google_project_iam_member" "storage-role" {
  project = var.gcp_project_id
  role = "roles/containerregistry.ServiceAgent"
  member = "serviceAccount:${google_service_account.workload-identity-user-sa.email}"
}

resource "google_project_iam_member" "workload_identity-role" {
  project = var.gcp_project_id
  role   = "roles/iam.workloadIdentityUser"
  member = "serviceAccount:${var.gcp_project_id}.svc.id.goog[default/test]"
}