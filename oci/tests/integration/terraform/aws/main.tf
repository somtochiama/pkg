provider "aws" {}

provider "aws" {
  alias  = "cross_region"
  region = var.cross_region
}

locals {
  name = "flux-test-${var.rand}"
}

module "eks" {
  source = "git::https://github.com/fluxcd/test-infra.git//tf-modules/aws/eks"

  name = local.name
  tags = var.tags
}

module "test_ecr" {
  source = "git::https://github.com/fluxcd/test-infra.git//tf-modules/aws/ecr"

  name = "test-repo-${local.name}"
  tags = var.tags
}

module "test_ecr_cross_reg" {
  source = "git::https://github.com/fluxcd/test-infra.git//tf-modules/aws/ecr"

  name = "test-repo-${local.name}-cross-reg"
  tags = var.tags
  providers = {
    aws = aws.cross_region
  }
}

module "test_app_ecr" {
  source = "git::https://github.com/fluxcd/test-infra.git//tf-modules/aws/ecr"

  name = "test-app-${local.name}"
  tags = var.tags
}

resource "aws_iam_role" "assume_role" {
  count = var.enable_wi ? 1 : 0
  name  = "test-wi-ecr"
  assume_role_policy = templatefile("oidc_assume_role_policy.json", {
    OIDC_ARN  = module.eks.cluster_oidc_arn, OIDC_URL = replace(module.eks.cluster_oidc_url, "https://", ""),
    NAMESPACE = var.k8s_serviceaccount_ns, SA_NAME = var.k8s_serviceaccount_name
  })
  tags = var.tags
}

resource "aws_iam_role_policy_attachment" "aws_node" {
  count      = var.enable_wi ? 1 : 0
  role       = aws_iam_role.assume_role[0].name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
  depends_on = [aws_iam_role.assume_role]
}
