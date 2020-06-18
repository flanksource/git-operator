package controllers

import (
	"context"

	gitflanksourcecomv1 "github.com/flanksource/git-operator/api/v1"
	"github.com/go-logr/logr"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PullRequestDiff struct {
	client.Client
	Log          logr.Logger
	Repository   *gitflanksourcecomv1.GitRepository
	GithubClient *scm.Client
}

func (p *PullRequestDiff) Merge(ctx context.Context, gh, k8s *gitflanksourcecomv1.GitPullRequest) error {
	if gh != nil && k8s == nil {
		err := p.Client.Create(ctx, gh)
		return errors.Wrapf(err, "failed to create GitPullRequest %s in namespace %s", gh.Name, gh.Namespace)
	} else if gh == nil && k8s != nil {
		err := p.createInGithub(ctx, k8s)
		return errors.Wrapf(err, "failed to create Github PullRequest for GitPullRequest %s in namespace %s", gh.Name, gh.Namespace)
	} else if gh == nil && k8s == nil {
		panic(errors.New("Received both github pull request and k8s pull request as nil values"))
	}

	return nil
}

func (p *PullRequestDiff) createInGithub(ctx context.Context, pr *gitflanksourcecomv1.GitPullRequest) error {
	repoName := pr.Spec.Repository
	prRequest := &scm.PullRequestInput{
		Title: pr.Spec.Title,
		Body:  pr.Spec.Body,
		Base:  pr.Spec.Base,
		Head:  pr.Spec.Head,
	}
	ghPR, _, err := p.GithubClient.PullRequests.Create(ctx, repoName, prRequest)
	if err != nil {
		return errors.Wrap(err, "failed to create PullRequest in Github")
	}
	// TODO: add assignee, reviewers, etc
	_ = ghPR
	return nil
}
