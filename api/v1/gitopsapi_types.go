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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GitopsAPISpec defines the desired state of GitopsAPI
type GitopsAPISpec struct {
	// The repository URL, can be a HTTP or SSH address.
	// +kubebuilder:validation:Pattern="^(http|https|ssh)://"
	// +required
	GitRepository string   `json:"gitRepository,omitempty"`
	GitUser       string   `json:"gitUser,omitempty"`
	GitEmail      string   `json:"gitEmail,omitempty"`
	Tags          []string `json:"gitTags,omitempty"`
	Assignee      []string `json:"gitAssignee,omitempty"`
	Branch        string   `json:"branch,omitempty"`
	PullRequest   bool     `json:"pullRequest,omitempty"`

	// The secret name containing the Git credentials.
	// For SSH repositories the secret must contain SSH_PRIVATE_KEY, SSH_PRIVATE_KEY_PASSORD
	// For Github repositories it must contain GITHUB_TOKEN
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`

	// The secret name containing the static credential to authenticate agaist either
	// as a `Authorization: Bearer` header or as a `?token=` argument
	// Must contain a key called TOKEN
	// +optional
	TokenRef *corev1.LocalObjectReference `json:"tokenRef,omitempty"`

	// List of github users which should approve the namespace request
	Reviewers []string `json:"reviewers,omitempty"`

	// The path to a kustomization file to insert or remove the resource, can included templated values .e.g `specs/clusters/{{.cluster}}/kustomization.yaml`
	// +required
	Kustomization string `json:"kustomization,omitempty"`

	// The path to save the resource into, should including templating to make it unique per cluster/namespace/kind/name tuple e.g. `specs/clusters/{{.cluster}}/{{.name}}.yaml`
	// +required
	Path string `json:"path,omitempty"`
}

// GitopsAPIStatus defines the observed state of GitopsAPI
type GitopsAPIStatus struct {
}

// +kubebuilder:object:root=true

// GitopsAPI is the Schema for the gitopsapis API
type GitopsAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitopsAPISpec   `json:"spec,omitempty"`
	Status GitopsAPIStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitopsAPIList contains a list of GitopsAPI
type GitopsAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitopsAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitopsAPI{}, &GitopsAPIList{})
}
