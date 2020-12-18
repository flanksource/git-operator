package connectors

import (
	"context"
	"fmt"
	"os"

	gitv1 "github.com/flanksource/git-operator/api/v1"
	v1 "github.com/flanksource/git-operator/api/v1"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/go-logr/logr"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/pkg/errors"
	ssh2 "golang.org/x/crypto/ssh"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GitSSH struct {
	*scm.Client
	k8sCrd client.Client
	log    logr.Logger
	url    string
	auth   transport.AuthMethod
}

func (g *GitSSH) Push(ctx context.Context) error {
	return nil
}

func (g *GitSSH) Clone(ctx context.Context, branch string) (billy.Filesystem, *git.Worktree, error) {
	// Filesystem abstraction based on memory
	fs := memfs.New()

	repo, err := git.Clone(memory.NewStorage(), fs, &git.CloneOptions{
		URL:      g.url,
		Progress: os.Stdout,
		Auth:     g.auth,
	})

	work, err := repo.Worktree()
	if err != nil {
		return nil, nil, err
	}

	return fs, work, nil
}

func NewGitSSH(client client.Client, log logr.Logger, url, user string, privateKey []byte, password string) (Connector, error) {
	publicKeys, err := ssh.NewPublicKeys(user, privateKey, password)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create public keys")
	}
	publicKeys.HostKeyCallback = ssh2.InsecureIgnoreHostKey()

	github := &GitSSH{
		k8sCrd: client,
		log:    log.WithName("connector").WithName("GitSSH"),
		url:    url,
		auth:   publicKeys,
	}
	return github, nil
}

func (g *GitSSH) ReconcileBranches(ctx context.Context, repository *gitv1.GitRepository) error {
	log := g.log.WithValues("gitrepository", g.url)

	remoteCrds, err := g.GetBranchCRDsFromRemote(ctx, repository)

	if err != nil {
		log.Error(err, "failed to build GitBranch CRD from Git remote")
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
	for _, remoteCrd := range remoteCrds {
		k8sCrd, found := inK8sByName[remoteCrd.Spec.BranchName]
		if !found {
			if err := g.k8sCrd.Create(ctx, &remoteCrd); err != nil {
				log.Error(err, "failed to create GitBranch CRD", "branch", remoteCrd.Spec.BranchName)
				return err
			}
			log.Info("Branch created", "branch", remoteCrd.Spec.BranchName)
		} else if k8sCrd.Status.Head != remoteCrd.Status.Head {
			if err := g.k8sCrd.Update(ctx, &remoteCrd); err != nil {
				log.Error(err, "failed to update GitBranch CRD", "branch", remoteCrd.Spec.BranchName)
				return err
			}
			log.Info("Branch updated", "branch", remoteCrd.Spec.BranchName)
		} else {
			log.Info("Branch did not change", "branch", remoteCrd.Spec.BranchName)
		}
	}

	return nil
}

func (g *GitSSH) ReconcilePullRequests(ctx context.Context, repository *gitv1.GitRepository) error {
	return nil
}

func (g *GitSSH) CreatePullRequest(ctx context.Context, pr PullRequest) error {
	return errors.Errorf("pull request for git/ssh not supported")
}

func (g *GitSSH) GetBranchCRDsFromRemote(ctx context.Context, repository *gitv1.GitRepository) ([]gitv1.GitBranch, error) {
	r, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL:  g.url,
		Auth: g.auth,
	})

	if err != nil {
		return nil, errors.Wrap(err, "failed to clone repository")
	}

	opts := &git.FetchOptions{
		RefSpecs: []config.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
		Auth:     g.auth,
	}

	if err := r.Fetch(opts); err != nil {
		return nil, errors.Wrap(err, "failed to fetch remote")
	}

	branchesIter, err := r.Branches()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list branches")
	}

	branches := []v1.GitBranch{}

	branchesIter.ForEach(func(ref *plumbing.Reference) error {
		// Ignore HEAD
		if ref.Name().Short() == "HEAD" {
			return nil
		}
		branch := g.GetBranchCRDFromRemote(repository, ref)
		branches = append(branches, branch)
		return nil
	})

	return branches, nil
}

func (g *GitSSH) GetBranchCRDFromRemote(repository *gitv1.GitRepository, ref *plumbing.Reference) gitv1.GitBranch {
	repositoryName := g.url
	branchName := ref.Name().Short()

	crd := gitv1.GitBranch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.branchName(repository.Name, branchName),
			Namespace: repository.Namespace,
			Labels: map[string]string{
				"git.flanksource.com/repository": repository.Name,
				"git.flanksource.com/branch":     branchName,
			},
		},
		Spec: gitv1.GitBranchSpec{
			Repository: repositoryName,
			BranchName: branchName,
		},
		Status: gitv1.GitBranchStatus{
			LastUpdated: metav1.Now(),
			Head:        ref.Hash().String(),
		},
	}

	return crd
}

func (g *GitSSH) branchName(repository string, branch string) string {
	return fmt.Sprintf("%s-%s", repository, branch)
}
