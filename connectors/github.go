package connectors

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	gitv1 "github.com/flanksource/git-operator/api/v1"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-logr/logr"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/factory"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Github struct {
	k8sCrd client.Client
	logr.Logger
	scm        *scm.Client
	repo       *git.Repository
	auth       transport.AuthMethod
	owner      string
	repoName   string
	repository string
}

func NewGithub(client client.Client, log logr.Logger, owner, repoName, githubToken string) (Connector, error) {
	scmClient, err := factory.NewClient("github", "", githubToken)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create github client")
	}

	github := &Github{
		k8sCrd:     client,
		Logger:     log.WithName("Github").WithName(owner + "/" + repoName),
		scm:        scmClient,
		owner:      owner,
		repoName:   repoName,
		repository: owner + "/" + repoName,
		auth:       &http.BasicAuth{Password: githubToken, Username: githubToken},
	}
	return github, nil
}

func (g *Github) Push(ctx context.Context, branch string) error {
	if g.repo == nil {
		return errors.New("Need to clone first, before pushing ")
	}

	g.V(1).Info("Pushing", "branch", branch)

	if err := g.repo.Push(&git.PushOptions{
		Auth:     g.auth,
		Progress: os.Stdout,
	}); err != nil {
		return err
	}
	ref, _ := g.repo.Head()
	g.Info("Pushed", "branch", branch, "ref", ref.String())
	return nil
}

func (g *Github) OpenPullRequest(ctx context.Context, base string, head string, spec *gitv1.PullRequestTemplate) (int, error) {
	if spec.Title == "" {
		spec.Title = head
	}
	g.V(1).Info("Creating PR", "title", spec.Title, "head", head, "base", base)
	pr, _, err := g.scm.PullRequests.Create(ctx, g.repository, &scm.PullRequestInput{
		Title: spec.Title,
		Body:  spec.Body,
		Head:  head,
		Base:  base,
	})

	if err != nil {
		return 0, errors.Wrapf(err, "failed to create pr repo=%s title=%s, head=%s base=%s", g.repository, spec.Title, head, base)
	}
	g.Info("PR created", "pr", pr.Number, "repository", g.repository)

	if len(spec.Reviewers) > 0 {
		g.Info("Requesting Reviews", "pr", pr.Number, "repository", g.repository, "reviewers", spec.Reviewers)
		if _, err := g.scm.PullRequests.RequestReview(ctx, g.repository, pr.Number, spec.Reviewers); err != nil {
			return 0, err
		}
	}

	if len(spec.Assignees) > 0 {
		g.Info("Assigning PR", "pr", pr.Number, "repository", g.repoName, "assignees", spec.Assignees)
		if _, err := g.scm.PullRequests.AssignIssue(ctx, g.repository, pr.Number, spec.Assignees); err != nil {
			return 0, err
		}
	}

	return pr.Number, nil
}

func (g *Github) ClosePullRequest(ctx context.Context, id int) error {
	if _, err := g.scm.PullRequests.Close(ctx, g.repository, id); err != nil {
		return errors.Wrap(err, "failed to close github pull request")
	}

	return nil
}

func (g *Github) Clone(ctx context.Context, branch, local string) (billy.Filesystem, *git.Worktree, error) {
	dir, _ := ioutil.TempDir("", "git-*")
	url := fmt.Sprintf("https://github.com/%s/%s.git", g.owner, g.repoName)
	g.Info("Cloning", "temp", dir)
	repo, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		URL:           url,
		Progress:      os.Stdout,
		Auth:          g.auth,
	})
	if err != nil {
		return nil, nil, err
	}
	g.repo = repo

	work, err := repo.Worktree()
	if err != nil {
		return nil, nil, err
	}
	if branch != local {
		// nolint: errcheck
		work.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewBranchReferenceName(local),
			Create: true,
		})
	}
	return osfs.New(dir), work, nil
}

