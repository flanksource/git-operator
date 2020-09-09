package connectors

import (
	"context"
	"strings"

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
	secretName := repository.Spec.SecretRef.Name
	secretNamespace := repository.Namespace

	secret, err := k8sClient.CoreV1().Secrets(secretNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get secret %s in namespace %s", secretName, secretNamespace)
	}

	url := repository.Spec.URL

	if strings.HasPrefix(url, "https://github.com/") {
		path := url[19:]
		parts := strings.Split(path, "/")
		if len(parts) != 2 {
			return nil, errors.Errorf("invalid repository url: %s", url)
		}
		owner := parts[0]
		repoName := parts[1]
		githubToken, found := secret.Data["GITHUB_TOKEN"]
		if !found {
			return nil, ErrGithubTokenNotFoundInSecret
		}
		return NewGithub(crdClient, log, owner, repoName, string(githubToken))
	} else if strings.HasPrefix(url, "ssh://") {
		sshURL := url[6:]
		user := strings.Split(sshURL, "@")[0]

		privateKey, found := secret.Data["SSH_PRIVATE_KEY"]
		if !found {
			return nil, ErrSSHPrivateKeyNotFoundInSecret
		}
		password, found := secret.Data["SSH_PRIVATE_KEY_PASSWORD"]
		if !found {
			password = []byte{}
		}
		return NewGitSSH(crdClient, log, sshURL, string(user), privateKey, string(password))
	}

	return nil, errors.New("no connector settings found")
}
