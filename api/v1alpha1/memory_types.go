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

// MemorySpec defines the desired state of Memory
type MemorySpec struct {
	// StorageSize is the size of the memory volume e.g. "10Gi"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([0-9]+)(Ki|Mi|Gi|Ti)$`
	StorageSize string `json:"storageSize"`

	// AccessMode defines how the memory volume can be mounted
	// +optional
	// +kubebuilder:default=ReadWriteOnce
	AccessMode MemoryAccessMode `json:"accessMode,omitempty"`
}

// MemoryAccessMode represents the access mode of the memory volume
type MemoryAccessMode string

const (
	MemoryAccessModeReadWriteOnce MemoryAccessMode = "ReadWriteOnce"
	MemoryAccessModeReadWriteMany MemoryAccessMode = "ReadWriteMany"
)

// MemoryStatus defines the observed state of Memory.
type MemoryStatus struct {
	// Phase is the current lifecycle phase of the memory resource
	// +kubebuilder:validation:Enum=Pending;Bound;Lost
	// +optional
	Phase MemoryPhase `json:"phase,omitempty"`

	// PVCRef is the name of the backing PersistentVolumeClaim
	// +optional
	PVCRef string `json:"pvcRef,omitempty"`

	// Conditions represent the current state of the Memory resource
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// MemoryPhase represents the lifecycle phase of a Memory resource
type MemoryPhase string

const (
	MemoryPhasePending MemoryPhase = "Pending"
	MemoryPhaseBound   MemoryPhase = "Bound"
	MemoryPhaseLost    MemoryPhase = "Lost"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Memory is the Schema for the memories API
type Memory struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Memory
	// +required
	Spec MemorySpec `json:"spec"`

	// status defines the observed state of Memory
	// +optional
	Status MemoryStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// MemoryList contains a list of Memory
type MemoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Memory `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Memory{}, &MemoryList{})
}
