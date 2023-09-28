package aws

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/aws/aws-sdk-go-v2/config"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	roleAnnotation = "eks.amazonaws.com/role-arn"
)

// Provider is an authentication provider for AWS.
type Provider struct {
	optFns []func(*config.LoadOptions) error

	serviceAccount types.NamespacedName
	client ctrlClient.Client

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

func WithServiceAccount(sa types.NamespacedName) ProviderOptFunc {
	return func(p *Provider) {
		p.serviceAccount = sa
	}
}

func WithControllerClient(client ctrlClient.Client) ProviderOptFunc {
	return func(p *Provider) {
		p.client = client
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
	if p.serviceAccount.String() != "" {
		assumeProvider, err := GetConfigProviderFromSA(ctx, p.client, p.serviceAccount)
		if err != nil {
			return nil, err
		}
		p.optFns = append(p.optFns, assumeProvider)
	}

	cfg, err := config.LoadDefaultConfig(ctx, p.optFns...)
	return cfg, err
}

func GetConfigProviderFromSA(ctx context.Context, client ctrlClient.Client, nsName types.NamespacedName) (func(*config.LoadOptions) error, error) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: nsName.Name, Namespace: nsName.Namespace},
	}
	if err := client.Get(ctx, types.NamespacedName{Namespace: sa.Namespace, Name: sa.Name}, sa); err != nil {
		return nil, err
	}

	roleARN := sa.Annotations[roleAnnotation]
	if roleARN == "" {
		return nil, fmt.Errorf("no `eks.amazonaws.com/role-arn` annotation on serviceaccount")
	}

	region, err := getRegionFromIMDS(ctx)
	if err != nil {
		return nil, err
	}
	tokFetch := NewTokenFetcher(client, sa)
	webIdProvider := stscreds.NewWebIdentityRoleProvider(sts.New(sts.Options{
		Region: region,
	}), roleARN, tokFetch)
	return config.WithCredentialsProvider(webIdProvider), nil
}

type tokenFetcher struct {
	client ctrlClient.Client
	sa     *corev1.ServiceAccount
}

func NewTokenFetcher(client ctrlClient.Client, sa *corev1.ServiceAccount) *tokenFetcher {
	return &tokenFetcher{
		client: client,
		sa:     sa,
	}
}

func (t *tokenFetcher) GetIdentityToken() ([]byte, error) {
	tr := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences: []string{"sts.amazonaws.com"},
		},
	}
	// should we simply return a token hg
	if err := t.client.SubResource("token").Create(context.Background(), t.sa, tr); err != nil {
		return nil, err
	}

	return []byte(tr.Status.Token), nil
}

func getRegionFromIMDS(ctx context.Context) (string, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return "", err
	}

	imdsClient := imds.NewFromConfig(cfg)
	response, err := imdsClient.GetRegion(ctx, &imds.GetRegionInput{})
	if err != nil {
		return "", err
	}
	return response.Region, err
}

