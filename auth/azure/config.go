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

package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	clientIDField                   = "clientId"
	tenantIDField                   = "tenantId"
	clientSecretField               = "clientSecret"
	clientCertificateField          = "clientCertificate"
	clientCertificatePasswordField  = "clientCertificatePassword"
	clientCertificateSendChainField = "clientCertificateSendChain"
	authorityHostField              = "authorityHost"
	accountKeyField                 = "accountKey"
	sasKeyField                     = "sasKey"
)

type Config struct {
	client ctrlClient.Client
	ServiceAccount types.NamespacedName
	Secret *corev1.Secret
}

type ConfigOptFunc func(config *Config)

func NewConfig(opts ...ConfigOptFunc) *Config {
	c := &Config{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func WithServiceAccount(client ctrlClient.Client, saName string, saNs string) ConfigOptFunc {
	return func(config *Config) {
		config.ServiceAccount = types.NamespacedName{
			Namespace: saNs,
			Name: saName,
		}
		config.client = client
	}
}

func WithSecret(secret *corev1.Secret) ConfigOptFunc {
	return func(config *Config) {
		config.Secret = secret
	}
}


func (c *Config) GetCredentials(ctx context.Context) (creds azcore.TokenCredential, err error) {
	if c.ServiceAccount.String() != "" {
		creds, err = GetAzureCredsFromServiceAccount(ctx, c.client, c.ServiceAccount)
		return creds, err
	}

	if c.Secret != nil {
		creds, err = GetAzureCredsFromSecret(c.Secret)
		return creds, err
	}

	return nil, nil
}

func GetAzureCredsFromSecret(secret *corev1.Secret) (azcore.TokenCredential, error) {
	if secret == nil {
		return nil, fmt.Errorf("cannot get az TokenCredential from empty secret")
	}
	err := ValidateSecret(secret)
	if err != nil {
		return nil, err
	}

	return tokenCredentialFromSecret(secret)
}

func GetAzureCredsFromServiceAccount(ctx context.Context, client ctrlClient.Client, nsName types.NamespacedName) (azcore.TokenCredential, error) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: nsName.Name, Namespace: nsName.Namespace},
	}
	if err := client.Get(ctx, types.NamespacedName{Namespace: sa.Namespace, Name: sa.Name}, sa); err != nil {
		return nil, err
	}

	clientID := sa.Annotations["azure.workload.identity/client-id"]
	if clientID == "" {
		return nil, fmt.Errorf("no client id annotation on serviceaccount")
	}
	tenantID := sa.Annotations["azure.workload.identity/tenant-id"]
	if tenantID == "" {
		return nil, fmt.Errorf("no tenamt id annotation on serviceaccount")
	}

	getAssertionToken := func(ctx context.Context) (string, error) {
		tr := &authenticationv1.TokenRequest{}
		if err := client.SubResource("token").Create(ctx, sa, tr); err != nil {
			return "", err
		}

		return tr.Status.Token, nil
	}

	clientCred, err := azidentity.NewClientAssertionCredential(tenantID, clientID, getAssertionToken, nil)
	if err != nil {
		return nil, err
	}


	return clientCred, nil
}

// ValidateSecret validates if the provided Secret does at least have one valid
// set of credentials. The provided Secret may be nil.
func ValidateSecret(secret *corev1.Secret) error {
	if secret == nil {
		return nil
	}

	var valid bool
	if _, hasTenantID := secret.Data[tenantIDField]; hasTenantID {
		if _, hasClientID := secret.Data[clientIDField]; hasClientID {
			if _, hasClientSecret := secret.Data[clientSecretField]; hasClientSecret {
				valid = true
			}
			if _, hasClientCertificate := secret.Data[clientCertificateField]; hasClientCertificate {
				valid = true
			}
		}
	}
	if _, hasClientID := secret.Data[clientIDField]; hasClientID {
		valid = true
	}
	if _, hasAccountKey := secret.Data[accountKeyField]; hasAccountKey {
		valid = true
	}
	if _, hasSasKey := secret.Data[sasKeyField]; hasSasKey {
		valid = true
	}
	if _, hasAuthorityHost := secret.Data[authorityHostField]; hasAuthorityHost {
		valid = true
	}

	if !valid {
		return fmt.Errorf("invalid '%s' secret data: requires a '%s' or '%s' field, a combination of '%s', '%s' and '%s', or '%s', '%s' and '%s'",
			secret.Name, clientIDField, accountKeyField, tenantIDField, clientIDField, clientSecretField, tenantIDField, clientIDField, clientCertificateField)
	}
	return nil
}


// tokenCredentialsFromSecret attempts to create an azcore.TokenCredential
// based on the data fields of the given Secret. It returns, in order:
//   - azidentity.ClientSecretCredential when `tenantId`, `clientId` and
//     `clientSecret` fields are found.
//   - azidentity.ClientCertificateCredential when `tenantId`,
//     `clientCertificate` (and optionally `clientCertificatePassword`) fields
//     are found.
//   - azidentity.ManagedIdentityCredential for a User ID, when a `clientId`
//     field but no `tenantId` is found.
//   - Nil, if no valid set of credential fields was found.
func tokenCredentialFromSecret(secret *corev1.Secret) (azcore.TokenCredential, error) {
	if secret == nil {
		return nil, nil
	}

	clientID, hasClientID := secret.Data[clientIDField]
	if tenantID, hasTenantID := secret.Data[tenantIDField]; hasTenantID && hasClientID {
		if clientSecret, hasClientSecret := secret.Data[clientSecretField]; hasClientSecret && len(clientSecret) > 0 {
			opts := &azidentity.ClientSecretCredentialOptions{}
			if authorityHost, hasAuthorityHost := secret.Data[authorityHostField]; hasAuthorityHost {
				opts.Cloud = cloud.Configuration{ActiveDirectoryAuthorityHost: string(authorityHost)}
			}
			return azidentity.NewClientSecretCredential(string(tenantID), string(clientID), string(clientSecret), opts)
		}
		if clientCertificate, hasClientCertificate := secret.Data[clientCertificateField]; hasClientCertificate && len(clientCertificate) > 0 {
			certs, key, err := azidentity.ParseCertificates(clientCertificate, secret.Data[clientCertificatePasswordField])
			if err != nil {
				return nil, fmt.Errorf("failed to parse client certificates: %w", err)
			}
			opts := &azidentity.ClientCertificateCredentialOptions{}
			if authorityHost, hasAuthorityHost := secret.Data[authorityHostField]; hasAuthorityHost {
				opts.Cloud = cloud.Configuration{ActiveDirectoryAuthorityHost: string(authorityHost)}
			}
			if v, sendChain := secret.Data[clientCertificateSendChainField]; sendChain {
				opts.SendCertificateChain = string(v) == "1" || strings.ToLower(string(v)) == "true"
			}
			return azidentity.NewClientCertificateCredential(string(tenantID), string(clientID), certs, key, opts)
		}
	}
	if hasClientID {
		return azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{
			ID: azidentity.ClientID(clientID),
		})
	}
	return nil, nil
}
