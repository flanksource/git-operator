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

// GitDeploymentSpec defines the desired state of GitDeployment
type GitDeploymentSpec struct {
	Ref         string `json:"ref,omitempty"`
	Sha         string `json:"sha,omitempty"`
	Name        string `json:"name,omitempty"`
	ID          string `json:"id,omitempty"`
	Environment string `json:"environment,omitempty"`
	Description string `json:"description,omitempty"`
	AutoMerge   bool   `json:"autoMerge,omitempty"`
}

// GitDeploymentStatus defines the observed state of GitDeployment
type GitDeploymentStatus struct {
	Ref            string `json:"ref,omitempty"`
	Sha            string `json:"sha,omitempty"`
	ID             string `json:"id,omitempty"`
	Name           string `json:"name,omitempty"`
	Environment    string `json:"environment,omitempty"`
	DeploymentLink string `json:"deploymentLink,omitempty"`
	StatusLink     string `json:"statusLink,omitempty"`
}

// +kubebuilder:object:root=true

// GitDeployment is the Schema for the gitdeployments API
type GitDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitDeploymentSpec   `json:"spec,omitempty"`
	Status GitDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitDeploymentList contains a list of GitDeployment
type GitDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitDeployment{}, &GitDeploymentList{})
}
