package connectors

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	gitv1 "github.com/flanksource/git-operator/api/v1"
	"github.com/go-logr/logr"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PullRequest struct {
	Repository string
	Title      string
	Body       string
	Base       string
	Head       string
	Reviewers  []string
}

type PullRequestDiff struct {
	client.Client
	Log          logr.Logger
	Repository   *gitv1.GitRepository
	GithubClient *scm.Client
}

func (p *PullRequestDiff) Merge(ctx context.Context, gh, k8s *gitv1.GitPullRequest) error {
	if gh != nil && k8s == nil {
		err := p.Client.Create(ctx, gh)
		return errors.Wrapf(err, "failed to create GitPullRequest %s in namespace %s", gh.Name, gh.Namespace)
	} else if gh == nil && k8s != nil {
		err := p.createInGithub(ctx, k8s)
		yml, _ := yaml.Marshal(k8s)
		fmt.Printf("Creating in Github for this PR\n%s", string(yml))
		return errors.Wrapf(err, "failed to create Github PullRequest for GitPullRequest %s in namespace %s", k8s.Name, k8s.Namespace)
	} else if gh == nil && k8s == nil {
		panic(errors.New("Received both github pull request and k8s pull request as nil values"))
	}

	return nil
}

func (p *PullRequestDiff) createInGithub(ctx context.Context, pr *gitv1.GitPullRequest) error {
	repoName := pr.Spec.Repository
	prRequest := &scm.PullRequestInput{
		Title: pr.Spec.Title,
		Body:  pr.Spec.Body,
		Base:  pr.Spec.Base,
		Head:  pr.Spec.Head,
	}
	ghPR, response, err := p.GithubClient.PullRequests.Create(ctx, repoName, prRequest)
	if err != nil {
		buf := new(bytes.Buffer)
		buf.ReadFrom(response.Body)
		log.Errorf("Github status code: %d", response.Status)
		log.Errorf("Github response:\n [%s]", buf.String())
		return errors.Wrap(err, "failed to create PullRequest in Github")
	}

	if len(pr.Spec.Reviewers) > 0 {
		if _, err = p.GithubClient.PullRequests.RequestReview(ctx, repoName, ghPR.Number, pr.Spec.Reviewers); err != nil {
			return errors.Wrap(err, "failed to add reviewers to Github PullRequest")
		}
	}

	pr.Status.ID = strconv.Itoa(ghPR.Number)
	pr.Status.Ref = fmt.Sprintf("refs/pull/%d/head", ghPR.Number)
	pr.Status.URL = fmt.Sprintf("https://github.com/%s/pull/%d.diff", repoName, ghPR.Number)
	if err := p.Client.Update(ctx, pr); err != nil {
		return errors.Wrapf(err, "failed to set PR number for GitPullRequest %s in namespace %s", pr.Name, pr.Namespace)
	}

	// TODO: add assignee / approvals
	return nil
}