func (g *Github) ReconcileBranches(ctx context.Context, repository *gitv1.GitRepository) error {
	lastUpdated := repository.Status.LastUpdated.Time
	log := g.WithValues("gitrepository", fmt.Sprintf("%s/%s", repository.Namespace, repository.Name))

	log.V(4).Info("lastUpdated: %s", lastUpdated.String())

	githubFetcher := &GithubFetcher{client: g.scm, repository: *repository, owner: g.owner, repoName: g.repoName}
	ghCrds, err := githubFetcher.BuildBranchCRDsFromGithub(ctx, lastUpdated)

	if err != nil {
		log.Error(err, "failed to build GitBranch CRD from Github")
		return err
	}

	listOptions := &client.MatchingLabels{
		"git.flanksource.com/repository": repository.Name,
	}

	k8sCrds := gitv1.GitBranchList{}
	if err := g.k8sCrd.List(ctx, &k8sCrds, listOptions); err != nil {
		return err
	}
	inK8sByName := map[string]*gitv1.GitBranch{}
	for _, k8sCrd := range k8sCrds.Items {
		inK8sByName[k8sCrd.Spec.BranchName] = k8sCrd.DeepCopy()
	}
	for _, ghCrd := range ghCrds {
		k8sCrd, found := inK8sByName[ghCrd.Spec.BranchName]
		if !found {
			if err := g.k8sCrd.Create(ctx, &ghCrd); err != nil {
				log.Error(err, "failed to create GitBranch CRD", "branch", ghCrd.Spec.BranchName)
				return err
			}
			log.Info("Branch created", "branch", ghCrd.Spec.BranchName)
		} else if k8sCrd.Status.Head != ghCrd.Status.Head {
			if err := g.k8sCrd.Update(ctx, &ghCrd); err != nil {
				log.Error(err, "failed to update GitBranch CRD", "branch", ghCrd.Spec.BranchName)
				return err
			}
			log.Info("Branch updated", "branch", ghCrd.Spec.BranchName)
		} else {
			log.Info("Branch did not change", "branch", ghCrd.Spec.BranchName)
		}
	}

	return nil
}

func (g *Github) ReconcilePullRequests(ctx context.Context, repository *gitv1.GitRepository) error {
	lastUpdated := repository.Status.LastUpdated.Time
	log := g.WithValues("gitrepository", fmt.Sprintf("%s/%s", repository.Namespace, repository.Name))

	log.V(4).Info("lastUpdated: %s\n", lastUpdated.String())

	githubFetcher := &GithubFetcher{client: g.scm, repository: *repository, owner: g.owner, repoName: g.repoName}
	ghCrds, err := githubFetcher.BuildPRCRDsFromGithub(ctx, lastUpdated)
	if err != nil {
		log.Error(err, "failed to build PullRequest CRD from Github")
		return err
	}

	listOptions := &client.MatchingLabels{
		"git.flanksource.com/repository": repository.Name,
	}

	k8sCrds := gitv1.GitPullRequestList{}
	if err := g.k8sCrd.List(ctx, &k8sCrds, listOptions); err != nil {
		return err
	}

	inGithubByID := map[string]*gitv1.GitPullRequest{}
	inK8sByID := map[string]*gitv1.GitPullRequest{}
	inK8sWithoutID := []*gitv1.GitPullRequest{}
	allIds := map[string]bool{}
	for _, crd := range ghCrds {
		inGithubByID[crd.Status.ID] = crd.DeepCopy()
		allIds[crd.Status.ID] = true
	}
	for _, crd := range k8sCrds.Items {
		if crd.Status.ID != "" {
			inK8sByID[crd.Status.ID] = crd.DeepCopy()
			allIds[crd.Status.ID] = true
		} else {
			inK8sWithoutID = append(inK8sWithoutID, crd.DeepCopy())
		}
	}

	diff := PullRequestDiff{
		Client:       g.k8sCrd,
		Log:          log,
		Repository:   repository,
		GithubClient: g.scm,
	}

	for id := range allIds {
		gh := inGithubByID[id]
		k8s := inK8sByID[id]

		if err := diff.Merge(ctx, gh, k8s); err != nil {
			return err
		}
	}

	// Now we go through the GitPullRequests which currently don't have an ID
	for _, crd := range inK8sWithoutID {
		if err := diff.Merge(ctx, nil, crd); err != nil {
			return err
		}
	}

	return nil
}
