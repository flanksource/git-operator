package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/flanksource/commons/console"
	"github.com/flanksource/commons/utils"
	gitv1 "github.com/flanksource/git-operator/api/v1"
	"github.com/flanksource/git-operator/connectors"
	"github.com/flanksource/git-operator/controllers"
	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	zapu "go.uber.org/zap"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	crdclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	namespace  = "platform-system"
	repository = "flanksource/git-operator-test"
	owner      = "flanksource"
	repoName   = "git-operator-test"
)

var (
	k8s    *kubernetes.Clientset
	crdK8s crdclient.Client
	tests  = map[string]Test{
		"git-operator-is-running":           TestGitOperatorIsRunning,
		"github-gitops-api-create":          TestGitopsAPICreate,
		"github-gitops-api-update":          TestGitopsAPIOrUpdate,
		"github-gitops-api-delete-multiple": TestGitopsAPIDeleteMultiple,
		"github-gitops-api-delete":          TestGitopsAPIDelete,
	}
	scheme              = runtime.NewScheme()
	log                 = ctrl.Log.WithName("e2e")
	pullRequestUsername string
)

type Test func(context.Context, *console.TestResults) error
type DeferFunc func()

func init() {
	pullRequestUsername = os.Getenv("GITHUB_USERNAME")
}

func main() {
	var timeout *time.Duration
	var err error
	ctrl.SetLogger(zap.New(zap.UseDevMode(true), zap.Level(zapu.DebugLevel)))

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.ExpandEnv("$HOME/.kube/config")
	}

	data, err := ioutil.ReadFile(kubeconfig)
	if err != nil {
		log.Error(err, "failed to read kubeconfig")
		os.Exit(1)
	}
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		log.Error(err, "failed to create clientset")
		os.Exit(1)
	}

	timeout = flag.Duration("timeout", 15*time.Minute, "Global timeout for all tests")
	flag.Parse()

	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(gitv1.AddToScheme(scheme))

	if err != nil {
		log.Error(err, "failed to create k8s config")
		os.Exit(1)
	}

	k8s, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Error(err, "failed to create clientset")
		os.Exit(1)
	}

	mapper, err := apiutil.NewDynamicRESTMapper(restConfig)
	if err != nil {
		log.Error(err, "failed to create mapper")
		os.Exit(1)
	}
	if crdK8s, err = crdclient.New(restConfig, crdclient.Options{Scheme: scheme, Mapper: mapper}); err != nil {
		log.Error(err, "failed to create mapper")
		os.Exit(1)
	}

	test := &console.TestResults{
		Writer: os.Stdout,
	}

	errors := map[string]error{}
	deadline, cancelFunc := context.WithTimeout(context.Background(), *timeout)
	defer cancelFunc()

	for name, t := range tests {
		log.Info("testing", "name", name)
		err := t(deadline, test)
		if err != nil {
			errors[name] = err
		}
	}

	if len(errors) > 0 {
		for _, err := range errors {
			log.Info(err.Error())
		}
		os.Exit(1)
	}

	log.Info("All tests passed !!!")
}

func TestGitopsAPICreate(ctx context.Context, test *console.TestResults) error {
	git, err := connectors.NewConnector(ctx, crdK8s, k8s, log, "platform-system", "https://github.com/"+repository, &v1.LocalObjectReference{
		Name: "github",
	})
	if err != nil {
		return err
	}

	body := fmt.Sprintf(`
	[
	{
		"apiVersion": "acmp.corp/v1",
		"kind": "NamespaceRequest",
		"metadata": {
			"name": "%s"
		},
		"spec": {
			"cpu": 4,
			"memory": 32
		 }
	}
	]
	`, getBranchName("test"))

	log.Info("json", "value", body)
	api := &gitv1.GitopsAPI{
		Spec: gitv1.GitopsAPISpec{
			GitRepository: repository,
			PullRequest: &gitv1.PullRequestTemplate{
				Title: "Automated PR: Created new object {{.metadata.name}}",
				Body:  "Somebody created a new PR {{.metadata.name}}",
			},
		},
	}
	work, title, err := controllers.CreateOrUpdateObject(ctx, log, git, api, bytes.NewReader([]byte(body)), "application/json")
	if err != nil {
		return err
	}
	_, err = controllers.CreateCommit(api, work, title)
	if err != nil {
		return err
	}

	if err = git.Push(ctx, fmt.Sprintf("%s:%s", api.Spec.Branch, api.Spec.Base)); err != nil {
		return err
	}
	pr, err := git.OpenPullRequest(ctx, api.Spec.Base, api.Spec.Branch, api.Spec.PullRequest)
	if err != nil {
		return err
	}
	if pr != 0 {
		if err = git.ClosePullRequest(ctx, pr); err != nil {
			return err
		}
	}
	return err
}

