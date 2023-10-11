variable "azure_location" {
  type    = string
  default = "eastus"
}

variable "tags" {
  type    = map(string)
  default = {}
}

variable "k8s_serviceaccount_name" {
  type        = string
  default     = "test"
  description = "Name of kubernetes service account to establish federated identity with"
}

variable "k8s_serviceaccount_ns" {
  type        = string
  default     = "default"
  description = "Namespace of kubernetes service account to establish federated identity with"
}

variable "enable_wi" {
  type        = bool
  default     = false
  description = "Enable workload identity on cluster and create federated identity"
}
