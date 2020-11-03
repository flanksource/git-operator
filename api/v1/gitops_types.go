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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GitOpsSpec defines the desired state of GitOps
type GitOpsSpec struct {
	// The name of the gitops deployment, defaults to namespace name
	Name string `json:"name,omitempty"`
	// Do not scan container image registries to fill in the registry cache, implies `--git-read-only` (default: true)
	DisableScanning *bool `json:"disableScanning,omitempty"`
	// The namespace to deploy the GitOps operator into, if empty then it will be deployed cluster-wide into kube-system
	Namespace string `json:"namespace,omitempty"`
	// The URL to git repository to clone
	GitURL string `json:"gitUrl"`
	// The git branch to use (default: `master`)
	GitBranch string `json:"gitBranch,omitempty"`
	// The path with in the git repository to look for YAML in (default: `.`)
	GitPath string `json:"gitPath,omitempty"`
	// The frequency with which to fetch the git repository (default: `5m0s`)
	GitPollInterval string `json:"gitPollInterval,omitempty"`
	// The frequency with which to sync the manifests in the repository to the cluster (default: `5m0s`)
	SyncInterval string `json:"syncInterval,omitempty"`
	// The Kubernetes secret to use for cloning, if it does not exist it will be generated (default: `flux-$name-git-deploy`)
	GitKey string `json:"gitKey,omitempty"`
	// The contents of the known_hosts file to mount into Flux and helm-operator
	KnownHosts string `json:"knownHosts,omitempty"`
	// The contents of the ~/.ssh/config file to mount into Flux and helm-operator
	SSHConfig string `json:"sshConfig,omitempty"`
	// The version to use for flux (default: 1.9.0 )
	FluxVersion string `json:"fluxVersion,omitempty"`
	// The version to use for flux (default: 1.9.0 )
	HelmOperatorVersion string `json:"helmOperatorVersion,omitempty"`
	// a map of args to pass to flux without -- prepended. See [fluxd](https://docs.fluxcd.io/en/1.19.0/references/daemon/) for a full list
	Args map[string]string `json:"args,omitempty"`
}

// GitOpsStatus defines the observed state of GitOps
type GitOpsStatus struct {
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
}

// +kubebuilder:object:root=true

// GitOps is the Schema for the gitops API
type GitOps struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitOpsSpec   `json:"spec,omitempty"`
	Status GitOpsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitOpsList contains a list of GitOps
type GitOpsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitOps `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitOps{}, &GitOpsList{})
}
