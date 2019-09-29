/*

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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VultrClusterSpec defines the desired state of VultrCluster
type VultrClusterSpec struct {
	// +kubebuilder:validation:Required

	// The Vultr Region (DCID) the cluster lives in.
	Region int `json:"region"`
}

// VultrClusterStatus defines the observed state of VultrCluster
type VultrClusterStatus struct {
	Ready bool `json:"ready"`
	// +optional
	APIEndpoints []APIEndpoint `json:"apiEndpoints,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// VultrCluster is the Schema for the vultrclusters API
type VultrCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VultrClusterSpec   `json:"spec,omitempty"`
	Status VultrClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VultrClusterList contains a list of VultrCluster
type VultrClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VultrCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VultrCluster{}, &VultrClusterList{})
}
