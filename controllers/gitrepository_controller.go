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
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gitflanksourcecomv1 "github.com/flanksource/git-operator/api/v1"
)

// GitRepositoryReconciler reconciles a GitRepository object
type GitRepositoryReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=git.flanksource.com,resources=gitrepositories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=git.flanksource.com,resources=gitrepositories/status,verbs=get;update;patch

func (r *GitRepositoryReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("gitrepository", req.NamespacedName)

	repository := &gitflanksourcecomv1.GitRepository{}
	if err := r.Get(ctx, req.NamespacedName, repository); err != nil {
		log.Error(err, "Failed to get GitRepository")
		return reconcile.Result{}, err
	}

	if repository.Spec.Github == nil {
		err := errors.Wrapf(ErrGithubFieldMissing, "for GitRepository %s in namespace %s", req.Name, req.Namespace)
		log.Error(ErrGithubFieldMissing, "invalid repository spec")
		return reconcile.Result{}, err
	}

	secretName := repository.Spec.Github.Credentials.Name
	secretNamespace := repository.Spec.Github.Credentials.Namespace
	secret, err := getRepositoryCredentials(ctx, r.Client, secretName, secretNamespace)
	if err != nil {
		log.Error(err, "failed to get secret %s in namespace %s", secretName, secretNamespace)
	}

	return ctrl.Result{}, nil
}

func (r *GitRepositoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gitflanksourcecomv1.GitRepository{}).
		Complete(r)
}