func TestGitopsAPIOrUpdate(ctx context.Context, test *console.TestResults) error {
	git, err := connectors.NewConnector(ctx, crdK8s, k8s, log, "platform-system", "https://github.com/"+repository, &v1.LocalObjectReference{
		Name: "github",
	})
	if err != nil {
		return err
	}
	branchName := getBranchName("test")
	body := `
	[
		{
			"apiVersion": "v1",
			"data": {
				"some-key": "some-value",
				"new-key": "new-value"
			},
			"kind": "ConfigMap",
			"metadata": {
				"name": "test-configmap",
				"namespace": "default"
			}
		},
		{
		  "apiVersion": "acmp.corp/v1",
		  "kind": "NamespaceRequest",
		  "metadata": {
			"name": "tenant8"
		  },
		  "spec": {
			"cluster": "dev01",
			"memory": 11,
			"some-new-key": "test-value"
		  }
		},
		{
		  "apiVersion": "v1",
		  "data": {
			"some-key": "some-value"
		  },
		  "kind": "ConfigMap",
		  "metadata": {
			"name": "sample-configmap"
		  }
		}
	]
	`
	api := &gitv1.GitopsAPI{
		Spec: gitv1.GitopsAPISpec{
			GitRepository: repository,
			Branch:        branchName,
			SearchPath:    "resources/",
			Kustomization: "resources/kustomization.yaml",
			PullRequest: &gitv1.PullRequestTemplate{
				Title: "Updating/Creating multiple objects",
				Body:  "Somebody created a new PR {{.metadata.name}}",
			},
		},
	}
	log.Info("json", "value", body)
	work, title, err := controllers.CreateOrUpdateObject(ctx, log, git, api, bytes.NewReader([]byte(body)), "application/json")
	if err != nil {
		return err
	}
	_, err = controllers.CreateCommit(api, work, title)
	if err != nil {
		return err
	}

	if err = git.Push(ctx, fmt.Sprintf("%s:%s", api.Spec.Branch, api.Spec.Base)); err != nil {
		return err
	}
	pr, err := git.OpenPullRequest(ctx, api.Spec.Base, api.Spec.Branch, api.Spec.PullRequest)
	if err != nil {
		return err
	}
	if pr != 0 {
		if err := git.ClosePullRequest(ctx, pr); err != nil {
			return err
		}
	}
	return err
}

func TestGitopsAPIDelete(ctx context.Context, test *console.TestResults) error {
	git, err := connectors.NewConnector(ctx, crdK8s, k8s, log, "platform-system", "https://github.com/"+repository, &v1.LocalObjectReference{
		Name: "github",
	})
	if err != nil {
		return err
	}
	branchName := getBranchName("test")
	body := `
	[
		{
		  "apiVersion": "v1",
		  "data": {
			"some-key": "some-value"
		  },
		  "kind": "ConfigMap",
		  "metadata": {
			"name": "some-configmap"
		  }
		}
	]
	`
	log.Info("json", "value", body)
	api := &gitv1.GitopsAPI{
		Spec: gitv1.GitopsAPISpec{
			GitRepository: repository,
			Branch:        branchName,
			SearchPath:    "resources/",
			PullRequest: &gitv1.PullRequestTemplate{
				Title: "Automated PR: Delete single object",
				Body:  "Somebody created a new PR {{.metadata.name}}",
			},
		},
	}
	work, title, err := controllers.DeleteObject(ctx, log, git, api, bytes.NewReader([]byte(body)), "application/json")
	if err != nil {
		return err
	}
	_, err = controllers.CreateCommit(api, work, title)
	if err != nil {
		return err
	}

	if err = git.Push(ctx, fmt.Sprintf("%s:%s", api.Spec.Branch, api.Spec.Base)); err != nil {
		return err
	}
	pr, err := git.OpenPullRequest(ctx, api.Spec.Base, api.Spec.Branch, api.Spec.PullRequest)
	if err != nil {
		return err
	}
	if pr != 0 {
		if err := git.ClosePullRequest(ctx, pr); err != nil {
			return err
		}
	}
	return err
}

