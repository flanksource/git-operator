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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5"
	gitv5 "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-logr/logr"
	"github.com/labstack/echo"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/types"

	"github.com/flanksource/commons/text"
	"github.com/flanksource/commons/utils"
	gitv1 "github.com/flanksource/git-operator/api/v1"
	"github.com/flanksource/git-operator/connectors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GitopsAPIReconciler reconciles a GitopsAPI object
type GitopsAPIReconciler struct {
	client.Client
	Clientset *kubernetes.Clientset
	Log       logr.Logger
	Scheme    *runtime.Scheme
}

// +kubebuilder:rbac:groups=git.flanksource.com,resources=gitopsapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=git.flanksource.com,resources=gitopsapis/status,verbs=get;update;patch

func (r *GitopsAPIReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("gitopsapi", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}

func serve(c echo.Context, r *GitopsAPIReconciler) error {
	ctx := context.Background()
	name := c.Param("name")
	namespace := c.Param("namespace")
	token := c.Param("token")
	if token == "" {
		token = c.QueryParam("token")
	}
	if token == "" {
		token = c.Request().Header.Get("Authorization")
	}

	api := gitv1.GitopsAPI{}
	if err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &api); err != nil {
		return c.String(http.StatusNotFound, "")
	}

	if api.Spec.TokenRef != nil {
		tokenValue, err := r.Clientset.CoreV1().Secrets(namespace).Get(ctx, api.Spec.TokenRef.Name, metav1.GetOptions{})
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}
		if token != string(tokenValue.Data["TOKEN"]) {
			return c.String(http.StatusForbidden, "")
		}
	}

	r.Log.Info("Found API", "name", name, "namespace", namespace, "repo", api.Spec.GitRepository, "secret", *api.Spec.SecretRef, "client", r.Client, "ctx", ctx)

	git, err := connectors.NewConnector(ctx, r.Client, r.Clientset, r.Log, namespace, api.Spec.GitRepository, api.Spec.SecretRef)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}
	fs, work, err := git.Clone(ctx, api.Spec.Branch)
	if err != nil {
		r.Log.Error(err, "error cloning")
		return c.String(http.StatusInternalServerError, err.Error())
	}

	timestamp := utils.ShortTimestamp()
	defaultBranch := api.Spec.Branch
	if defaultBranch == "" {
		defaultBranch = "master"
	}
	branchName := fmt.Sprintf("automated-update-%s", timestamp)

	if api.Spec.PullRequest {
		work.Checkout(&gitv5.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(branchName), Create: true})
	}

	obj := unstructured.Unstructured{Object: make(map[string]interface{})}
	body, _ := ioutil.ReadAll(c.Request().Body)
	if err := json.Unmarshal(body, &obj.Object); err != nil {
		r.Log.Error(err, "error unmarshalling")
		return c.String(http.StatusInternalServerError, err.Error())
	}

	prettyYaml, err := yaml.Marshall(obj.Object)
	if err != nil {
		r.Log.Error(err, "error marshalling to yaml, falling back to plaintext")
	} else {
		body = prettyYaml
	}

	r.Log.Info("Received", "name", name, "namespace", namespace, "body", obj.GetName())

	kustomizationPath, err := text.Template(api.Spec.Kustomization, obj.Object)
	if err != nil {
		r.Log.Error(err, "error determining kustomization.yaml path")
		return c.String(http.StatusInternalServerError, err.Error())
	}
	contentPath, err := text.Template(api.Spec.Path, obj.Object)
	if err != nil {
		r.Log.Error(err, "error determining path")
		return c.String(http.StatusInternalServerError, err.Error())
	}

	r.Log.Info("Saving to", "path", contentPath, "kustomization", kustomizationPath, "name", name, "namespace", namespace)

	if err := copy(body, contentPath, fs, work); err != nil {
		r.Log.Error(err, "error saving content")
		return c.String(http.StatusInternalServerError, err.Error())
	}

	existing, err := fs.Open(kustomizationPath)
	if err != nil {
		r.Log.Error(err, "error opening kustomization file", "path", kustomizationPath)
		return c.String(http.StatusInternalServerError, err.Error())
	}
	existingKustomization, err := ioutil.ReadAll(existing)
	if err != nil {
		r.Log.Error(err, "error reading kustomization file", "path", kustomizationPath)
		return c.String(http.StatusInternalServerError, err.Error())
	}
	kustomization := types.Kustomization{}
	if err := yaml.Unmarshal(existingKustomization, &kustomization); err != nil {
		r.Log.Error(err, "error unmarshalling kustomization")
		return c.String(http.StatusInternalServerError, err.Error())
	}
	relativePath := strings.Replace(contentPath, path.Dir(kustomizationPath)+"/", "", -1)
	kustomization.Resources = append(kustomization.Resources, relativePath)
	existingKustomization, _ = yaml.Marshal(kustomization)

	if err := copy(existingKustomization, kustomizationPath, fs, work); err != nil {
		r.Log.Error(err, "error saving kustomization")
		return c.String(http.StatusInternalServerError, err.Error())
	}
	author := &object.Signature{
		Name:  api.Spec.GitUser,
		Email: api.Spec.GitEmail,
		When:  time.Now(),
	}
	if author.Name == "" {
		author.Name = "Git Operator"
	}
	if author.Email == "" {
		author.Email = "git-operator@noreply.flanksource.com"
	}
	hash, err := work.Commit("Automated Update", &gitv5.CommitOptions{
		Author: author,
		All:    true,
	})

	if err != nil {
		r.Log.Error(err, "error updating fs")
		return c.String(http.StatusInternalServerError, err.Error())
	}

	if err := git.Push(ctx); err != nil {
		r.Log.Error(err, "error pushing")
		return c.String(http.StatusInternalServerError, err.Error())
	}

	if api.Spec.PullRequest {
		pr := connectors.PullRequest{
			Title:     fmt.Sprintf("Automated update %s", timestamp),
			Body:      fmt.Sprintf("Added resource `%s/%s/%s` in `%s`", obj.GetKind(), obj.GetNamespace(), obj.GetName(), contentPath),
			Head:      branchName,
			Base:      defaultBranch,
			Reviewers: api.Spec.Reviewers,
		}
		if err := git.CreatePullRequest(ctx, pr); err != nil {
			r.Log.Error(err, "error creating pull request")
			return c.String(http.StatusInternalServerError, err.Error())
		}
	}

	return c.String(http.StatusAccepted, "Committed "+hash.String())
}

func copy(data []byte, path string, fs billy.Filesystem, work *gitv5.Worktree) error {
	dst, err := openOrCreate(path, fs)
	if err != nil {
		return errors.Wrap(err, "failed to open")
	}
	src := bytes.NewBuffer([]byte(data))
	if _, err := io.Copy(dst, src); err != nil {
		return errors.Wrap(err, "failed to copy")
	}
	if err := dst.Close(); err != nil {
		return errors.Wrap(err, "failed to close")
	}
	_, err = work.Add(path)
	return errors.Wrap(err, "failed to add to git")
}

func openOrCreate(path string, fs billy.Filesystem) (billy.File, error) {
	return fs.Create(path)
}

func (r *GitopsAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	r.Clientset = clientset
	r.Client = mgr.GetClient()
	ctrl.NewControllerManagedBy(mgr).
		For(&gitv1.GitopsAPI{}).
		Complete(r)
	e := echo.New()
	e.POST("/:namespace/:name/:token", func(c echo.Context) error {
		return serve(c, r)
	})
	e.POST("/:namespace/:name", func(c echo.Context) error {
		return serve(c, r)
	})
	go func() {
		e.Logger.Fatal(e.Start(":8888"))
	}()

	return nil

}
