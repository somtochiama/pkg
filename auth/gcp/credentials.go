/*
Copyright 2023 The Flux authors

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

package gcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"cloud.google.com/go/compute/metadata"
	credentials "cloud.google.com/go/iam/credentials/apiv1"
	"cloud.google.com/go/iam/credentials/apiv1/credentialspb"
	"github.com/googleapis/gax-go/v2"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/oauth"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	serviceAccountAnnotation = "iam.gke.io/gcp-service-account"
)

func GetTokenFromServiceAccount(ctx context.Context, client ctrlClient.Client, nsName types.NamespacedName) (string, time.Time, error) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: nsName.Name, Namespace: nsName.Namespace},
	}
	if err := client.Get(ctx, types.NamespacedName{Namespace: sa.Namespace, Name: sa.Name}, sa); err != nil {
		return "", time.Time{}, err
	}

	gcpSAName := sa.Annotations[serviceAccountAnnotation]
	if gcpSAName == "" {
		return "", time.Time{}, fmt.Errorf("no `%s` annotation on serviceaccount", serviceAccountAnnotation)
	}

	// exchange oidc token for identity binding token
	idPool, idProvider, err := getDetailsFromMetadataService()
	if err != nil {
		return "", time.Time{}, err
	}

	tr := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences: []string{idPool},
		},
	}
	if err := client.SubResource("token").Create(ctx, sa, tr); err != nil {
		return "", time.Time{}, err
	}

	accessToken, err := tradeIDBindToken(ctx, tr.Status.Token, idPool, idProvider)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("error exchanging token: '%s'", err)
	}
	// exchange identity binding token for iam token
	iamClient, err := credentials.NewIamCredentialsClient(ctx)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("error creating iam client: '%s'", err)
	}

	saResponse, err := iamClient.GenerateAccessToken(ctx, &credentialspb.GenerateAccessTokenRequest{
		Name: fmt.Sprintf("projects/-/serviceAccounts/%s", gcpSAName),
		Scope: []string{
			"https://www.googleapis.com/auth/cloud-platform",
		},
	}, gax.WithGRPCOptions(grpc.PerRPCCredentials(oauth.TokenSource{TokenSource: oauth2.StaticTokenSource(accessToken)})))

	if err != nil {
		return "", time.Time{}, fmt.Errorf("error exchanging access token w gcp iam: '%w'", err)
	}

	return saResponse.GetAccessToken(), saResponse.GetExpireTime().AsTime(), nil
}

func getDetailsFromMetadataService() (string, string, error) {
	projectID, err := metadata.ProjectID()
	if err != nil {
		return "", "", fmt.Errorf("unable to get project id from metadata server: '%s'", err)
	}

	location, err := metadata.InstanceAttributeValue("cluster-location")
	if err != nil {
		return "", "", fmt.Errorf("unable to get cluster location from metadata server: '%s'", err)
	}

	clusterName, err := metadata.InstanceAttributeValue("cluster-name")
	if err != nil {
		return "", "", fmt.Errorf("unable to get cluster name from metadata server: '%s'", err)
	}

	idProvider := fmt.Sprintf("https://container.googleapis.com/v1/projects/%s/locations/%s/clusters/%s",
		projectID, location, clusterName)

	idPool := fmt.Sprintf("%s.svc.id.goog", projectID)
	return idPool, idProvider, nil
}

// Copied from: https://github.com/GoogleCloudPlatform/secrets-store-csi-driver-provider-gcp/blob/053d18c0a8fe522d5acea547b22b97a04ac7134d/auth/auth.go#L269C1-L307C2
func tradeIDBindToken(ctx context.Context, k8sToken, idPool, idProvider string) (*oauth2.Token, error) {
	body, err := json.Marshal(map[string]string{
		"grant_type":           "urn:ietf:params:oauth:grant-type:token-exchange",
		"subject_token_type":   "urn:ietf:params:oauth:token-type:jwt",
		"requested_token_type": "urn:ietf:params:oauth:token-type:access_token",
		"subject_token":        k8sToken,
		"audience":             fmt.Sprintf("identitynamespace:%s:%s", idPool, idProvider),
		"scope":                "https://www.googleapis.com/auth/cloud-platform",
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://securetoken.googleapis.com/v1/identitybindingtoken", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not get idbindtoken token, status: %v", resp.StatusCode)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	idBindToken := &oauth2.Token{}
	if err := json.Unmarshal(respBody, idBindToken); err != nil {
		return nil, err
	}
	return idBindToken, nil
}
