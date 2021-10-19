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

	"github.com/gosimple/slug"
	"github.com/labstack/gommon/random"
	"github.com/pkg/errors"

	"github.com/flanksource/kommons"
	"github.com/go-git/go-billy/v5"
	gitv5 "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-logr/logr"
	"github.com/imdario/mergo"
	"github.com/labstack/echo"
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
	getObj := strings.HasPrefix(c.Path(), "/_get")
	if token == "" {
		token = c.QueryParam("token")
	}
	if token == "" {
		token = c.Request().Header.Get("Authorization")
	}
	contentType := c.Request().Header.Get("Content-Type")
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
	var work *gitv5.Worktree
	var title, hash string
	var pr int
	if deleteObj {
		work, title, err = DeleteObject(ctx, r.Log, git, &api, c.Request().Body, contentType)
	} else if getObj {
		kind := c.QueryParam("kind")
		name := c.QueryParam("name")
		namespace := c.QueryParam("namespace")
		accept := c.Request().Header.Get("Accept")
		if accept == "" {
			accept = "application/json"
		}

		body, err := GetObject(ctx, c.Request(), r.Log, git, &api, accept, kind, name, namespace)
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}
		c.Response().Header().Set("Content-Type", accept)
		return c.String(http.StatusOK, string(body))
	} else {
		work, title, err = CreateOrUpdateObject(ctx, r.Log, git, &api, c.Request().Body, contentType)
	}
	if err != nil {
		r.Log.Error(err, "error updating files")
		return c.String(http.StatusInternalServerError, err.Error())
	}
	hash, err = CreateCommit(&api, work, title)
	if err != nil {
		r.Log.Error(err, "error creating commit")
		return c.String(http.StatusInternalServerError, err.Error())
	}
	if err = git.Push(ctx, fmt.Sprintf("%s:%s", api.Spec.Branch, api.Spec.Base)); err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}

	if api.Spec.PullRequest != nil {
		pr, err = git.OpenPullRequest(ctx, api.Spec.Base, api.Spec.Branch, api.Spec.PullRequest)
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}
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
	existingKustomization, err := ioutil.ReadAll(existing)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(existingKustomization, &kustomization); err != nil {
		return nil, err
	}
	return &kustomization, nil
}

func CreateOrUpdateObject(ctx context.Context, logger logr.Logger, git connectors.Connector, api *gitv1.GitopsAPI, contents io.Reader, contentType string) (work *gitv5.Worktree, title string, err error) {
	addDefaults(api)
	body, err := ioutil.ReadAll(contents)
	if err != nil {
		return
	}
	body = []byte(TabToSpace(string(body)))
	var objs []*unstructured.Unstructured
	if strings.Contains(contentType, "yaml") || strings.Contains(contentType, "yml") {
		objs, err = kommons.GetUnstructuredObjects(body)
		if err != nil {
			return
		}
	} else {
		objs, err = kommons.GetUnstructuredObjectsFromJson(body)
		if err != nil {
			return
		}
	}
	fs, work, err := git.Clone(ctx, api.Spec.Base, api.Spec.Branch)
	if err != nil {
		return nil, "", err
	}
	var contentPaths map[string]string
	if api.Spec.SearchPath != "" {
		contentPaths, err = getContentPaths(fs.Root(), api.Spec.SearchPath)
		if err != nil {
			return nil, "", err
		}
	}
	title = "Add/Update "
	for _, obj := range objs {
		if err = templateAPIObject(api, obj); err != nil {
			return
		}
		contentPath, err := getContentPath(api, obj, contentPaths)
		if err != nil {
			return nil, "", err
		}
		if contentPath == "" {
			// need to create a new file with the content
			contentPath = filepath.Join(api.Spec.SearchPath, fmt.Sprintf("%s-%s-%s.yaml", obj.GetKind(), obj.GetNamespace(), obj.GetName()))
			body, err = yaml.Marshal(obj)
			if err != nil {
				return nil, "", err
			}
		} else {
			// file already exists performing merge
			body, err = performStrategicMerge(filepath.Join(fs.Root(), contentPath), obj)
			if err != nil {
				return nil, "", err
			}
		}
		title = title + fmt.Sprintf("%s/%s/%s ", obj.GetKind(), obj.GetNamespace(), obj.GetName())
		logger.Info("Received", "name", api.GetName(), "namespace", api.GetNamespace(), "object", title)
		logger.Info("Saving to", "path", contentPath, "kustomization", api.Spec.Kustomization, "object", title)
		kustomization, err := GetKustomizaton(fs, api.Spec.Kustomization)
		if err != nil {
			return nil, "", err
		}
		relativePath := strings.Replace(contentPath, path.Dir(api.Spec.Kustomization)+"/", "", -1)
		if err = copy(body, contentPath, fs, work); err != nil {
			return nil, "", err
		}
		index := findElement(kustomization.Resources, relativePath)
		if index == -1 {
			kustomization.Resources = append(kustomization.Resources, relativePath)
		}
		existingKustomization, err := yaml.Marshal(kustomization)
		if err != nil {
			return nil, "", err
		}
		if err = copy(existingKustomization, api.Spec.Kustomization, fs, work); err != nil {
			return nil, "", err
		}
	}

	return work, title, nil
}

