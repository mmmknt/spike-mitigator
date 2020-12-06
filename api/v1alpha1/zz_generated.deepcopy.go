// +build !ignore_autogenerated

/*
Copyright 2020 mmmknt.

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

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BalancingRule) DeepCopyInto(out *BalancingRule) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BalancingRule.
func (in *BalancingRule) DeepCopy() *BalancingRule {
	if in == nil {
		return nil
	}
	out := new(BalancingRule)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BalancingRule) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BalancingRuleList) DeepCopyInto(out *BalancingRuleList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]BalancingRule, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BalancingRuleList.
func (in *BalancingRuleList) DeepCopy() *BalancingRuleList {
	if in == nil {
		return nil
	}
	out := new(BalancingRuleList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BalancingRuleList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BalancingRuleSpec) DeepCopyInto(out *BalancingRuleSpec) {
	*out = *in
	out.ExternalAuthorizationRef = in.ExternalAuthorizationRef
	out.HostInfoHeaderKeyRef = in.HostInfoHeaderKeyRef
	if in.MonitoredHPANames != nil {
		in, out := &in.MonitoredHPANames, &out.MonitoredHPANames
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	out.MetricsStoreSecretRef = in.MetricsStoreSecretRef
	out.MetricsCondition = in.MetricsCondition
	out.OptionalAuthorization = in.OptionalAuthorization
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BalancingRuleSpec.
func (in *BalancingRuleSpec) DeepCopy() *BalancingRuleSpec {
	if in == nil {
		return nil
	}
	out := new(BalancingRuleSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BalancingRuleStatus) DeepCopyInto(out *BalancingRuleStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BalancingRuleStatus.
func (in *BalancingRuleStatus) DeepCopy() *BalancingRuleStatus {
	if in == nil {
		return nil
	}
	out := new(BalancingRuleStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MetricsCondition) DeepCopyInto(out *MetricsCondition) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MetricsCondition.
func (in *MetricsCondition) DeepCopy() *MetricsCondition {
	if in == nil {
		return nil
	}
	out := new(MetricsCondition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MetricsStoreSecretRef) DeepCopyInto(out *MetricsStoreSecretRef) {
	*out = *in
	out.DDApiKeyRef = in.DDApiKeyRef
	out.DDAppKeyRef = in.DDAppKeyRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MetricsStoreSecretRef.
func (in *MetricsStoreSecretRef) DeepCopy() *MetricsStoreSecretRef {
	if in == nil {
		return nil
	}
	out := new(MetricsStoreSecretRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OptionalAuthorization) DeepCopyInto(out *OptionalAuthorization) {
	*out = *in
	out.KeyRef = in.KeyRef
	out.ValueRef = in.ValueRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OptionalAuthorization.
func (in *OptionalAuthorization) DeepCopy() *OptionalAuthorization {
	if in == nil {
		return nil
	}
	out := new(OptionalAuthorization)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SecretRef) DeepCopyInto(out *SecretRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SecretRef.
func (in *SecretRef) DeepCopy() *SecretRef {
	if in == nil {
		return nil
	}
	out := new(SecretRef)
	in.DeepCopyInto(out)
	return out
}