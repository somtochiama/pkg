package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
)

// Provider is an authentication provider for AWS.
type Provider struct {
	optFns []func(*config.LoadOptions) error
}

type ProviderOptFunc func(*Provider)

func NewProvider(opts ...ProviderOptFunc) *Provider {
	p := &Provider{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func WithRegion(region string) ProviderOptFunc {
	return func(p *Provider) {
		p.optFns = append(p.optFns, config.WithRegion(region))
	}
}

func (p *Provider) WithOptFns(optFns []func(*config.LoadOptions) error) {
	p.optFns = append(p.optFns, optFns...)
}

// GetConfig returns the default config constructed using any options that the
// provider was configured with. If OIDC/IRSA has been configured for the EKS
// cluster, then the config object will also be configured with the necessary
// credentials. The returned config object can be used to fetch tokens to access
// particular AWS services.
func (p *Provider) GetConfig(ctx context.Context) (config.Config, error) {
	cfg, err := config.LoadDefaultConfig(ctx, p.optFns...)
	return cfg, err
}
