package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	gitflanksourcecomv1 "github.com/flanksource/git-operator/api/v1"
	"github.com/jenkins-x/go-scm/scm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type GithubFetcher struct {
	client     *scm.Client
	repository gitflanksourcecomv1.GitRepository
}

func (g *GithubFetcher) BuildPRCRDsFromGithub(ctx context.Context, lastUpdated time.Time) ([]gitflanksourcecomv1.GitPullRequest, error) {
	crdPRs := []gitflanksourcecomv1.GitPullRequest{}
	repoName := getRepositoryName(g.repository)

	prs, _, err := g.client.PullRequests.List(ctx, repoName, scm.PullRequestListOptions{UpdatedAfter: &lastUpdated})
	if err != nil {
		return nil, err
	}

	for _, pr := range prs {
		prCrd, err := g.BuildPRCRDFromGithub(ctx, pr, lastUpdated)
		if err != nil {
			return nil, err
		}
		crdPRs = append(crdPRs, *prCrd)
	}

	return crdPRs, nil
}

func (g *GithubFetcher) BuildPRCRDFromGithub(ctx context.Context, pr *scm.PullRequest, lastUpdated time.Time) (*gitflanksourcecomv1.GitPullRequest, error) {
	repositoryName := getRepositoryName(g.repository)
	reviewers := []string{}
	approvers := map[string]bool{}

	reviews, _, err := g.client.Reviews.List(ctx, repositoryName, pr.Number, scm.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, r := range reviews {
		reviewers = append(reviewers, r.Author.Login)
		approvers[r.Author.Login] = r.State == "APPROVED"
	}

	head := pr.Source
	if pr.Fork != repositoryName {
		head = fmt.Sprintf("%s:%s", strings.Split(pr.Fork, "/")[0], head)
	}

	crd := gitflanksourcecomv1.GitPullRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pullRequestName(g.repository.Name, pr.Number),
			Namespace: g.repository.Namespace,
			Labels: map[string]string{
				"git.flanksource.com/repository": g.repository.Name,
			},
		},
		Spec: gitflanksourcecomv1.GitPullRequestSpec{
			Repository: repositoryName,
			ID:         strconv.Itoa(pr.Number),
			SHA:        pr.Sha,
			Ref:        pr.Ref,
			Head:       head,
			Body:       pr.Body,
			Base:       pr.Target,
			Title:      pr.Title,
			Fork:       pr.Fork,
			Reviewers:  reviewers,
		},
		Status: gitflanksourcecomv1.GitPullRequestStatus{
			URL:       pr.Link,
			Author:    pr.Author.Login,
			Approvers: approvers,
		},
	}

	return &crd, nil
}

func (g *GithubFetcher) BuildBranchCRDsFromGithub(ctx context.Context, lastUpdated time.Time) ([]gitflanksourcecomv1.GitBranch, error) {
	crdBranches := []gitflanksourcecomv1.GitBranch{}
	repoName := getRepositoryName(g.repository)

	branches, _, err := g.client.Git.ListBranches(ctx, repoName, scm.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, branch := range branches {
		branchCrd, err := g.BuildBranchCRDFromGithub(ctx, branch, lastUpdated)
		if err != nil {
			return nil, err
		}
		crdBranches = append(crdBranches, *branchCrd)
	}

	return crdBranches, nil
}

func (g *GithubFetcher) BuildBranchCRDFromGithub(ctx context.Context, branch *scm.Reference, lastUpdated time.Time) (*gitflanksourcecomv1.GitBranch, error) {
	repositoryName := getRepositoryName(g.repository)

	crd := gitflanksourcecomv1.GitBranch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      branchName(g.repository.Name, branch.Name),
			Namespace: g.repository.Namespace,
			Labels: map[string]string{
				"git.flanksource.com/repository": g.repository.Name,
				"git.flanksource.com/branch":     branch.Name,
			},
		},
		Spec: gitflanksourcecomv1.GitBranchSpec{
			Repository: repositoryName,
			BranchName: branch.Name,
		},
		Status: gitflanksourcecomv1.GitBranchStatus{
			LastUpdated: metav1.Now(),
			Head:        branch.Sha,
		},
	}

	return &crd, nil
}
