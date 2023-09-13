package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// Provider is an authentication provider for Azure.
type Provider struct {
	credential azcore.TokenCredential
	scopes     []string
}

type ProviderOptFunc func(*Provider)

func NewProvider(opts ...ProviderOptFunc) *Provider {
	p := &Provider{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func WithCredential(cred azcore.TokenCredential) ProviderOptFunc {
	return func(p *Provider) {
		p.credential = cred
	}
}

func WithAzureGovtScope() ProviderOptFunc {
	return func(p *Provider) {
		p.scopes = []string{cloud.AzureGovernment.Services[cloud.ResourceManager].Endpoint + "/" + ".default"}
	}
}

func WithAzureChinaScope() ProviderOptFunc {
	return func(p *Provider) {
		p.scopes = []string{cloud.AzureChina.Services[cloud.ResourceManager].Endpoint + "/" + ".default"}
	}
}

// GetResourceManagerToken fetches the Azure Resource Manager token using the
// credential that the provider is configured with. If it isn't, then a new
// credential chain is constructed using the default method, which includes
// trying to use Workload Identity, Managed Identity, etc.
// By default, the scope of the request targets the Azure Public cloud, but this
// is configurable using WithAzureGovtScope or WithAzureChinaScope.
func (p *Provider) GetResourceManagerToken(ctx context.Context) (*azcore.AccessToken, error) {
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
