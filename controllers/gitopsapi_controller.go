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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5"
	gitv5 "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-logr/logr"
	"github.com/labstack/echo"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/yaml"

	"github.com/flanksource/commons/text"
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

	return ctrl.Result{}, nil
}

func serve(c echo.Context, r *GitopsAPIReconciler) error {
	ctx := context.Background()
	name := c.Param("name")
	namespace := c.Param("namespace")
	token := c.Param("token")
	deleteObj := strings.HasPrefix(c.Path(), "/_delete")
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
	var hash string
	var pr int
	if deleteObj {
		hash, pr, err = DeleteObject(ctx, r.Log, git, api, c.Request().Body)
	} else {
		hash, pr, err = CreateOrUpdateObject(ctx, r.Log, git, api, c.Request().Body)
	}

	//hash, pr, err := HandleGitopsAPI(ctx, r.Log, git, api, c.Request().Body, deleteObj)

	if err != nil {
		r.Log.Error(err, "error pushing to git")
		return c.String(http.StatusInternalServerError, err.Error())
	}
	return c.String(http.StatusAccepted, fmt.Sprintf("Committed %s, PR: %d ", hash, pr))
}

func GetKustomizaton(fs billy.Filesystem, path string) (*types.Kustomization, error) {
	kustomization := types.Kustomization{}

	if _, err := fs.Stat(path); err != nil {
		return &kustomization, nil
	}
	existing, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	existingKustomization, _ := ioutil.ReadAll(existing)
	if err := yaml.Unmarshal(existingKustomization, &kustomization); err != nil {
		return nil, err
	}
	return &kustomization, nil
}

func CreateOrUpdateObject(ctx context.Context, logger logr.Logger, git connectors.Connector, api gitv1.GitopsAPI, contents io.Reader) (hash string, pr int, err error) {
	api = addDefaults(api)
	obj := unstructured.Unstructured{Object: make(map[string]interface{})}
	body, _ := ioutil.ReadAll(contents)
	if err = json.Unmarshal(body, &obj.Object); err != nil {
		return hash, pr, errors.WithStack(err)
	}
	body, _ = yaml.JSONToYAML(body)
	api.Spec.Branch, err = text.Template(api.Spec.Branch, obj.Object)
	if err != nil {
		return
	}
	fs, work, err := git.Clone(ctx, api.Spec.Base, api.Spec.Branch)
	if err != nil {
		return
	}
	contentPath, err := getContentPath(api, fs.Root(), obj)
	if err != nil {
		return
	}
	if contentPath == "" {
		contentPath = fmt.Sprintf("%s-%s-%s.yaml", obj.GetKind(), obj.GetNamespace(), obj.GetName())
	}
	title := fmt.Sprintf("Add/Update %s/%s/%s", obj.GetKind(), obj.GetNamespace(), obj.GetName())
	logger.Info("Received", "name", api.GetName(), "namespace", api.GetNamespace(), "object", title)

	kustomizationPath, err := text.Template(api.Spec.Kustomization, obj.Object)
	if err != nil {
		return
	}
	contentPath, err = text.Template(contentPath, obj.Object)
	if err != nil {
		return
	}
	logger.Info("Saving to", "path", contentPath, "kustomization", kustomizationPath, "object", title)
	kustomization, err := GetKustomizaton(fs, kustomizationPath)
	if err != nil {
		return
	}
	relativePath := strings.Replace(contentPath, path.Dir(kustomizationPath)+"/", "", -1)
	if err = copy(body, contentPath, fs, work); err != nil {
		return
	}
	index := findElement(kustomization.Resources, relativePath)
	if index == -1 {
		kustomization.Resources = append(kustomization.Resources, relativePath)
	}
	existingKustomization, _ := yaml.Marshal(kustomization)
	if err = copy(existingKustomization, kustomizationPath, fs, work); err != nil {
		return
	}
	hash, err = createCommit(ctx, api, git, work, title)
	if err != nil {
		return
	}
	if api.Spec.PullRequest != nil {
		pr, err = createPullRequest(ctx, api, obj, git)
		if err != nil {
			return
		}
	}
	return hash, pr, nil
}

func DeleteObject(ctx context.Context, logger logr.Logger, git connectors.Connector, api gitv1.GitopsAPI, contents io.Reader) (hash string, pr int, err error) {
	api = addDefaults(api)
	obj := unstructured.Unstructured{Object: make(map[string]interface{})}
	body, _ := ioutil.ReadAll(contents)
	if err = json.Unmarshal(body, &obj.Object); err != nil {
		return hash, pr, errors.WithStack(err)
	}
	api.Spec.Branch, err = text.Template(api.Spec.Branch, obj.Object)
	if err != nil {
		return
	}
	fs, work, err := git.Clone(ctx, api.Spec.Base, api.Spec.Branch)
	if err != nil {
		return
	}
	contentPath, err := getContentPath(api, fs.Root(), obj)
	if err != nil {
		return
	}
	if contentPath == "" {
		return hash, pr, fmt.Errorf("could not find the object to delete")
	}
	title := fmt.Sprintf("Delete %s/%s/%s", obj.GetKind(), obj.GetNamespace(), obj.GetName())
	logger.Info("Received", "name", api.GetName(), "namespace", api.GetNamespace(), "object", title)

	kustomizationPath, err := text.Template(api.Spec.Kustomization, obj.Object)
	if err != nil {
		return
	}
	contentPath, err = text.Template(contentPath, obj.Object)
	if err != nil {
		return
	}
	logger.Info("Saving to", "path", contentPath, "kustomization", kustomizationPath, "object", title)
	kustomization, err := GetKustomizaton(fs, kustomizationPath)
	if err != nil {
		return
	}
	relativePath := strings.Replace(contentPath, path.Dir(kustomizationPath)+"/", "", -1)

	if err = deleteFile(contentPath, work, fs.Root()); err != nil {
		return
	}
	index := findElement(kustomization.Resources, relativePath)
	if index != -1 {
		kustomization.Resources = removeElement(kustomization.Resources, index)
		existingKustomization, _ := yaml.Marshal(kustomization)
		if err = copy(existingKustomization, kustomizationPath, fs, work); err != nil {
			return
		}
	}
	hash, err = createCommit(ctx, api, git, work, title)
	if err != nil {
		return
	}
	if api.Spec.PullRequest != nil {
		pr, err = createPullRequest(ctx, api, obj, git)
		if err != nil {
			return
		}
	}
	return hash, pr, nil
}

