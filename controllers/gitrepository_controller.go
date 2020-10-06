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

	"github.com/go-logr/logr"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gitv1 "github.com/flanksource/git-operator/api/v1"
	"github.com/flanksource/git-operator/connectors"
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

// +kubebuilder:rbac:groups=git.flanksource.com,resources=gitbranches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=git.flanksource.com,resources=gitbranches/status,verbs=get;update;patch

func (r *GitRepositoryReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("gitrepository", req.NamespacedName)

	repository := &gitv1.GitRepository{}
	if err := r.Get(ctx, req.NamespacedName, repository); err != nil {
		if kerrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		log.Error(err, "Failed to get GitRepository")
		return ctrl.Result{}, err
	}

	log.Info("Got git repository from server", "repository", repository.Name)

	connector, err := connectors.NewConnector(ctx, r.Client, r.Clientset, r.Log, repository.Namespace, repository.Spec.URL, repository.Spec.SecretRef)
	if err != nil {
		log.Error(err, "failed to create connector")
		return ctrl.Result{}, err
	}

	if err := connector.ReconcileBranches(ctx, repository); err != nil {
		log.Error(err, "failed to reconcile pull requests")
	}

	if err := connector.ReconcilePullRequests(ctx, repository); err != nil {
		log.Error(err, "failed to reconcile branches")
	}

	return ctrl.Result{}, nil
}

func (r *GitRepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gitv1.GitRepository{}).
		Complete(r)
}
