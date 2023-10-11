variable "gcp_project_id" {
  type = string
}

variable "gcp_region" {
  type    = string
  default = "us-central1"
}

variable "gcp_zone" {
  type    = string
  default = "us-central1-c"
}

variable "gcr_region" {
  type    = string
  default = "" // Empty default to use gcr.io.
}

variable "tags" {
  type    = map(string)
  default = {}
}

variable "k8s_serviceaccount_name" {
  type        = string
  default     = "test"
  description = "Name of kubernetes service account to be bound to GCP IAM serviceaccount"
}

variable "k8s_serviceaccount_ns" {
  type        = string
  default     = "default"
  description = "Namespace of kubernetes service account to be bound to GCP IAM serviceaccount"
}

variable "enable_wi" {
  type        = bool
  default     = false
  description = "Enable workload identity on cluster and create a federated identity with serviceaccount"
}