func (r *GitopsAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	r.Clientset = clientset
	r.Client = mgr.GetClient()
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&gitv1.GitopsAPI{}).
		Complete(r); err != nil {
		return err
	}
	e := echo.New()
	e.POST("/:namespace/:name/:token", func(c echo.Context) error {
		return serve(c, r)
	})
	e.POST("/:namespace/:name", func(c echo.Context) error {
		return serve(c, r)
	})
	e.POST("/_delete/:namespace/:name", func(c echo.Context) error {
		return serve(c, r)
	})
	e.POST("/_delete/:namespace/:name/:token", func(c echo.Context) error {
		return serve(c, r)
	})
	go func() {
		e.Logger.Fatal(e.Start(":8888"))
	}()

	return nil
}

func addDefaults(api gitv1.GitopsAPI) gitv1.GitopsAPI {
	if api.Spec.Base == "" {
		api.Spec.Base = "master"
	}
	if api.Spec.Branch == "" {
		api.Spec.Branch = api.Spec.Base
	}
	if api.Spec.Kustomization == "" {
		api.Spec.Kustomization = "kustomization.yaml"
	}
	return api
}

func getContentPath(api gitv1.GitopsAPI, repoRoot string, obj unstructured.Unstructured) (contentPath string, err error) {
	if api.Spec.SearchPath != "" {
		objKey := getObjectKey(obj)
		if strings.HasSuffix(repoRoot, "/") {
			api.Spec.SearchPath = repoRoot + api.Spec.SearchPath
		} else {
			api.Spec.SearchPath = repoRoot + "/" + api.Spec.SearchPath
		}
		if err := filepath.Walk(api.Spec.SearchPath, func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Name() == "kustomization.yaml" || info.IsDir() {
				return nil
			}
			if path.Ext(filePath) == ".yaml" || path.Ext(filePath) == ".yml" {
				resource := unstructured.Unstructured{Object: make(map[string]interface{})}
				buf, err := ioutil.ReadFile(filePath)
				if err != nil {
					return err
				}
				if err := yaml.Unmarshal(buf, &resource); err != nil {
					return err
				}
				resourceKey := fmt.Sprintf("%s-%s-%s", resource.GetName(), resource.GetNamespace(), resource.GetKind())
				if objKey == resourceKey {
					contentPath, err = filepath.Rel(repoRoot, filePath)
					if err != nil {
						return err
					}
					return nil
				}
			}
			return nil
		}); err != nil {
			return "", errors.WithStack(err)
		}
	} else {
		if api.Spec.Path != "" {
			contentPath = api.Spec.Path
		}
	}
	return contentPath, nil
}

func createCommit(ctx context.Context, api gitv1.GitopsAPI, git connectors.Connector, work *gitv5.Worktree, title string) (hash string, err error) {
	author := &object.Signature{
		Name:  api.Spec.GitUser,
		Email: api.Spec.GitEmail,
		When:  time.Now(),
	}
	if author.Name == "" {
		author.Name = "Git Operator"
	}
	if author.Email == "" {
		author.Email = "noreply@git-operator"
	}
	_hash, err := work.Commit(title, &gitv5.CommitOptions{
		Author: author,
		All:    true,
	})

	if err != nil {
		return
	}
	hash = _hash.String()

	if err = git.Push(ctx, fmt.Sprintf("%s:%s", api.Spec.Branch, api.Spec.Base)); err != nil {
		return
	}
	return
}

func createPullRequest(ctx context.Context, api gitv1.GitopsAPI, obj unstructured.Unstructured, git connectors.Connector) (pr int, err error) {
	api.Spec.PullRequest.Body, err = text.Template(api.Spec.PullRequest.Body, obj.Object)
	if err != nil {
		return
	}
	api.Spec.PullRequest.Title, err = text.Template(api.Spec.PullRequest.Title, obj.Object)
	if err != nil {
		return
	}
	pr, err = git.OpenPullRequest(ctx, api.Spec.Base, api.Spec.Branch, api.Spec.PullRequest)
	return
}

func getObjectKey(obj unstructured.Unstructured) string {
	return fmt.Sprintf("%s-%s-%s", obj.GetName(), obj.GetNamespace(), obj.GetKind())
}
