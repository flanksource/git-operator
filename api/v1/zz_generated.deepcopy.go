//+build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GitopsAPI) DeepCopyInto(out *GitopsAPI) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GitopsAPI.
func (in *GitopsAPI) DeepCopy() *GitopsAPI {
	if in == nil {
		return nil
	}
	out := new(GitopsAPI)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GitopsAPI) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GitopsAPIList) DeepCopyInto(out *GitopsAPIList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]GitopsAPI, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GitopsAPIList.
func (in *GitopsAPIList) DeepCopy() *GitopsAPIList {
	if in == nil {
		return nil
	}
	out := new(GitopsAPIList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GitopsAPIList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GitopsAPISpec) DeepCopyInto(out *GitopsAPISpec) {
	*out = *in
	if in.PullRequest != nil {
		in, out := &in.PullRequest, &out.PullRequest
		*out = new(PullRequestTemplate)
		(*in).DeepCopyInto(*out)
	}
	if in.SecretRef != nil {
		in, out := &in.SecretRef, &out.SecretRef
		*out = new(corev1.LocalObjectReference)
		**out = **in
	}
	if in.TokenRef != nil {
		in, out := &in.TokenRef, &out.TokenRef
		*out = new(corev1.LocalObjectReference)
		**out = **in
	}
	if in.Reviewers != nil {
		in, out := &in.Reviewers, &out.Reviewers
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GitopsAPISpec.
func (in *GitopsAPISpec) DeepCopy() *GitopsAPISpec {
	if in == nil {
		return nil
	}
	out := new(GitopsAPISpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GitopsAPIStatus) DeepCopyInto(out *GitopsAPIStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GitopsAPIStatus.
func (in *GitopsAPIStatus) DeepCopy() *GitopsAPIStatus {
	if in == nil {
		return nil
	}
	out := new(GitopsAPIStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PullRequestTemplate) DeepCopyInto(out *PullRequestTemplate) {
	*out = *in
	if in.Reviewers != nil {
		in, out := &in.Reviewers, &out.Reviewers
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Assignees != nil {
		in, out := &in.Assignees, &out.Assignees
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Tags != nil {
		in, out := &in.Tags, &out.Tags
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PullRequestTemplate.
func (in *PullRequestTemplate) DeepCopy() *PullRequestTemplate {
	if in == nil {
		return nil
	}
	out := new(PullRequestTemplate)
	in.DeepCopyInto(out)
	return out
}
