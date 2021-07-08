package connectors

import (
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	// ErrGithubFieldMissing is returned when spec.github is missing
	ErrGithubFieldMissing = errors.New("Github field in type spec is missing")
	// ErrProviderNotFoundInSecret is returned if PROVIDER=github is not present in credentials secret
	ErrProviderNotFoundInSecret = errors.New("PROVIDER field not found in credentials secret")
	// ErrGithubTokenNotFoundInSecret is returned if GITHUB_TOKEN field is not present in credentials secret
	ErrGithubTokenNotFoundInSecret = errors.New("GITHUB_TOKEN field not found in credentials secret")
	// ErrAzureDevopsTokenNotFoundInSecret is returned if AZURE_DEVOPS_TOKEN field is not present in credentials secret
	ErrAzureDevopsTokenNotFoundInSecret = errors.New("AZURE_DEVOPS_TOKEN field not found in credentials secret")
	// ErrProviderNotSupported is returned when PROVIDER field in credentials secret does not match any known provider
	ErrProviderNotSupported = errors.New("PROVIDER not supported, valid providers are: github")
	// ErrSSHUserNotFoundInSecret is returned when SSH_USER is not present in credentials secret
	ErrSSHUserNotFoundInSecret = errors.New("SSH_USER field not found in credentials secret")
	// ErrSSHPrivateKeyNotFoundInSecret is returned when SSH_PRIVATE_KEY is not present in credentials secret
	ErrSSHPrivateKeyNotFoundInSecret = errors.New("SSH_PRIVATE_KEY field not found in credentials secret")
	// ErrGitSSHURLIsEmpty is returned when gitSSH.url is not present
	ErrGitSSHURLIsEmpty = errors.New("GitSSH url was not provided")
)

type RepositoryCredentials struct {
	Provider  string
	AuthToken string
}

// +kubebuilder:rbac:groups="",namespace=system,resources=secrets,verbs=get;list;watch

func GetRepositoryCredentials(ctx context.Context, k8s *kubernetes.Clientset, secretName, namespace string) (*RepositoryCredentials, error) {
	secret, err := k8s.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get secret %s in namespace %s", secretName, namespace)
	}

	provider, found := secret.Data["PROVIDER"]
	if !found {
		return nil, ErrProviderNotFoundInSecret
	}
	if string(provider) == "github" {
		githubToken, found := secret.Data["GITHUB_TOKEN"]
		if !found {
			return nil, ErrGithubTokenNotFoundInSecret
		}
		return &RepositoryCredentials{Provider: string(provider), AuthToken: string(githubToken)}, nil
	}

	return nil, ErrProviderNotSupported
}
