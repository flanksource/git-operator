package connectors

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	gitv1 "github.com/flanksource/git-operator/api/v1"
	"github.com/jenkins-x/go-scm/scm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type GithubFetcher struct {
	client     *scm.Client
	repository gitv1.GitRepository
	owner      string
	repoName   string
}

func (g *GithubFetcher) BuildPRCRDsFromGithub(ctx context.Context, lastUpdated time.Time) ([]gitv1.GitPullRequest, error) {
	var crdPRs []gitv1.GitPullRequest
	repoName := g.repositoryName()

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

func (g *GithubFetcher) BuildPRCRDFromGithub(ctx context.Context, pr *scm.PullRequest, lastUpdated time.Time) (*gitv1.GitPullRequest, error) {
	repositoryName := g.repositoryName()
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

	crd := gitv1.GitPullRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.pullRequestName(g.repository.Name, pr.Number),
			Namespace: g.repository.Namespace,
			Labels: map[string]string{
				"git.flanksource.com/repository": g.repository.Name,
			},
		},
		Spec: gitv1.GitPullRequestSpec{
			Repository: repositoryName,
			SHA:        pr.Sha,
			Head:       head,
			Body:       pr.Body,
			Base:       pr.Target,
			Title:      pr.Title,
			Fork:       pr.Fork,
			Reviewers:  reviewers,
		},
		Status: gitv1.GitPullRequestStatus{
			ID:        strconv.Itoa(pr.Number),
			URL:       pr.Link,
			Ref:       pr.Ref,
			Author:    pr.Author.Login,
			Approvers: approvers,
		},
	}

	return &crd, nil
}

func (g *GithubFetcher) BuildBranchCRDsFromGithub(ctx context.Context, lastUpdated time.Time) ([]gitv1.GitBranch, error) {
	crdBranches := []gitv1.GitBranch{}
	repoName := g.repositoryName()

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

func (g *GithubFetcher) BuildBranchCRDFromGithub(ctx context.Context, branch *scm.Reference, lastUpdated time.Time) (*gitv1.GitBranch, error) {
	repositoryName := g.repositoryName()

	crd := gitv1.GitBranch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.branchName(g.repository.Name, branch.Name),
			Namespace: g.repository.Namespace,
			Labels: map[string]string{
				"git.flanksource.com/repository": g.repository.Name,
				"git.flanksource.com/branch":     branch.Name,
			},
		},
		Spec: gitv1.GitBranchSpec{
			Repository: repositoryName,
			BranchName: branch.Name,
		},
		Status: gitv1.GitBranchStatus{
			LastUpdated: metav1.Now(),
			Head:        branch.Sha,
		},
	}

	return &crd, nil
}

func (g *GithubFetcher) BuildDeploymentCRDsFromGithub(ctx context.Context, lastUpdated time.Time) ([]gitv1.GitDeployment, error) {
	crdDeployments := []gitv1.GitDeployment{}
	repoName := g.repositoryName()

	deployments, _, err := g.client.Deployments.List(ctx, repoName, scm.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, deployment := range deployments {
		deploymentCrd := g.BuildDeploymentCRDFromGithub(ctx, deployment)
		if err != nil {
			return nil, err
		}
		crdDeployments = append(crdDeployments, *deploymentCrd)
	}

	return crdDeployments, nil
}

func (g *GithubFetcher) BuildDeploymentCRDFromGithub(ctx context.Context, deployment *scm.Deployment) *gitv1.GitDeployment {
	crd := &gitv1.GitDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.deploymentName(g.repository.Name, deployment.Name, deployment.Ref),
			Namespace: g.repository.Namespace,
			Labels: map[string]string{
				"git.flanksource.com/repository": g.repository.Name,
				"git.flanksource.com/deployment": deployment.Name,
			},
		},
		Spec: gitv1.GitDeploymentSpec{
			Ref:         deployment.Ref,
			Sha:         deployment.Sha,
			Name:        deployment.Name,
			ID:          deployment.ID,
			Environment: deployment.Environment,
			Description: deployment.Description,
		},
		Status: gitv1.GitDeploymentStatus{
			Ref:            deployment.Ref,
			Sha:            deployment.Sha,
			DeploymentLink: deployment.Link,
			StatusLink:     deployment.StatusLink,
			ID:             deployment.ID,
			Name:           deployment.Name,
			Environment:    deployment.Environment,
		},
	}
	return crd
}

func (g *GithubFetcher) repositoryName() string {
	return fmt.Sprintf("%s/%s", g.owner, g.repoName)
}

func (g *GithubFetcher) branchName(repository string, name string) string {
	return fmt.Sprintf("%s-%s", repository, name)
}

func (g *GithubFetcher) deploymentName(repository, name, ref string) string {
	return fmt.Sprintf("%s-%s-%s", repository, name, ref)
}

func (g *GithubFetcher) pullRequestName(repository string, number int) string {
	return fmt.Sprintf("%s-%d", repository, number)
}