func TestGitopsAPIDeleteMultiple(ctx context.Context, test *console.TestResults) error {
	git, err := connectors.NewConnector(ctx, crdK8s, k8s, log, "platform-system", "https://github.com/"+repository, &v1.LocalObjectReference{
		Name: "github",
	})
	if err != nil {
		return err
	}
	branchName := getBranchName("test")
	body := `
	[
		{
		  "apiVersion": "v1",
		  "data": {
			"some-key": "some-value"
		  },
		  "kind": "ConfigMap",
		  "metadata": {
			"name": "some-configmap"
		  }
		},
		{
		  "apiVersion": "acmp.corp/v1",
		  "kind": "NamespaceRequest",
		  "metadata": {
			"name": "tenant8"
		  },
		  "spec": {
			"cluster": "dev01",
			"memory": 11,
			"some-new-key": "test-value"
		  }
		}
	]
	`
	log.Info("json", "value", body)
	api := &gitv1.GitopsAPI{
		Spec: gitv1.GitopsAPISpec{
			GitRepository: repository,
			Branch:        branchName,
			SearchPath:    "resources/",
			PullRequest: &gitv1.PullRequestTemplate{
				Title: "Automated PR: Delete multiple objects",
				Body:  "Somebody created a new PR {{.metadata.name}}",
			},
		},
	}
	work, title, err := controllers.DeleteObject(ctx, log, git, api, bytes.NewReader([]byte(body)), "application/json")
	if err != nil {
		return err
	}
	_, err = controllers.CreateCommit(api, work, title)
	if err != nil {
		return err
	}

	if err = git.Push(ctx, fmt.Sprintf("%s:%s", api.Spec.Branch, api.Spec.Base)); err != nil {
		return err
	}
	pr, err := git.OpenPullRequest(ctx, api.Spec.Base, api.Spec.Branch, api.Spec.PullRequest)
	if err != nil {
		return err
	}
	if pr != 0 {
		if err := git.ClosePullRequest(ctx, pr); err != nil {
			return err
		}
	}
	return err
}

func TestGitOperatorIsRunning(ctx context.Context, test *console.TestResults) error {
	pods, err := k8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "control-plane=git-operator"})
	if err != nil {
		test.Failf("TestGitOperatorIsRunning", "failed to list git-operator pods: %v", err)
		return err
	}
	if len(pods.Items) != 1 {
		test.Failf("TestGitOperatorIsRunning", "expected 1 pod got %d", len(pods.Items))
		return errors.Errorf("Expected 1 pod got %d", len(pods.Items))
	}
	test.Passf("TestGitOperatorIsRunning", "%s pod is running", pods.Items[0].Name)
	return nil
}

func assertEquals(test *console.TestResults, name, actual, expected string) error { // nolint: unparam
	if actual != expected {
		test.Failf(name, "expected %s to equal %s", actual, expected)
		return errors.Errorf("Test %s expected %s to equal %s", name, actual, expected)
	}
	return nil
}

func assertInterfaceEquals(test *console.TestResults, name string, actual, expected interface{}) error {
	actualYml, err := yaml.Marshal(actual)
	if err != nil {
		return errors.Wrap(err, "failed to marshal actual")
	}

	expectedYml, err := yaml.Marshal(expected)
	if err != nil {
		return errors.Wrap(err, "failed to marshal expected")
	}

	if string(actualYml) != string(expectedYml) {
		test.Failf("Test %s expected: %s\n\nTo Equal:\n%s\n", name, string(actualYml), string(expectedYml))
		return errors.Errorf("Test %s expected:\n%s\nTo Match:\n%s\n", name, actualYml, expectedYml)
	}

	return nil
}

func getBranchName(baseName string) string {
	date := time.Now().Format("20060201150405")
	hash := utils.RandomString(4)
	return fmt.Sprintf("%s-%s-%s", baseName, date, hash)
}

func githubClient(ctx context.Context) (*github.Client, error) {
	authToken := os.Getenv("GITHUB_TOKEN")
	if authToken == "" {
		return nil, errors.New("GITHUB_TOKEN not provided")
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: authToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	return client, nil
}
