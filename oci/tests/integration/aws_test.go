//go:build integration
// +build integration

/*
Copyright 2022 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

import (
	"context"
	"fmt"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/fluxcd/test-infra/tftestenv"
)

const (
	roleArnAnnotation = "eks.amazonaws.com/role-arn"
)

// createKubeconfigEKS constructs kubeconfig from the terraform state output at
// the given kubeconfig path.
func createKubeconfigEKS(ctx context.Context, state map[string]*tfjson.StateOutput, kcPath string) error {
	clusterName := state["eks_cluster_name"].Value.(string)
	eksHost := state["eks_cluster_endpoint"].Value.(string)
	eksClusterArn := state["eks_cluster_arn"].Value.(string)
	eksCa := state["eks_cluster_ca_certificate"].Value.(string)
	return tftestenv.CreateKubeconfigEKS(ctx, clusterName, eksHost, eksClusterArn, eksCa, kcPath)
}

// registryLoginECR logs into the container/artifact registries using the
// provider's CLI tools and returns a list of test repositories.
func registryLoginECR(ctx context.Context, output map[string]*tfjson.StateOutput) (map[string]string, error) {
	// NOTE: ECR provides pre-existing registry per account. It requires
	// repositories to be created explicitly using their API before pushing
	// image.
	testRepos := map[string]string{}
	region := output["region"].Value.(string)

	testRepoURL := output["ecr_repository_url"].Value.(string)
	if err := tftestenv.RegistryLoginECR(ctx, region, testRepoURL); err != nil {
		return nil, err
	}
	testRepos["ecr"] = testRepoURL

	// test the cross-region repository
	cross_region := output["cross_region"].Value.(string)
	testCrossRepo := output["ecr_cross_region_repository_url"].Value.(string)
	if err := tftestenv.RegistryLoginECR(ctx, cross_region, testCrossRepo); err != nil {
		return nil, err
	}
	testRepos["ecr_cross_region"] = testCrossRepo

	// Log into the test app repository to be able to push to it.
	// This image is not used in testing and need not be included in
	// testRepos.
	ircRepoURL := output["ecr_test_app_repo_url"].Value.(string)
	if err := tftestenv.RegistryLoginECR(ctx, region, ircRepoURL); err != nil {
		return nil, err
	}

	return testRepos, nil
}

// pushAppTestImagesECR pushes test app image that is being tested. It must be
// called after registryLoginECR to ensure the local docker client is already
// logged in and is capable of pushing the test images.
func pushAppTestImagesECR(ctx context.Context, localImgs map[string]string, output map[string]*tfjson.StateOutput) (map[string]string, error) {
	// Get the registry name and construct the image names accordingly.
	repo := output["ecr_test_app_repo_url"].Value.(string)
	remoteImage := repo + ":test"
	return tftestenv.PushTestAppImagesECR(ctx, localImgs, remoteImage)
}

// getServiceAccountAnnotationAWS returns annotations for a kubernetes service account required to configure IRSA on AWS.
// It gets the role ARN from the terraform output and returns the map[eks.amazonaws.com/role-arn=<arn>]
func getServiceAccountAnnotationAWS(output map[string]*tfjson.StateOutput) (map[string]string, error) {
	iamARN := output["aws_iam_arn"].Value.(string)
	if iamARN == "" {
		return nil, fmt.Errorf("no AWS iam role arn in terraform output")
	}

	return map[string]string{
		roleArnAnnotation: iamARN,
	}, nil
}
