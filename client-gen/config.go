package client_gen

import (
	"context"
	"fmt"
	"strings"

	"github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubeConfig struct {
	SecretRef meta.LocalObjectReference
}

type AuthInfo struct {
	ServiceAccount string
	Username       string
	KubeConfig     *KubeConfig
	Namespace      string
}

func getServiceAccountToken(ctx context.Context, client client.Client, info AuthInfo) (string, error) {
	namespacedName := types.NamespacedName{
		Namespace: info.Namespace,
		Name:      info.ServiceAccount,
	}

	var serviceAccount corev1.ServiceAccount
	err := client.Get(ctx, namespacedName, &serviceAccount)
	if err != nil {
		return "", err
	}

	secretName := types.NamespacedName{
		Namespace: info.Namespace,
		Name:      info.ServiceAccount,
	}

	for _, secret := range serviceAccount.Secrets {
		if strings.HasPrefix(secret.Name, fmt.Sprintf("%s-token", serviceAccount.Name)) {
			secretName.Name = secret.Name
			break
		}
	}

	var secret corev1.Secret
	err = client.Get(ctx, secretName, &secret)
	if err != nil {
		return "", err
	}

	var token string
	if data, ok := secret.Data["token"]; ok {
		token = string(data)
	} else {
		return "", fmt.Errorf("the service account secret '%s' does not containt a token", secretName.String())
	}

	return token, nil
}

func getImpersonatedConfig(config *rest.Config, username string, namespace string) *rest.Config {
	config.Impersonate = rest.ImpersonationConfig{
		UserName: username,
		Groups:   []string{"flux:users", "flux:users:" + namespace},
	}

	return config
}

func GetKubeConfig(ctx context.Context, client client.Client, secretName types.NamespacedName) ([]byte, error) {
	var secret corev1.Secret
	if err := client.Get(ctx, secretName, &secret); err != nil {
		return nil, fmt.Errorf("unable to read KubeConfig secret '%s' error: %w", secretName.String(), err)
	}

	kubeConfig, ok := secret.Data["value"]
	if !ok {
		return nil, fmt.Errorf("KubeConfig secret '%s' doesn't contain a 'value' key ", secretName.String())
	}

	return kubeConfig, nil
}

func SetImpersonationOnConfig(ctx context.Context, client client.Client, config *rest.Config, authInfo AuthInfo, tokenImp bool) (*rest.Config, error) {
	var username string
	namespace := authInfo.Namespace

	// TODO(somtochiama): error out if both user and kubeconfig is set?

	if authInfo.ServiceAccount != "" {
		if tokenImp {
			token, err := getServiceAccountToken(ctx, client, authInfo)
			if err != nil {
				return nil, err
			}
			config.BearerToken = token
			config.BearerTokenFile = ""

			return config, nil
		}

		username = fmt.Sprintf("system:serviceaccount:%s:%s", namespace, authInfo.ServiceAccount)
	}

	if authInfo.Username != "" {
		username = fmt.Sprintf("flux:user:%s:%s", namespace, authInfo.Username)
	}

	// Sets default username if both service account and user name is unset
	if username == "" {
		username = fmt.Sprintf("flux:user:%s:%s", namespace, "reconciler")
	}

	return getImpersonatedConfig(config, username, namespace), nil
}
