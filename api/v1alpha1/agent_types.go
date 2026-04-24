/*
Copyright 2026.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentSpec defines the desired state of Agent
type AgentSpec struct {
	// Objective is the high-level goal of the agent
	// +kubebuilder:validation:Required
	Objective string `json:"objective"`

	// System is the custom system prompt for the agent
	// +optional
	System *string `json:"system"`

	// Termination defines when the agent should stop
	// +kubebuilder:validation:Required
	Termination string `json:"termination"`
}

// AgentPhase represents the lifecycle phase of an Agent
type AgentPhase string

const (
	AgentPhasePending   AgentPhase = "Pending"   // just created
	AgentPhaseWorking   AgentPhase = "Working"   // generating tokens
	AgentPhaseWaiting   AgentPhase = "Waiting"   // waiting input from user
	AgentPhaseSucceeded AgentPhase = "Succeeded" // succeeded
	AgentPhaseFailed    AgentPhase = "Failed"    // failed
)

// AgentStatus defines the observed state of Agent.
type AgentStatus struct {
	// Phase is the current lifecycle phase of the agent
	// +kubebuilder:validation:Enum=Pending;Working;Waiting;Succeeded;Failed
	// +optional
	Phase AgentPhase `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Agent is the Schema for the agents API
type Agent struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Agent
	// +required
	Spec AgentSpec `json:"spec"`

	// status defines the observed state of Agent
	// +optional
	Status AgentStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AgentList contains a list of Agent
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Agent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