func DeleteObject(ctx context.Context, logger logr.Logger, git connectors.Connector, api *gitv1.GitopsAPI, contents io.Reader, contentType string) (work *gitv5.Worktree, title string, err error) {
	addDefaults(api)
	body, err := ioutil.ReadAll(contents)
	if err != nil {
		return
	}
	body = []byte(TabToSpace(string(body)))
	var objs []*unstructured.Unstructured
	if strings.Contains(contentType, "yaml") || strings.Contains(contentType, "yml") {
		objs, err = kommons.GetUnstructuredObjects(body)
		if err != nil {
			return
		}
	} else {
		objs, err = kommons.GetUnstructuredObjectsFromJson(body)
		if err != nil {
			return
		}
	}
	fs, work, err := git.Clone(ctx, api.Spec.Base, api.Spec.Branch)
	if err != nil {
		return
	}
	var contentPaths map[string]string
	if api.Spec.SearchPath != "" {
		contentPaths, err = getContentPaths(fs.Root(), api.Spec.SearchPath)
		if err != nil {
			return nil, "", err
		}
	}
	title = "Delete "
	for _, obj := range objs {
		if err = templateAPIObject(api, obj); err != nil {
			return nil, "", err
		}
		contentPath, err := getContentPath(api, obj, contentPaths)
		if err != nil {
			return nil, "", err
		}
		if contentPath == "" {
			return nil, "", fmt.Errorf("could not find the object %v to delete", getObjectKey(obj))
		}
		title = title + fmt.Sprintf("%s/%s/%s ", obj.GetKind(), obj.GetNamespace(), obj.GetName())
		logger.Info("Received", "name", api.GetName(), "namespace", api.GetNamespace(), "object", title)

		logger.Info("Saving to", "path", contentPath, "kustomization", api.Spec.Kustomization, "object", title)
		kustomization, err := GetKustomizaton(fs, api.Spec.Kustomization)
		if err != nil {
			return nil, "", err
		}
		relativePath := strings.Replace(contentPath, path.Dir(api.Spec.Kustomization)+"/", "", -1)
		body, err = deleteObjectFromFile(filepath.Join(fs.Root(), contentPath), obj)
		if err != nil {
			return nil, "", err
		}
		if err = copy(body, contentPath, fs, work); err != nil {
			return nil, "", err
		}
		delete, err := isFileEmpty(body)
		if err != nil {
			return nil, "", err
		}
		if delete {
			if err = deleteFile(contentPath, work, fs.Root()); err != nil {
				return nil, "", err
			}
			index := findElement(kustomization.Resources, relativePath)
			if index != -1 {
				kustomization.Resources = removeElement(kustomization.Resources, index)
				existingKustomization, err := yaml.Marshal(kustomization)
				if err != nil {
					return nil, "", err
				}
				if err = copy(existingKustomization, api.Spec.Kustomization, fs, work); err != nil {
					return nil, "", err
				}
			}
		}
	}
	return work, title, nil
}

func GetObject(ctx context.Context, req *http.Request, logger logr.Logger, git connectors.Connector, api *gitv1.GitopsAPI, contentType, kind, name, namespace string) ([]byte, error) {
	obj, err := GetObjectWithKindNameNamespace(ctx, logger, git, api, kind, name, namespace)
	if err != nil {
		return nil, err
	}
	if contentType == "application/json" {
		body, err := json.Marshal(obj.Object)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal json")
		}
		return body, nil
	} else if contentType == "application/yaml" {
		body, err := yaml.Marshal(obj.Object)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal json")
		}
		return body, nil
	}

	return nil, errors.Errorf("Content-Type %s not supproted", contentType)
}

func GetObjectWithKindNameNamespace(ctx context.Context, logger logr.Logger, git connectors.Connector, api *gitv1.GitopsAPI, kind, name, namespace string) (*unstructured.Unstructured, error) {
	addDefaults(api)
	fs, _, err := git.Clone(ctx, api.Spec.Base, api.Spec.Branch)
	if err != nil {
		return nil, errors.Wrap(err, "failed to clone repository")
	}
	var contentPaths map[string]string
	if api.Spec.SearchPath != "" {
		contentPaths, err = getContentPaths(fs.Root(), api.Spec.SearchPath)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get content paths")
		}
	}

	fakeObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}

	contentPath, err := getContentPath(api, fakeObj, contentPaths)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get content path for %s/%s/%s", kind, namespace, name)
	}
	if contentPath == "" {
		return nil, fmt.Errorf("could not find the object %s/%s/%s", kind, namespace, name)
	}

	obj, err := getObjectFromFile(filepath.Join(fs.Root(), contentPath), fakeObj)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get object from file")
	}

	return obj, nil
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
	e.GET("/_get/:namespace/:name/:token", func(c echo.Context) error {
		return serve(c, r)
	})
	go func() {
		e.Logger.Fatal(e.Start(":8888"))
	}()

	return nil
}

