package connectors

import (
	"context"
	"fmt"

	gitv1 "github.com/flanksource/git-operator/api/v1"
	"github.com/go-logr/logr"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/factory"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GitSSH struct {
	client *scm.Client
	k8s    *kubernetes.Clientset
	k8sCrd client.Client
	log    logr.Logger
	scm    *scm.Client
}

func NewGithub(client client.Client, clientset *kubernetes.Clientset, log logr.Logger, githubToken string) (Connector, error) {
	scmClient, err := factory.NewClient("github", "", githubToken)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create github client")
	}

	github := &GitSSH{
		k8s:    clientset,
		k8sCrd: client,
		log:    log.WithName("connector").WithName("Github"),
		scm:    scmClient,
	}
	return github, nil
}

func (g *GitSSH) ReconcileBranches(ctx context.Context, repository *gitv1.GitRepository) error {
	lastUpdated := repository.Status.LastUpdated.Time
	log := g.log.WithValues("gitrepository", fmt.Sprintf("%s/%s", repository.Namespace, repository.Name))

	log.V(4).Info("lastUpdated: %s", lastUpdated.String())

	githubFetcher := &GithubFetcher{client: g.scm, repository: *repository}
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

func (g *GitSSH) ReconcilePullRequests(ctx context.Context, repository *gitv1.GitRepository) error {
	return nil
}
