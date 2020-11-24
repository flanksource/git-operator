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

package main

import (
	"flag"
	"os"
	"time"

	zapu "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/flanksource/commons/logger"
	gitv1 "github.com/flanksource/git-operator/api/v1"
	"github.com/flanksource/git-operator/controllers"
	"github.com/flanksource/kommons"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	logLevel string
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = gitv1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func logLevelFromString(logLevel string) *zapu.AtomicLevel {
	var level zapcore.Level

	switch logLevel {
	case "debug":
		level = zapu.DebugLevel
	case "info":
		level = zapu.InfoLevel
	case "error":
		level = zapu.ErrorLevel
	default:
		level = zapu.ErrorLevel
	}

	ll := zapu.NewAtomicLevelAt(level)
	return &ll
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var syncPeriod time.Duration
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.DurationVar(&syncPeriod, "sync-period", 60*time.Second, "The resync period used to check Github for new resources")
	flag.StringVar(&logLevel, "log-level", "error", "Logging level: debug, info, error")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true), zap.Level(logLevelFromString(logLevel))))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "bc88107d.flanksource.com",
		SyncPeriod:         &syncPeriod,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "failed to create clientset")
		os.Exit(1)
	}

	kommonsClient := kommons.NewClient(mgr.GetConfig(), logger.StandardLogger())

	if err = (&controllers.GitRepositoryReconciler{
		Client:    mgr.GetClient(),
		Clientset: clientset,
		Log:       ctrl.Log.WithName("controllers").WithName("GitRepository"),
		Scheme:    mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitRepository")
		os.Exit(1)
	}
	if err = (&controllers.GitPullRequestReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("GitPullRequest"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitPullRequest")
		os.Exit(1)
	}
	if err = (&controllers.GitopsAPIReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("GitopsAPI"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GitopsAPI")
		os.Exit(1)
	}
	if err = (&controllers.GitOpsReconciler{
		Client:        mgr.GetClient(),
		Clientset:     clientset,
		KommonsClient: kommonsClient,
		Log:           ctrl.Log.WithName("controllers").WithName("Gitops"),
		Scheme:        mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Gitops")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
