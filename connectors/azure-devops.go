package connectors

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/pkg/errors"

	"github.com/go-git/go-billy/v5/osfs"

	gitv1 "github.com/flanksource/git-operator/api/v1"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-logr/logr"
	git2go "github.com/libgit2/git2go/v31"
	"github.com/microsoft/azure-devops-go-api/azuredevops"
	azGit "github.com/microsoft/azure-devops-go-api/azuredevops/git"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AzureDevops struct {
	azGitClient azGit.Client
	repo        *git.Repository
	git2goRepo  *git2go.Repository
	logr.Logger
	organizationName    string
	projectName         string
	repoName            string
	token               string
	credentialsCallBack git2go.CredentialsCallback
}

func NewAzureDevops(client client.Client, log logr.Logger, organizationName, projectName, repoName, azureDevopsToken string) (Connector, error) {
	organizationUrl := fmt.Sprintf("https://dev.azure.com/%s", organizationName)
	connection := azuredevops.NewPatConnection(organizationUrl, azureDevopsToken)
	azGitClient, err := azGit.NewClient(context.TODO(), connection)
	if err != nil {
		return nil, err
	}
	azureDevops := &AzureDevops{
		azGitClient:      azGitClient,
		Logger:           log.WithName("AzureDevops").WithName(fmt.Sprintf("%s/%s/%s", organizationName, projectName, repoName)),
		organizationName: organizationName,
		projectName:      projectName,
		repoName:         repoName,
		token:            azureDevopsToken,
		credentialsCallBack: func(url, username string, t git2go.CredentialType) (*git2go.Credential, error) {
			cred, err := git2go.NewCredUserpassPlaintext(azureDevopsToken, "x-oauth-basic")
			if err != nil {
				return nil, err
			}
			return cred, nil
		},
	}
	return azureDevops, nil
}

func (a *AzureDevops) Clone(ctx context.Context, branch, local string) (billy.Filesystem, *git.Worktree, error) {
	dir, _ := ioutil.TempDir("", "git-*")
	url := fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s", a.organizationName, a.projectName, a.repoName)
	a.Info("Cloning", "temp", dir, "url", url, "branch", branch)

	// Use git2go to checkout Azure DevOps repo
	// then switch to go-git to handle local git tasks
	git2goRepo, err := git2go.Clone(url, dir, &git2go.CloneOptions{
		CheckoutOpts: &git2go.CheckoutOpts{
			Strategy:        git2go.CheckoutSafe,
			TargetDirectory: "",
			Paths:           nil,
			Baseline:        nil,
		},
		FetchOptions: &git2go.FetchOptions{
			RemoteCallbacks: git2go.RemoteCallbacks{
				CredentialsCallback: a.credentialsCallBack,
			},
		},
		CheckoutBranch: branch,
	})
	if err != nil {
		return nil, nil, err
	}
	a.git2goRepo = git2goRepo

	dot, _ := osfs.New(dir).Chroot(".git")
	storage := filesystem.NewStorage(dot, cache.NewObjectLRUDefault())
	repo, err := git.Open(storage, osfs.New(dir))
	if err != nil {
		fmt.Printf("err when open %v", err)
		return nil, nil, err
	}
	a.repo = repo

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

func (a *AzureDevops) Push(ctx context.Context, branch string) error {
	if a.git2goRepo == nil {
		return errors.New("Need to clone before pushing")
	}

	ref, _ := a.git2goRepo.Head()
	branchName, _ := ref.Branch().Name()

	a.V(1).Info("Pushing", "branch", branchName)

	remote, err := a.git2goRepo.Remotes.Lookup("origin")
	if err != nil {
		return err
	}
	err = remote.Push([]string{"+refs/heads/" + branchName}, &git2go.PushOptions{
		RemoteCallbacks: git2go.RemoteCallbacks{
			CredentialsCallback: a.credentialsCallBack,
		},
	})
	if err != nil {
		return err
	}
	a.Info("Pushed", "branch", branchName, "ref", ref)
	return nil
}

func (a *AzureDevops) OpenPullRequest(ctx context.Context, base string, head string, spec *gitv1.PullRequestTemplate) (int, error) {
	if spec.Title == "" {
		spec.Title = head
	}
	a.V(1).Info("Creating PR", "title", spec.Title, "head", head, "base", base)
	targetRefName := fmt.Sprintf("refs/heads/%s", base)
	sourceRefName := fmt.Sprintf("refs/heads/%s", head)
	pr, err := a.azGitClient.CreatePullRequest(ctx, azGit.CreatePullRequestArgs{
		GitPullRequestToCreate: &azGit.GitPullRequest{
			Title:         &spec.Title,
			TargetRefName: &targetRefName,
			SourceRefName: &sourceRefName,
		},
		RepositoryId: &a.repoName,
		Project:      &a.projectName,
	})
	if err != nil {
		return 0, errors.Wrapf(err, "failed to create pr repo=%s title=%s, head=%s base=%s", a.repoName, spec.Title, head, base)
	}
	a.Info("PR created", "pr", pr.PullRequestId, "repository", a.repoName)
	return *pr.PullRequestId, nil
}

func (a *AzureDevops) ReconcileBranches(ctx context.Context, repository *gitv1.GitRepository) error {
	panic("implement me")
}

func (a *AzureDevops) ReconcilePullRequests(ctx context.Context, repository *gitv1.GitRepository) error {
	panic("implement me")
}

func (a *AzureDevops) ClosePullRequest(ctx context.Context, id int) error {
	panic("implement me")
}

func (a *AzureDevops) ReconcileDeployments(ctx context.Context, repository *gitv1.GitRepository) error {
	panic("implement me")
}
