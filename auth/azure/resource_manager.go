package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	"k8s.io/apimachinery/pkg/types"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Provider is an authentication provider for Azure.
type Provider struct {
	serviceAccount types.NamespacedName
	secret types.NamespacedName
	client ctrlClient.Client

	credential azcore.TokenCredential
	scopes     []string
}

type ProviderOption func(*Provider)

func NewProvider(opts ...ProviderOption) *Provider {
	p := &Provider{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func WithCredential(cred azcore.TokenCredential) ProviderOption {
	return func(p *Provider) {
		p.credential = cred
	}
}

func WithSecret(secret types.NamespacedName) ProviderOption {
	return func(p *Provider) {
		p.secret = secret
	}
}

func WithServiceAccount(sa types.NamespacedName) ProviderOption {
	return func(p *Provider) {
		p.serviceAccount = sa
	}
}

func WithClient(client ctrlClient.Client) ProviderOption {
	return func(p *Provider) {
		p.client = client
	}
}


func WithAzureGovtScope() ProviderOption {
	return func(p *Provider) {
		p.scopes = []string{cloud.AzureGovernment.Services[cloud.ResourceManager].Endpoint + "/" + ".default"}
	}
}

func WithAzureChinaScope() ProviderOption {
	return func(p *Provider) {
		p.scopes = []string{cloud.AzureChina.Services[cloud.ResourceManager].Endpoint + "/" + ".default"}
	}
}

// GetResourceManagerToken fetches the Azure Resource Manager token using the
// credential chain, secret or service account that the provider is configured with,
// in that order. If none of these are configured,
// credential chain is constructed using the default method, which includes
// trying to use Workload Identity, Managed Identity, etc.
// By default, the scope of the request targets the Azure Public cloud, but this
// is configurable using WithAzureGovtScope or WithAzureChinaScope.
func (p *Provider) GetResourceManagerToken(ctx context.Context) (*azcore.AccessToken, error) {
	if p.credential == nil && p.secret.String() != "" {
		cred, err := getAzureCredsFromSecret(ctx, p.client, p.secret)
		if err != nil {
			return nil, err
		}
		p.credential = cred
	}

	if p.credential == nil && p.serviceAccount.String() != "" {
		cred, err := getAzureCredsFromServiceAccount(ctx, p.client, p.serviceAccount)
		if err != nil {
			return nil, err
		}
		p.credential = cred
	}

	if p.credential == nil {
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, err
		}
		p.credential = cred
	}
	if len(p.scopes) == 0 {
		p.scopes = []string{cloud.AzurePublic.Services[cloud.ResourceManager].Endpoint + "/" + ".default"}
	}

	accessToken, err := p.credential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: p.scopes,
	})
	if err != nil {
		return nil, err
	}

	return &accessToken, nil
}
