package connectors

import (
	"context"

	gitv1 "github.com/flanksource/git-operator/api/v1"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Connector interface {
	ReconcileBranches(ctx context.Context, repository *gitv1.GitRepository) error
	ReconcilePullRequests(ctx context.Context, repository *gitv1.GitRepository) error
}

func NewConnector(ctx context.Context, crdClient client.Client, k8sClient *kubernetes.Clientset, log logr.Logger, repository *gitv1.GitRepository) (Connector, error) {
	var secretName string
	var secretNamespace string

	if repository.Spec.Github != nil {
		secretName = repository.Spec.Github.SecretRef.Name
		secretNamespace = repository.Spec.Github.SecretRef.Namespace
	} else if repository.Spec.GitSSH != nil {
		secretName = repository.Spec.GitSSH.SecretRef.Name
		secretNamespace = repository.Spec.GitSSH.SecretRef.Namespace
	} else {
		return nil, errors.New("no connector settings found")
	}

	secret, err := k8sClient.CoreV1().Secrets(secretNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get secret %s in namespace %s", secretName, secretNamespace)
	}

	if repository.Spec.Github != nil {
		githubToken, found := secret.Data["GITHUB_TOKEN"]
		if !found {
			return nil, ErrGithubTokenNotFoundInSecret
		}
		return NewGithub(crdClient, k8sClient, log, string(githubToken))
	}

	return nil, errors.New("no connector settings found")
}
