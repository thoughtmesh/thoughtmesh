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

// WorkerModelRoleConfig defines the configuration for a worker model role
type WorkerModelRoleConfig struct {
	// provider is the LLM provider name (e.g. anthropic, openai, ollama, vertex, azure-openai, mistral)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=anthropic;openai;ollama;vertex;azure-openai;mistral
	Provider string `json:"provider"`

	// apiName is the model name identifier (e.g. claude-sonnet-4-20250514, gpt-4o, llama3.3)
	// +kubebuilder:validation:Required
	ModelName string `json:"apiName"`

	// endpoint is the custom endpoint URL. Omit to use the provider default.
	// Required for Ollama and Azure OpenAI.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// temperature is the sampling temperature (0.0–1.0). Lower values are more deterministic.
	// Stored as string to comply with Kubernetes API conventions.
	// +kubebuilder:validation:Pattern=`^0(\.[0-9]+)?$|^1(\.0+)?$`
	// +kubebuilder:default="0.2"
	// +optional
	Temperature string `json:"temperature,omitempty"`

	// systemPrompt overrides the base system prompt injected by the ThoughtMesh runtime.
	// Omit to use the default.
	// +optional
	SystemPrompt string `json:"systemPrompt,omitempty"`

	// params is a free-form map for provider-specific parameters (e.g. top_p, top_k).
	// Values are passed through to the provider client as-is.
	// +optional
	Params map[string]string `json:"params,omitempty"`
}

// ModelConfig defines the role-based model configuration
type ModelConfig struct {
	// worker defines the model configuration for the worker role
	// +kubebuilder:validation:Required
	Worker WorkerModelRoleConfig `json:"worker"`
}

// Tool defines an MCP server available to the agent
type Tool struct {
	// name is the tool identifier
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// type is the tool type (currently only "mcp" is supported)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=mcp
	Type string `json:"type"`

	// url is the MCP server URL
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`
}

// Context defines Kubernetes-native config and credential sources
type Context struct {
	// configMapRefs is an ordered list of ConfigMaps mounted as env vars.
	// Later entries overwrite conflicting keys from earlier ones.
	// +optional
	ConfigMapRefs []string `json:"configMapRefs,omitempty"`

	// secretRefs is an ordered list of Secrets mounted as env vars.
	// Later entries overwrite conflicting keys from earlier ones.
	// +optional
	SecretRefs []string `json:"secretRefs,omitempty"`
}

// Limits defines resource and time constraints for the agent
type Limits struct {
	// timeout is the hard time limit in Go duration format (e.g. 5m, 30s, 1h).
	// Agent is killed if exceeded.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$`
	Timeout string `json:"timeout"`
}

// RetryPolicy defines retry behavior on failure
type RetryPolicy struct {
	// maxRetries is the maximum number of retry attempts
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxRetries int32 `json:"maxRetries,omitempty"`

	// backoffSeconds is the delay in seconds before retrying
	// +kubebuilder:validation:Minimum=0
	// +optional
	BackoffSeconds int32 `json:"backoffSeconds,omitempty"`
}

// CompletionPolicy defines completion conditions and Pod disposition
type CompletionPolicy struct {
	// condition defines when the agent is considered done
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=objective-achieved;max-steps;timeout
	Condition string `json:"condition"`

	// onSuccess defines Pod disposition after success
	// +kubebuilder:validation:Enum=delete;retain;archive
	// +kubebuilder:default=retain
	// +optional
	OnSuccess string `json:"onSuccess,omitempty"`

	// onFailure defines Pod disposition after failure
	// +kubebuilder:validation:Enum=delete;retain;archive
	// +kubebuilder:default=retain
	// +optional
	OnFailure string `json:"onFailure,omitempty"`
}

// Lifecycle defines retry and completion policies
type Lifecycle struct {
	// retryPolicy defines retry behavior on failure
	// +optional
	RetryPolicy *RetryPolicy `json:"retryPolicy,omitempty"`

	// completion defines completion conditions and Pod disposition
	// +kubebuilder:validation:Required
	Completion CompletionPolicy `json:"completion"`
}

// AgentTemplateSpec defines the desired state of AgentTemplate
type AgentTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// objective is the plain-language goal for the agent. One task, one goal.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Objective string `json:"objective"`

	// model is the role-based model configuration
	// +kubebuilder:validation:Required
	Model ModelConfig `json:"model"`

	// image is the container image for the agent Pod.
	// Omit to use the ThoughtMesh default runtime image.
	// +optional
	Image string `json:"image,omitempty"`

	// tools is the list of MCP servers available to the agent
	// +optional
	Tools []Tool `json:"tools,omitempty"`

	// context declares Kubernetes-native config and credential sources
	// +optional
	Context *Context `json:"context,omitempty"`

	// limits defines resource and time constraints for the agent
	// +kubebuilder:validation:Required
	Limits Limits `json:"limits"`

	// lifecycle defines retry and completion policies
	// +kubebuilder:validation:Required
	Lifecycle Lifecycle `json:"lifecycle"`
}

// AgentTemplateStatus defines the observed state of AgentTemplate.
type AgentTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the AgentTemplate resource.
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
// +kubebuilder:printcolumn:name="Objective",type=string,JSONPath=`.spec.objective`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentTemplate is the Schema for the agenttemplates API
type AgentTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AgentTemplate
	// +required
	Spec AgentTemplateSpec `json:"spec"`

	// status defines the observed state of AgentTemplate
	// +optional
	Status AgentTemplateStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AgentTemplateList contains a list of AgentTemplate
type AgentTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AgentTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentTemplate{}, &AgentTemplateList{})
}
