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

const TMAgentImage = "ghcr.io/thoughtmesh/thoughtmesh/thoughtmesh-agent:main"

// AgentReconciler reconciles an Agent object
type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agents/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
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

	// Reconcile Deployment
	if err := r.reconcileDeployment(ctx, agent); err != nil {
		log.Error(err, "failed to reconcile Deployment")
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

func (r *AgentReconciler) reconcileDeployment(ctx context.Context, agent *corev1alpha1.Agent) error {
	desired := r.buildDeployment(agent)

	existing := &appsv1.Deployment{}
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

	// Update the image and env if changed
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
	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, deploy); err != nil {
		return err
	}

	patch := client.MergeFrom(agent.DeepCopy())

	if deploy.Status.ReadyReplicas > 0 {
		agent.Status.Phase = corev1alpha1.AgentPhaseWorking
	} else {
		agent.Status.Phase = corev1alpha1.AgentPhasePending
	}

	return r.Status().Patch(ctx, agent, patch)
}

func (r *AgentReconciler) buildDeployment(agent *corev1alpha1.Agent) *appsv1.Deployment {
	labels := labelsForAgent(agent.Name)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.Name,
			Namespace: agent.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32(1)),
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
							Image: TMAgentImage,
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
			ClusterIP: "None", // headless service
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
	var system string
	if agent.Spec.System != nil {
		system = *agent.Spec.System
	}

	return []corev1.EnvVar{
		{Name: "AGENT_NAME", Value: agent.Name},
		{Name: "AGENT_OBJECTIVE", Value: agent.Spec.Objective},
		{Name: "AGENT_SYSTEM", Value: system},
		{Name: "AGENT_TERMINATION", Value: agent.Spec.Termination},
	}
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
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
