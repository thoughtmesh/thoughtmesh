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

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/thoughtmesh/thoughtmesh/api/v1alpha1"
)

const TMAgentImge = "ghcr.io/thoughtmesh/thoughtmesh/tm-agent:latest"

// AgentReconciler reconciles an Agent object
type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agents/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Agent
	agent := &corev1alpha1.Agent{}
	if err := r.Get(ctx, req.NamespacedName, agent); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Reconcile StatefulSet
	if err := r.reconcileStatefulSet(ctx, agent); err != nil {
		log.Error(err, "failed to reconcile StatefulSet")
		return ctrl.Result{}, err
	}

	// Reconcile Service
	if err := r.reconcileService(ctx, agent); err != nil {
		log.Error(err, "failed to reconcile Service")
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.reconcileStatus(ctx, agent); err != nil {
		log.Error(err, "failed to reconcile status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AgentReconciler) reconcileStatefulSet(ctx context.Context, agent *corev1alpha1.Agent) error {
	desired := r.buildStatefulSet(agent)

	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, existing)
	if errors.IsNotFound(err) {
		if err := ctrl.SetControllerReference(agent, desired, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Update the image if it changed
	existing.Spec.Template.Spec.Containers[0].Image = desired.Spec.Template.Spec.Containers[0].Image
	existing.Spec.Template.Spec.Containers[0].Env = desired.Spec.Template.Spec.Containers[0].Env
	return r.Update(ctx, existing)
}

func (r *AgentReconciler) reconcileService(ctx context.Context, agent *corev1alpha1.Agent) error {
	desired := r.buildService(agent)

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, existing)
	if errors.IsNotFound(err) {
		if err := ctrl.SetControllerReference(agent, desired, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, desired)
	}
	return err
}

func (r *AgentReconciler) reconcileStatus(ctx context.Context, agent *corev1alpha1.Agent) error {
	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, sts); err != nil {
		return err
	}

	patch := client.MergeFrom(agent.DeepCopy())

	if sts.Status.ReadyReplicas > 0 {
		agent.Status.Phase = corev1alpha1.AgentPhaseRunning
	} else {
		agent.Status.Phase = corev1alpha1.AgentPhasePending
	}

	return r.Status().Patch(ctx, agent, patch)
}

func (r *AgentReconciler) buildStatefulSet(agent *corev1alpha1.Agent) *appsv1.StatefulSet {
	labels := labelsForAgent(agent.Name)

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.Name,
			Namespace: agent.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    new(int32(1)),
			ServiceName: agent.Name,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "agent",
							Image: TMAgentImge,
							Env:   r.buildEnvVars(agent),
						},
					},
				},
			},
		},
	}
}

func (r *AgentReconciler) buildService(agent *corev1alpha1.Agent) *corev1.Service {
	labels := labelsForAgent(agent.Name)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.Name,
			Namespace: agent.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector:  labels,
			ClusterIP: "None", // headless, for StatefulSet DNS
			Ports: []corev1.ServicePort{
				{
					Name:     "queue",
					Port:     8080,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}
}

func (r *AgentReconciler) buildEnvVars(agent *corev1alpha1.Agent) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "AGENT_NAME",
			Value: agent.Name,
		},
		{
			Name:  "AGENT_NAMESPACE",
			Value: agent.Namespace,
		},
		{
			Name:  "AGENT_OBJECTIVE",
			Value: agent.Spec.Objective,
		},
	}

	// Ending conditions
	if agent.Spec.EndingCondition.Natural != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "ENDING_CONDITION_NATURAL",
			Value: *agent.Spec.EndingCondition.Natural,
		})
	}
	if agent.Spec.EndingCondition.MaxTurns != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "ENDING_CONDITION_MAX_TURNS",
			Value: fmt.Sprintf("%d", *agent.Spec.EndingCondition.MaxTurns),
		})
	}
	if agent.Spec.EndingCondition.TimeoutSeconds != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "ENDING_CONDITION_TIMEOUT_SECONDS",
			Value: fmt.Sprintf("%d", *agent.Spec.EndingCondition.TimeoutSeconds),
		})
	}

	// Input
	if agent.Spec.Input != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "INPUT_TYPE",
			Value: string(agent.Spec.Input.Type),
		})
		if agent.Spec.Input.Value != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "INPUT_VALUE",
				Value: *agent.Spec.Input.Value,
			})
		}
		if agent.Spec.Input.Path != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "INPUT_PATH",
				Value: *agent.Spec.Input.Path,
			})
		}
	}

	return envVars
}

func labelsForAgent(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "agent",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/managed-by": "thoughtmesh",
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.Agent{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
