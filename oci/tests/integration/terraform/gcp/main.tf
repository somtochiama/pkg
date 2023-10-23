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
  source = "git::https://github.com/fluxcd/test-infra.git//tf-modules/gcp/gke"

  name      = local.name
  tags      = var.tags
  enable_wi = var.enable_wi
  oauth_scopes = var.enable_wi ? [] : [
    "https://www.googleapis.com/auth/cloud-platform"
  ]
}

module "gcr" {
  source = "git::https://github.com/fluxcd/test-infra.git//tf-modules/gcp/gcr"

  name = local.name
  tags = var.tags
}

resource "google_service_account" "test" {
  count      = var.enable_wi ? 1 : 0
  account_id = "test-workload-id"
  project    = var.gcp_project_id
}

resource "google_project_iam_member" "admin-account-iam" {
  count   = var.enable_wi ? 1 : 0
  project = var.gcp_project_id
  role    = "roles/artifactregistry.repoAdmin"
  member  = "serviceAccount:${google_service_account.test[0].email}"
}

resource "google_project_iam_member" "gcr-account-iam" {
  count   = var.enable_wi ? 1 : 0
  project = var.gcp_project_id
  role    = "roles/containerregistry.ServiceAgent"
  member  = "serviceAccount:${google_service_account.test[0].email}"
}


resource "google_service_account_iam_member" "main" {
  count              = var.enable_wi ? 1 : 0
  service_account_id = google_service_account.test[0].name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.gcp_project_id}.svc.id.goog[${var.wi_k8s_sa_ns}/${var.wi_k8s_sa_name}]"
}
