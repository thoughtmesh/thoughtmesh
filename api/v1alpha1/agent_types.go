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

	// Tasks is the list of tasks the agent should complete
	// +optional
	Tasks []Task `json:"tasks,omitempty"`

	// EndingCondition defines when the agent should stop
	// +optional
	EndingCondition *EndingCondition `json:"endingCondition"`

	// Input defines the input data for the agent
	// +optional
	Input *AgentIO `json:"input,omitempty"`

	// Output defines where the agent should write its result
	// +optional
	Output *AgentIO `json:"output,omitempty"`

	// Memory is a reference to a Memory resource to attach to this agent
	// +optional
	Memory *MemoryRef `json:"memory,omitempty"`
}

// Task defines a unit of work for the agent
type Task struct {
	// Description of the task
	// +kubebuilder:validation:Required
	Description string `json:"description"`

	// Priority of the task, lower value means higher priority
	// +optional
	// +kubebuilder:default=0
	Priority int32 `json:"priority,omitempty"`
}

// EndingCondition defines one or more conditions that will stop the agent
type EndingCondition struct {
	// Natural is an LLM-evaluated natural language stopping condition
	// +optional
	Natural *string `json:"natural,omitempty"`

	// MaxTurns is the maximum number of turns before the agent stops
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxTurns *int32 `json:"maxTurns,omitempty"`

	// TimeoutSeconds is the maximum time in seconds before the agent stops
	// +optional
	// +kubebuilder:validation:Minimum=1
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// AgentIOType represents the type of input or output for an Agent
type AgentIOType string

const (
	AgentIOTypeString AgentIOType = "string"
	AgentIOTypeFile   AgentIOType = "file"
)

// AgentIO defines input or output for the agent
type AgentIO struct {
	// Type is either "string" or "file"
	// +kubebuilder:validation:Enum=string;file
	Type AgentIOType `json:"type"`

	// Value is used when type is "string"
	// +optional
	Value *string `json:"value,omitempty"`

	// Path is used when type is "file"
	// +optional
	Path *string `json:"path,omitempty"`
}

// MemoryRef references a Memory resource
type MemoryRef struct {
	// Ref is the name of a Memory resource in the same namespace
	// +kubebuilder:validation:Required
	Ref string `json:"ref"`
}

// AgentPhase represents the lifecycle phase of an Agent
type AgentPhase string

const (
	AgentPhasePending   AgentPhase = "Pending"
	AgentPhaseRunning   AgentPhase = "Running"
	AgentPhasePaused    AgentPhase = "Paused"
	AgentPhaseSucceeded AgentPhase = "Succeeded"
	AgentPhaseFailed    AgentPhase = "Failed"
)

// AgentStatus defines the observed state of Agent.
type AgentStatus struct {
	// Phase is the current lifecycle phase of the agent
	// +kubebuilder:validation:Enum=Pending;Running;Paused;Succeeded;Failed
	// +optional
	Phase AgentPhase `json:"phase,omitempty"`

	// CurrentTurn is the current agentic loop iteration
	// +optional
	CurrentTurn int32 `json:"currentTurn,omitempty"`

	// CurrentTask is the description of the task currently being executed
	// +optional
	CurrentTask string `json:"currentTask,omitempty"`

	// StartTime is when the agent started running
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// TerminationReason explains why the agent stopped
	// +kubebuilder:validation:Enum=NaturalConditionMet;MaxTurnsReached;TimedOut;Error;Stopped
	// +optional
	TerminationReason string `json:"terminationReason,omitempty"`
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
