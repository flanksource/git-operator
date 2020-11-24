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
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	gitv1 "github.com/flanksource/git-operator/api/v1"
	"github.com/flanksource/kommons"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	KubeSystemNamespace   = "kube-system"
	GitopsDeleteFinalizer = "termination.flanksource.com/protect"
)

// GitOpsReconciler reconciles a GitOps object
type GitOpsReconciler struct {
	client.Client
	Clientset     *kubernetes.Clientset
	KommonsClient *kommons.Client
	Log           logr.Logger
	Scheme        *runtime.Scheme
}

// +kubebuilder:rbac:groups=git.flanksource.com,resources=gitops,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=git.flanksource.com,resources=gitops/status,verbs=get;update;patch

func (r *GitOpsReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("gitops", req.NamespacedName)

	config := &gitv1.GitOps{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		if kerrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		log.Error(err, "Failed to get GitOps")
		return ctrl.Result{}, err
	}

	hasFinalizer := false
	for _, finalizer := range config.ObjectMeta.Finalizers {
		if finalizer == GitopsDeleteFinalizer {
			hasFinalizer = true
		}
	}

	objects := NewGitops(config)

	if config.ObjectMeta.DeletionTimestamp != nil {
		for _, obj := range objects {
			client, _, unstructuredObject, err := r.KommonsClient.GetDynamicClientFor(config.Spec.Namespace, obj)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to get dynamic client")
			}
			if err := client.Delete(ctx, unstructuredObject.GetName(), metav1.DeleteOptions{}); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "failed to delete object")
			}
		}

		finalizers := []string{}
		for _, finalizer := range config.ObjectMeta.Finalizers {
			if finalizer != GitopsDeleteFinalizer {
				finalizers = append(finalizers, finalizer)
			}
		}
		config.ObjectMeta.Finalizers = finalizers
		config.Status.LastUpdated = metav1.Now()
		if err := r.Update(ctx, config); err != nil {
			log.Error(err, "failed to remove finalizer from object")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if !hasFinalizer {
		config.ObjectMeta.Finalizers = append(config.ObjectMeta.Finalizers, GitopsDeleteFinalizer)
		config.Status.LastUpdated = metav1.Now()
		if err := r.Update(ctx, config); err != nil {
			log.Error(err, "failed to add finalizer to object")
			return ctrl.Result{}, err
		}
	}

	for _, obj := range objects {
		if err := r.KommonsClient.Apply(config.Spec.Namespace, obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *GitOpsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gitv1.GitOps{}).
		Complete(r)
}

// +kubebuilder:rbac:groups="*",resources="*",verbs="*"

func NewGitops(gitops *gitv1.GitOps) []runtime.Object {
	gitopsDefaults(gitops)
	spec := gitops.Spec
	memcacheName := fmt.Sprintf("flux-memcache-%s", spec.Name)
	secretName := fmt.Sprintf("flux-git-deploy-%s", spec.Name)
	sshConfig := fmt.Sprintf("flux-ssh-%s", spec.Name)
	saName := fmt.Sprintf("flux-" + spec.Name)
	argMap := map[string]string{
		"git-url":                spec.GitURL,
		"git-branch":             spec.GitBranch,
		"git-path":               spec.GitPath,
		"git-poll-interval":      spec.GitPollInterval,
		"sync-interval":          spec.SyncInterval,
		"k8s-secret-name":        secretName,
		"ssh-keygen-dir":         "/etc/fluxd/ssh",
		"memcached-hostname":     memcacheName,
		"manifest-generation":    "true",
		"registry-exclude-image": "*",
		// use ClusterIP rather than DNS SRV lookup
		"memcached-service": "",
	}

	builder := kommons.Builder{
		Namespace: spec.Namespace,
	}

	if *spec.DisableScanning {
		argMap["git-readonly"] = "true"
		argMap["registry-disable-scanning"] = "true"
	} else {
		// memcache is only deployed for scanning
		builder.Deployment(memcacheName, "docker.io/memcached:1.4.36-alpine").
			Args("-m 512", "-p 11211", "-I 5m").
			Expose(11211).
			Build()
	}

	repo := "docker.io/fluxcd/flux"
	if strings.Contains(spec.FluxVersion, "flanksource") {
		repo = "docker.io/flanksource/flux"
	}
	builder.Deployment("flux-"+spec.Name, fmt.Sprintf("%s:%s", repo, spec.FluxVersion)).
		Labels(map[string]string{
			"app": "flux",
		}).
		Args(getArgs(gitops, argMap)...).
		ServiceAccount(saName).
		MountSecret(secretName, "/etc/fluxd/ssh", int32(0400)).
		MountConfigMap(sshConfig, "/root/.ssh").
		Expose(3030).
		Build()

	var sa *kommons.ServiceAccountBuilder
	if spec.Namespace == KubeSystemNamespace {
		builder.ServiceAccount(saName).AddClusterRole("cluster-admin")
	} else {
		sa = builder.ServiceAccount(saName).AddRole("namespace-admin").AddRole("namespace-creator")
	}

	if spec.HelmOperatorVersion != "" {
		args := []string{"--enabled-helm-versions=v3"}
		if spec.Namespace != KubeSystemNamespace {
			args = append(args, "--allow-namespace="+spec.Namespace)
		}
		builder.Deployment("helm-operator-"+spec.Name, fmt.Sprintf("docker.io/fluxcd/helm-operator:%s", spec.HelmOperatorVersion)).
			Labels(map[string]string{
				"app": "helm-operator",
			}).
			Args(args...).
			ServiceAccount(saName).
			MountSecret(secretName, "/etc/fluxd/ssh", int32(0400)).
			MountConfigMap(sshConfig, "/root/.ssh").
			Expose(3030).
			Build()
		if sa != nil {
			sa.AddClusterRole("helm-operator-admin")
		}
	}
	//TODO: else delete existing helm-operator deployment

	data, _ := base64.StdEncoding.DecodeString(spec.GitKey)
	builder.Secret(secretName, map[string][]byte{
		"identity": data,
	})
	builder.ConfigMap(sshConfig, map[string]string{
		"known_hosts": spec.KnownHosts,
		"config":      spec.SSHConfig,
	})

	return builder.Objects
}

func gitopsDefaults(cr *gitv1.GitOps) {
	if cr.Spec.Name == "" {
		cr.Spec.Name = cr.Namespace
	}

	if cr.Spec.Namespace == "" {
		cr.Spec.Namespace = KubeSystemNamespace
	}

	if cr.Spec.GitBranch == "" {
		cr.Spec.GitBranch = "master"
	}

	if cr.Spec.GitPath == "" {
		cr.Spec.GitPath = "./"
	}

	if cr.Spec.GitPollInterval == "" {
		cr.Spec.GitPollInterval = "60s"
	}

	if cr.Spec.SyncInterval == "" {
		cr.Spec.SyncInterval = "5m00s"
	}

	if cr.Spec.FluxVersion == "" {
		cr.Spec.FluxVersion = "1.20.0"
	}

	if cr.Spec.DisableScanning == nil {
		t := true
		cr.Spec.DisableScanning = &t
	}
}

func getArgs(cr *gitv1.GitOps, argMap map[string]string) []string {
	var args []string
	for key, value := range cr.Spec.Args {
		argMap[key] = value
	}
	for key, value := range argMap {
		args = append(args, fmt.Sprintf("--%s=%s", key, value))
	}

	sort.Strings(args)
	return args
}