func addDefaults(api *gitv1.GitopsAPI) {
	if api.Spec.Base == "" {
		api.Spec.Base = "master"
	}
	if api.Spec.Branch == "" {
		if api.Spec.PullRequest != nil {
			api.Spec.Branch = slug.Make(api.Spec.PullRequest.Title) + "-" + random.String(4)
		} else {
			api.Spec.Branch = api.Spec.Base
		}
	}
	if api.Spec.Kustomization == "" {
		if api.Spec.SearchPath != "" {
			api.Spec.Kustomization = filepath.Join(api.Spec.SearchPath, "kustomization.yaml")
		} else {
			api.Spec.Kustomization = "kustomization.yaml"
		}
	}
}

func templateAPIObject(api *gitv1.GitopsAPI, obj *unstructured.Unstructured) (err error) {
	api.Spec.Kustomization, err = text.Template(api.Spec.Kustomization, obj.Object)
	if err != nil {
		return
	}
	api.Spec.PullRequest.Body, err = text.Template(api.Spec.PullRequest.Body, obj.Object)
	if err != nil {
		return
	}
	api.Spec.PullRequest.Title, err = text.Template(api.Spec.PullRequest.Title, obj.Object)
	if err != nil {
		return
	}
	return nil
}

func getContentPath(api *gitv1.GitopsAPI, obj *unstructured.Unstructured, contentPaths map[string]string) (contentPath string, err error) {
	if api.Spec.SearchPath != "" {
		contentPath = contentPaths[getObjectKey(obj)]
	} else {
		if api.Spec.Path != "" {
			contentPath = api.Spec.Path
		}
	}
	contentPath, err = text.Template(contentPath, obj.Object)
	if err != nil {
		return "", err
	}
	return contentPath, nil
}

func CreateCommit(api *gitv1.GitopsAPI, work *gitv5.Worktree, title string) (hash string, err error) {
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
	return
}

func getObjectKey(obj *unstructured.Unstructured) string {
	return fmt.Sprintf("%s-%s-%s", obj.GetName(), obj.GetNamespace(), obj.GetKind())
}

func performStrategicMerge(file string, obj *unstructured.Unstructured) (body []byte, err error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	fileObjs, err := kommons.GetUnstructuredObjects(data)
	if err != nil {
		return
	}
	index := getObjectIndex(obj, fileObjs)
	oldObj, err := yaml.Marshal(fileObjs[index])
	if err != nil {
		return nil, err
	}
	err = mergo.Merge(&fileObjs[index].Object, obj.Object, mergo.WithOverride)
	if err != nil {
		return nil, err
	}
	newObj, err := yaml.Marshal(fileObjs[index])
	if err != nil {
		return nil, err
	}
	body = []byte(strings.Replace(string(data), string(oldObj), string(newObj), -1))
	return
}

func getObjectFromFile(file string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	fileObjs, err := kommons.GetUnstructuredObjects(data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get unstructured objects")
	}
	index := getObjectIndex(obj, fileObjs)
	if index == -1 {
		return nil, errors.Errorf("object %s/%s/%s not found", obj.GetKind(), obj.GetNamespace(), obj.GetKind())
	}
	return fileObjs[index], nil
}

func deleteObjectFromFile(file string, obj *unstructured.Unstructured) (body []byte, err error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	fileObjs, err := kommons.GetUnstructuredObjects(data)
	if err != nil {
		return
	}
	index := getObjectIndex(obj, fileObjs)
	oldObj, err := yaml.Marshal(fileObjs[index])
	if err != nil {
		return nil, err
	}
	body = []byte(strings.Replace(string(data), string(oldObj), "", -1))
	return
}

func getObjectIndex(obj *unstructured.Unstructured, fileObjs []*unstructured.Unstructured) (index int) {
	index = -1
	for i, fileObj := range fileObjs {
		if getObjectKey(fileObj) == getObjectKey(obj) {
			index = i
			break
		}
	}
	return index
}

func isFileEmpty(data []byte) (bool, error) {
	fileObjs, err := kommons.GetUnstructuredObjects(data)
	if err != nil {
		return false, err
	}
	return len(fileObjs) == 0, nil
}

func getContentPaths(repoRoot, searchPath string) (map[string]string, error) {
	contentPaths := make(map[string]string)
	walkPath := filepath.Join(repoRoot, searchPath)
	if err := filepath.Walk(walkPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == "kustomization.yaml" || info.IsDir() {
			return nil
		}
		if path.Ext(filePath) == ".yaml" || path.Ext(filePath) == ".yml" {
			buf, err := ioutil.ReadFile(filePath)
			if err != nil {
				return err
			}
			resources, err := kommons.GetUnstructuredObjects(buf)
			if err != nil {
				return err
			}
			for _, resource := range resources {
				contentPaths[getObjectKey(resource)], err = filepath.Rel(repoRoot, filePath)
				if err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return contentPaths, nil
}
