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

// GitBranchSpec defines the desired state of GitBranch
type GitBranchSpec struct {
	Repository string `json:"repository,omitempty"`
	BranchName string `json:"branchName"`
}

// GitBranchStatus defines the observed state of GitBranch
type GitBranchStatus struct {
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
	// The Git SHA1 of the main/master branch
	Head string `json:"head,omitempty"`
}

// +kubebuilder:object:root=true

// GitBranch is the Schema for the gitbranches API
type GitBranch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitBranchSpec   `json:"spec,omitempty"`
	Status GitBranchStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitBranchList contains a list of GitBranch
type GitBranchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitBranch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitBranch{}, &GitBranchList{})
}
