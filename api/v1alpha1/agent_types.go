/*
Copyright 2026 thoughtmesh.

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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TemplateRef references an AgentTemplate
type TemplateRef struct {
	// name is the name of the AgentTemplate to run
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// AgentTemplateOverrides allows selective override of AgentTemplate fields for a specific run
type AgentTemplateOverrides struct {
	// model overrides the model configuration
	// +optional
	Model *ModelConfig `json:"model,omitempty"`

	// image overrides the container image
	// +optional
	Image *string `json:"image,omitempty"`

	// tools overrides the tools list
	// +optional
	Tools []Tool `json:"tools,omitempty"`

	// context overrides the context configuration
	// +optional
	Context *Context `json:"context,omitempty"`

	// limits overrides the limits configuration
	// +optional
	Limits *Limits `json:"limits,omitempty"`

	// lifecycle overrides the lifecycle configuration
	// +optional
	Lifecycle *Lifecycle `json:"lifecycle,omitempty"`
}

// AgentSpec defines the desired state of Agent
type AgentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// templateRef references the AgentTemplate to run
	// +kubebuilder:validation:Required
	TemplateRef TemplateRef `json:"templateRef"`

	// overrides allows selectively overriding any AgentTemplate field for this run
	// +optional
	Overrides *AgentTemplateOverrides `json:"overrides,omitempty"`
}

// JobReference references a Kubernetes Job
type JobReference struct {
	// name is the name of the Job
	// +optional
	Name string `json:"name,omitempty"`
}

// AgentStatus defines the observed state of Agent.
type AgentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// phase is the current phase of the Agent
	// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Retrying
	// +optional
	Phase string `json:"phase,omitempty"`

	// startTime is the time the Agent started running
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// completionTime is the time the Agent completed
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// llmCallsUsed is the number of LLM API calls made during the run
	// +optional
	LLMCallsUsed int32 `json:"llmCallsUsed,omitempty"`

	// jobRef is a reference to the Kubernetes Job created for this Agent
	// +optional
	JobRef *JobReference `json:"jobRef,omitempty"`

	// result is the output or summary produced by the agent
	// +optional
	Result string `json:"result,omitempty"`

	// conditions represent the current state of the Agent resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=`.spec.templateRef.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

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
