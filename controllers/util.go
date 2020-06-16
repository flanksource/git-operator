package controllers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jenkins-x/go-scm/scm"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	gitflanksourcecomv1 "github.com/flanksource/git-operator/api/v1"
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
	Provider  string
	AuthToken string
}

// +kubebuilder:rbac:groups="",namespace=system,resources=secrets,verbs=get;list;watch

func getRepositoryCredentials(ctx context.Context, k8s *kubernetes.Clientset, secretName, namespace string) (*RepositoryCredentials, error) {
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

func getRepositoryName(r gitflanksourcecomv1.GitRepository) string {
	if r.Spec.Github == nil {
		return ""
	}
	return fmt.Sprintf("%s/%s", r.Spec.Github.Owner, r.Spec.Github.Repository)
}

func pullRequestName(repository string, number int) string {
	return fmt.Sprintf("%s-%d", repository, number)
}

func buildPullRequestCRDsFromGithub(prs []*scm.PullRequest, repository gitflanksourcecomv1.GitRepository) []gitflanksourcecomv1.GitPullRequest {
	crdPRs := []gitflanksourcecomv1.GitPullRequest{}

	for _, pr := range prs {
		crdPRs = append(crdPRs, buildPullRequestCRDFromGithub(pr, repository))
	}

	return crdPRs
}

func buildPullRequestCRDFromGithub(pr *scm.PullRequest, repository gitflanksourcecomv1.GitRepository) gitflanksourcecomv1.GitPullRequest {
	repositoryName := getRepositoryName(repository)

	reviewers := make([]string, len(pr.Reviewers))
	for i, reviewer := range pr.Reviewers {
		reviewers[i] = reviewer.Login
	}

	crd := gitflanksourcecomv1.GitPullRequest{
		ObjectMeta: metav1.ObjectMeta{Name: pullRequestName(repositoryName, pr.Number), Namespace: repository.Namespace},
		Spec: gitflanksourcecomv1.GitPullRequestSpec{
			Repository: repositoryName,
			ID:         strconv.Itoa(pr.Number),
			SHA:        pr.Sha,
			Ref:        pr.Ref,
			Title:      pr.Title,
			Fork:       pr.Fork,
			Reviewers:  reviewers,
		},
		Status: gitflanksourcecomv1.GitPullRequestStatus{
			URL:    pr.Link,
			Author: pr.Author.Login,
		},
	}

	return crd
}
