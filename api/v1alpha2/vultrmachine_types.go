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

// VultrMachineSpec defines the desired state of VultrMachine
type VultrMachineSpec struct {
	// PlanID is the id of Vultr VPS plan (VPSPLANID).
	PlanID int `json:"planId,omitempty"`

	// SSHKeyName is the name of the ssh key to attach to the instance.
	SSHKeyName string `json:"sshKeyName,omitempty"`
}

// VultrMachineStatus defines the observed state of VultrMachine
type VultrMachineStatus struct {
	Ready bool `json:"ready"`

	// ServerStatus represents the status of subscription.
	SubscriptionStatus *SubscriptionStatus `json:"subscriptionStatus,omitempty"`

	// PowerStatus represents that the VPS is powerd on or not
	PowerStatus *PowerStatus `json:"powerStatus,omitempty"`

	// ServerState represents a detail of server state.
	ServerState *ServerState `json:"serverState,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// VultrMachine is the Schema for the vultrmachines API
type VultrMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VultrMachineSpec   `json:"spec,omitempty"`
	Status VultrMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VultrMachineList contains a list of VultrMachine
type VultrMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VultrMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VultrMachine{}, &VultrMachineList{})
}
