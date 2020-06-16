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

func (r *GitRepositoryReconciler) reconcilePullRequests(ctx context.Context, client *scm.Client, repository *gitflanksourcecomv1.GitRepository) error {
	repoName := getRepositoryName(*repository)
	lastUpdated := repository.Status.LastUpdated.Time

	fmt.Printf("lastUpdated: %s\n", lastUpdated.String())

	prs, _, err := client.PullRequests.List(ctx, repoName, scm.PullRequestListOptions{UpdatedAfter: &lastUpdated})
	if err != nil {
		return err
	}

	ghCrds := buildPullRequestCRDsFromGithub(prs, *repository)
	yml, err := yaml.Marshal(ghCrds)
	if err != nil {
		return err
	}
	fmt.Printf("YAML:\n%s\n", string(yml))

	crdPRs := gitflanksourcecomv1.GitPullRequestList{}
	if err := r.List(ctx, &crdPRs); err != nil {
		return err
	}

	for _, pr := range prs {
		fmt.Printf("PullRequest id=%d title=%s author=%s head=%s sha=%s source=%s\n", pr.Number, pr.Title, pr.Author.Login, pr.Head.Sha, pr.Sha, pr.Source)
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
