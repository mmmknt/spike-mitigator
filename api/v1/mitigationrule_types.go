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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MitigationRuleSpec defines the desired state of MitigationRule
type MitigationRuleSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of MitigationRule. Edit MitigationRule_types.go to remove/update
	ExternalHost string `json:"externalHost,omitempty"`
	InternalHost string `json:"internalHost,omitempty"`
	HPAUpperLimit int `json:"hpaUpperLimit,omitempty"`
	ddApiKey string `json:"ddApiKey,omitempty"`
	ddAppKey string `json:"ddAppKey,omitempty"`
}

// MitigationRuleStatus defines the observed state of MitigationRule
type MitigationRuleStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MitigationRule is the Schema for the mitigationrules API
// +kubebuilder:subresource:status
type MitigationRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MitigationRuleSpec   `json:"spec,omitempty"`
	Status MitigationRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MitigationRuleList contains a list of MitigationRule
type MitigationRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MitigationRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MitigationRule{}, &MitigationRuleList{})
}