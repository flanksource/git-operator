/*
Copyright 2020 The Kubernetes authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/jenkins-x/go-scm/scm"
	"github.com/jenkins-x/go-scm/scm/factory"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gitflanksourcecomv1 "github.com/flanksource/git-operator/api/v1"
)

// GitRepositoryReconciler reconciles a GitRepository object
type GitRepositoryReconciler struct {
	client.Client
	Clientset *kubernetes.Clientset
	Log       logr.Logger
	Scheme    *runtime.Scheme
}

// +kubebuilder:rbac:groups=git.flanksource.com,resources=gitrepositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=git.flanksource.com,resources=gitrepositories/status,verbs=get;update;patch

func (r *GitRepositoryReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("gitrepository", req.NamespacedName)

	repository := &gitflanksourcecomv1.GitRepository{}
	if err := r.Get(ctx, req.NamespacedName, repository); err != nil {
		if kerrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		log.Error(err, "Failed to get GitRepository")
		return ctrl.Result{}, err
	}

	log.Info("Got git repository from server", "repository", repository.Name)

	if repository.Spec.Github == nil {
		err := errors.Wrapf(ErrGithubFieldMissing, "for GitRepository %s in namespace %s", req.Name, req.Namespace)
		log.Error(ErrGithubFieldMissing, "invalid repository spec")
		return ctrl.Result{}, err
	}

	secretName := repository.Spec.Github.Credentials.Name
	secretNamespace := repository.Spec.Github.Credentials.Namespace
	log.Info("Searching secret", "name", secretName, "namespace", secretNamespace)
	credentials, err := getRepositoryCredentials(ctx, r.Clientset, secretName, secretNamespace)
	if err != nil {
		log.Error(err, "failed to get", "secret", secretName, "namespace", secretNamespace)
		return ctrl.Result{}, err
	}

	client, err := factory.NewClient(credentials.Provider, "", credentials.AuthToken)
	if err != nil {
		log.Error(err, "failed to create go-scm factory")
		return ctrl.Result{}, err
	}

	if err := r.reconcilePullRequests(ctx, client, repository); err != nil {
		log.Error(err, "failed to reconcile pull requests for", "repository", getRepositoryName(*repository))
	}

	// if err := r.reconcileBranches(ctx, client, repository); err != nil {
	// 	log.Error(err, "failed to reconcile branches for", "repository", getRepositoryName(*repository))
	// }

	return ctrl.Result{}, nil
}

func (r *GitRepositoryReconciler) reconcilePullRequests(ctx context.Context, githubClient *scm.Client, repository *gitflanksourcecomv1.GitRepository) error {
	lastUpdated := repository.Status.LastUpdated.Time
	log := r.Log.WithValues("gitrepository", fmt.Sprintf("%s/%s", repository.Namespace, repository.Name))

	fmt.Printf("lastUpdated: %s\n", lastUpdated.String())

	githubFetcher := &GithubFetcher{client: githubClient, repository: *repository}
	ghCrds, err := githubFetcher.BuildPRCRDsFromGithub(ctx, lastUpdated)
	if err != nil {
		log.Error(err, "failed to build PullRequest CRD from Github")
		return err
	}

	listOptions := &client.MatchingLabels{
		"git.flanksource.com/repository": repository.Name,
	}

	k8sCrds := gitflanksourcecomv1.GitPullRequestList{}
	if err := r.List(ctx, &k8sCrds, listOptions); err != nil {
		return err
	}

	inGithubById := map[string]*gitflanksourcecomv1.GitPullRequest{}
	inK8sById := map[string]*gitflanksourcecomv1.GitPullRequest{}
	allIds := map[string]bool{}
	for _, crd := range ghCrds {
		inGithubById[crd.Spec.ID] = crd.DeepCopy()
		allIds[crd.Spec.ID] = true
	}
	for _, crd := range k8sCrds.Items {
		inK8sById[crd.Spec.ID] = crd.DeepCopy()
		allIds[crd.Spec.ID] = true
	}

	diff := PullRequestDiff{
		Client:       r.Client,
		Log:          log,
		Repository:   repository,
		GithubClient: githubClient,
	}

	for id, _ := range allIds {
		gh := inGithubById[id]
		k8s := inK8sById[id]

		yml, _ := yaml.Marshal(gh)
		fmt.Printf("YAML:\n%s", string(yml))

		if err := diff.Merge(ctx, gh, k8s); err != nil {
			return err
		}
	}

	return nil
}

func (r *GitRepositoryReconciler) reconcileBranches(ctx context.Context, client *scm.Client, repository *gitflanksourcecomv1.GitRepository) error {
	return nil
}

func (r *GitRepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gitflanksourcecomv1.GitRepository{}).
		Complete(r)
}
