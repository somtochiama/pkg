variable "rand" {
  type = string
}

variable "tags" {
  type    = map(string)
  default = {}
}

variable "cross_region" {
  type        = string
  description = "different region for testing cross region resources"
}

variable "k8s_serviceaccount_name" {
  type        = string
  default     = "test"
  description = "Name of kubernetes service account that can assume the IAM role"
}

variable "k8s_serviceaccount_ns" {
  type        = string
  default     = "default"
  description = "Namespace of kubernetes service account that can assume the IAM rolet"
}

variable "enable_wi" {
  type        = bool
  default     = false
  description = "If set to true, will creat IAM role and policy for workload identity"
}
