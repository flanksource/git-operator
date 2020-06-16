package controllers

import (
	"context"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrGithubFieldMissing is returned when spec.github is missing
	ErrGithubFieldMissing = errors.New("Github field in type spec is missing")
	// ErrProviderNotFoundInSecret is returned if PROVIDER=github is not present in credentials secret
	ErrProviderNotFoundInSecret = errors.New("PROVIDER field not found in credentials secret")
	// ErrGithubTokenNotFoundInSecret is returned if GITHUB_TOKEN field is not present in credentials secret
	ErrGithubTokenNotFoundInSecret = errors.New("GITHUB_TOKEN field not found in credentials secret")
	// ErrProviderNotSupported is returned when PROVIDER field in credentials secret does not match any known provider
	ErrProviderNotSupported = errors.New("PROVIDER not supported, valid providers are: github")
)

type RepositoryCredentials struct {
	Provider    string
	GithubToken string
}

func getRepositoryCredentials(ctx context.Context, k8s client.Client, secretName, namespace string) (*RepositoryCredentials, error) {
	secret := &v1.Secret{}
	if err := k8s.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
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
		return &RepositoryCredentials{Provider: string(provider), GithubToken: string(githubToken)}, nil
	}

	return nil, ErrProviderNotSupported
}
