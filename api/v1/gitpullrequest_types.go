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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GitPullRequestSpec defines the desired state of GitPullRequest
type GitPullRequestSpec struct {
	// Full repository name including owner
	Repository string `json:"repository,omitempty"`
	ID         string `json:"id,omitempty"`
	SHA        string `json:"sha,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Fork       string `json:"fork,omitempty"`

	// Update to add or remove reviewers from the pull request
	Reviewers []string `json:"reviewers,omitempty"`
}

// GitPullRequestStatus defines the observed state of GitPullRequest
type GitPullRequestStatus struct {
	URL       string          `json:"url,omitempty"`
	Author    string          `json:"author,omitempty"`
	Approvers map[string]bool `json:"approvers,omitempty"`
}

type CheckStatus string

const CheckStatusPassed CheckStatus = "passed"
const CheckStatusFailed CheckStatus = "failed"
const CheckStatusRunning CheckStatus = "running"
const CheckStatusQueued CheckStatus = "queued"
const CheckStatusPending CheckStatus = "pending"

type Check struct {
	Name        string          `json:"name,omitempty"`
	Status      CheckStatus     `json:"status,omitempty"`
	Description string          `json:"description,omitempty"`
	Duration    metav1.Duration `json:"duration,omitempty"`
	Started     metav1.Time     `json:"started,omitempty"`
	Ended       metav1.Time     `json:"ended,omitempty"`
}

// +kubebuilder:object:root=true

// GitPullRequest is the Schema for the gitpullrequests API
type GitPullRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitPullRequestSpec   `json:"spec,omitempty"`
	Status GitPullRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitPullRequestList contains a list of GitPullRequest
type GitPullRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitPullRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitPullRequest{}, &GitPullRequestList{})
}
