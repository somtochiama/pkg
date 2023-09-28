package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

// GCP_TOKEN_URL is the default GCP metadata endpoint used for authentication.
const GCP_TOKEN_URL = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"

// Provider is an authentication provider for GCP.
type Provider struct {
	serviceAccount types.NamespacedName
	secret types.NamespacedName
	client ctrlClient.Client

	tokenURL string
	accessToken string
}

type ProviderOptFunc func(*Provider)

func NewProvider(opts ...ProviderOptFunc) *Provider {
	p := &Provider{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func WithTokenURL(tokenURL string) ProviderOptFunc {
	return func(p *Provider) {
		p.tokenURL = tokenURL
	}
}

// ServiceAccountToken is the object returned by the GKE metadata server
// upon requesting for a GCP service account token.
// Ref: https://cloud.google.com/kubernetes-engine/docs/concepts/workload-identity#metadata_server
type ServiceAccountToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// GetWorkloadIdentityToken fetches the token for the service account that the
// Pod is configured to run as, using Workload Identity. The token is fetched by
// reaching out to the GKE metadata server which runs on each node (if Workload
// Identity is enabled). Ref: https://cloud.google.com/kubernetes-engine/docs/concepts/workload-identity
func (p *Provider) GetWorkloadIdentityToken(ctx context.Context) (string, time.Time, error) {
	if p.serviceAccount.String() != "" {
		return GetTokenFromServiceAccount(ctx, p.client, p.serviceAccount)
	}

	if p.tokenURL == "" {
		p.tokenURL = GCP_TOKEN_URL
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.tokenURL, nil)
	if err != nil {
		return "", time.Time{}, err
	}

	request.Header.Add("Metadata-Flavor", "Google")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return "", time.Time{}, err
	}
	defer response.Body.Close()
	defer io.Copy(io.Discard, response.Body)

	if response.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("unexpected status from metadata service: %s", response.Status)
	}

	var accessToken *ServiceAccountToken
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(accessToken); err != nil {
		return "", time.Time{}, err
	}

	return accessToken.AccessToken, time.Now().Add(time.Duration(accessToken.ExpiresIn) * time.Second), nil
}
