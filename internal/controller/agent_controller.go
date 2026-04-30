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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/thoughtmesh/thoughtmesh/api/v1alpha1"
)

const (
	agentFinalizerName   = "thoughtmesh.dev/agent-protection"
	defaultRuntimeImage  = "ghcr.io/thoughtmesh/thoughtmesh-runtime:latest"
	retryCountAnnotation = "thoughtmesh.dev/retry-count"
)

// AgentReconciler reconciles a Agent object
type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agents/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Agent object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the Agent instance
	agent := &corev1alpha1.Agent{}
	if err := r.Get(ctx, req.NamespacedName, agent); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Agent resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Agent")
		return ctrl.Result{}, err
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(agent, agentFinalizerName) {
		log.Info("Adding finalizer to Agent")
		controllerutil.AddFinalizer(agent, agentFinalizerName)
		if err := r.Update(ctx, agent); err != nil {
			log.Error(err, "Failed to add finalizer to Agent")
			return ctrl.Result{}, err
		}
		// Re-fetch after update
		if err := r.Get(ctx, req.NamespacedName, agent); err != nil {
			log.Error(err, "Failed to re-fetch Agent after adding finalizer")
			return ctrl.Result{}, err
		}
	}

	// Handle deletion
	if !agent.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, agent)
	}

	// Check if Job already exists
	job := &batchv1.Job{}
	jobName := fmt.Sprintf("%s-job", agent.Name)
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: agent.Namespace}, job)

	if err != nil && apierrors.IsNotFound(err) {
		// Create new Job
		job, err = r.createJob(ctx, agent)
		if err != nil {
			log.Error(err, "Failed to create Job")
			return ctrl.Result{}, err
		}
		log.Info("Created Job", "job", job.Name)
	} else if err != nil {
		log.Error(err, "Failed to get Job")
		return ctrl.Result{}, err
	}

	// Sync Job status to Agent status
	return r.syncJobStatus(ctx, agent, job)
}

// handleDeletion handles the deletion of an Agent resource
func (r *AgentReconciler) handleDeletion(ctx context.Context, agent *corev1alpha1.Agent) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(agent, agentFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Check if owned Job exists
	jobName := fmt.Sprintf("%s-job", agent.Name)
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: agent.Namespace}, job)

	if err == nil {
		// Job exists, delete it
		log.Info("Deleting owned Job")
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
			log.Error(err, "Failed to delete Job")
			return ctrl.Result{}, err
		}
		// Requeue to wait for Job deletion
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	} else if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get Job")
		return ctrl.Result{}, err
	}

	// Job is gone, remove finalizer
	log.Info("Removing finalizer from Agent")
	controllerutil.RemoveFinalizer(agent, agentFinalizerName)
	if err := r.Update(ctx, agent); err != nil {
		log.Error(err, "Failed to remove finalizer from Agent")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// createJob creates a Kubernetes Job for the Agent
func (r *AgentReconciler) createJob(ctx context.Context, agent *corev1alpha1.Agent) (*batchv1.Job, error) {
	log := logf.FromContext(ctx)

	spec := &agent.Spec

	// Parse timeout duration if specified
	var activeDeadlineSeconds *int64
	if spec.Limits != nil && spec.Limits.Timeout != "" {
		timeout, err := time.ParseDuration(spec.Limits.Timeout)
		if err != nil {
			log.Error(err, "Failed to parse timeout duration")
			return nil, err
		}
		seconds := int64(timeout.Seconds())
		activeDeadlineSeconds = &seconds
	}

	// Build environment variables
	envVars := []corev1.EnvVar{}

	// Add model configuration as env vars
	modelConfigJSON, err := json.Marshal(spec.Model)
	if err != nil {
		log.Error(err, "Failed to marshal model config")
		return nil, err
	}
	envVars = append(envVars, corev1.EnvVar{
		Name:  "THOUGHTMESH_MODEL_CONFIG",
		Value: string(modelConfigJSON),
	})

	// Add objective
	envVars = append(envVars, corev1.EnvVar{
		Name:  "THOUGHTMESH_OBJECTIVE",
		Value: spec.Objective,
	})

	// Add key results if present
	if len(spec.KeyResults) > 0 {
		keyResultsJSON, err := json.Marshal(spec.KeyResults)
		if err != nil {
			log.Error(err, "Failed to marshal key results")
			return nil, err
		}
		envVars = append(envVars, corev1.EnvVar{
			Name:  "THOUGHTMESH_KEY_RESULTS",
			Value: string(keyResultsJSON),
		})
	}

	// Add tools configuration
	if len(spec.Tools) > 0 {
		toolsJSON, err := json.Marshal(spec.Tools)
		if err != nil {
			log.Error(err, "Failed to marshal tools config")
			return nil, err
		}
		envVars = append(envVars, corev1.EnvVar{
			Name:  "THOUGHTMESH_TOOLS",
			Value: string(toolsJSON),
		})
	}

	// Mount ConfigMaps and Secrets as env vars
	envFrom := []corev1.EnvFromSource{}
	if spec.Context != nil {
		// Add ConfigMaps in order
		for _, cmName := range spec.Context.ConfigMapRefs {
			envFrom = append(envFrom, corev1.EnvFromSource{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cmName,
					},
				},
			})
		}

		// Add Secrets in order
		for _, secretName := range spec.Context.SecretRefs {
			envFrom = append(envFrom, corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
				},
			})
		}
	}

	var zero int32 = 0

	// Create Job spec
	jobName := fmt.Sprintf("%s-job", agent.Name)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: agent.Namespace,
			Labels: map[string]string{
				"thoughtmesh.dev/agent": agent.Name,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          &zero,
			ActiveDeadlineSeconds: activeDeadlineSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"thoughtmesh.dev/agent": agent.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "agent-runtime",
							Image:   r.getImage(spec),
							Env:     envVars,
							EnvFrom: envFrom,
							Command: []string{"/thoughtmesh/runtime"},
						},
					},
				},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(agent, job, r.Scheme); err != nil {
		log.Error(err, "Failed to set controller reference")
		return nil, err
	}

	// Create the Job
	if err := r.Create(ctx, job); err != nil {
		log.Error(err, "Failed to create Job")
		return nil, err
	}

	// Update Agent status with Job reference
	agent.Status.JobRef = &corev1alpha1.JobReference{Name: jobName}
	agent.Status.Phase = "Pending"
	if err := r.Status().Update(ctx, agent); err != nil {
		log.Error(err, "Failed to update Agent status")
		return nil, err
	}

	return job, nil
}

// getImage returns the image to use for the agent, using default if not specified
func (r *AgentReconciler) getImage(spec *corev1alpha1.AgentSpec) string {
	if spec.Image != "" {
		return spec.Image
	}
	return defaultRuntimeImage
}

// syncJobStatus syncs the Job status to Agent status
func (r *AgentReconciler) syncJobStatus(ctx context.Context, agent *corev1alpha1.Agent, job *batchv1.Job) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Update Job reference if not set
	if agent.Status.JobRef == nil || agent.Status.JobRef.Name != job.Name {
		agent.Status.JobRef = &corev1alpha1.JobReference{Name: job.Name}
	}

	// Sync start time
	if job.Status.StartTime != nil && agent.Status.StartTime == nil {
		agent.Status.StartTime = job.Status.StartTime
	}

	// Determine phase from Job status
	previousPhase := agent.Status.Phase

	if job.Status.Active > 0 {
		agent.Status.Phase = "Running"
	} else if job.Status.Succeeded > 0 {
		agent.Status.Phase = "Succeeded"
		if agent.Status.CompletionTime == nil {
			now := metav1.Now()
			agent.Status.CompletionTime = &now
		}
	} else if job.Status.Failed > 0 {
		agent.Status.Phase = "Failed"
		if agent.Status.CompletionTime == nil {
			now := metav1.Now()
			agent.Status.CompletionTime = &now
		}
	} else {
		agent.Status.Phase = "Pending"
	}

	// Update status
	if err := r.Status().Update(ctx, agent); err != nil {
		log.Error(err, "Failed to update Agent status")
		return ctrl.Result{}, err
	}

	// Handle completion
	if agent.Status.Phase == "Succeeded" && previousPhase != "Succeeded" {
		return r.handleSuccess(ctx, agent, job)
	} else if agent.Status.Phase == "Failed" && previousPhase != "Failed" {
		return r.handleFailure(ctx, agent, job)
	}

	// If still running or pending, continue monitoring
	if agent.Status.Phase == "Running" || agent.Status.Phase == "Pending" {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// handleSuccess handles successful Job completion
func (r *AgentReconciler) handleSuccess(ctx context.Context, agent *corev1alpha1.Agent, job *batchv1.Job) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Set ObjectiveAchieved condition
	r.setCondition(agent, metav1.Condition{
		Type:               "ObjectiveAchieved",
		Status:             metav1.ConditionTrue,
		Reason:             "Succeeded",
		Message:            "Agent successfully completed its objective",
		ObservedGeneration: agent.Generation,
	})

	r.setCondition(agent, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Completed",
		Message:            "Agent execution completed successfully",
		ObservedGeneration: agent.Generation,
	})

	if err := r.Status().Update(ctx, agent); err != nil {
		log.Error(err, "Failed to update Agent status")
		return ctrl.Result{}, err
	}

	// Apply onSuccess disposition policy
	disposition := "retain"
	if agent.Spec.Lifecycle != nil && agent.Spec.Lifecycle.Completion.OnSuccess != "" {
		disposition = agent.Spec.Lifecycle.Completion.OnSuccess
	}

	switch disposition {
	case "delete":
		log.Info("Deleting Pod after success")
		// Delete the pods owned by the Job
		if err := r.deleteJobPods(ctx, job); err != nil {
			log.Error(err, "Failed to delete Job pods")
			return ctrl.Result{}, err
		}
	case "archive":
		log.Info("Archiving Pod after success")
		// TODO: Implement archiving logic when archive store is configured
		// For now, just delete the pods
		if err := r.deleteJobPods(ctx, job); err != nil {
			log.Error(err, "Failed to delete Job pods after archive")
			return ctrl.Result{}, err
		}
	case "retain":
		log.Info("Retaining Pod after success")
		// Do nothing
	}

	return ctrl.Result{}, nil
}

// handleFailure handles failed Job completion
func (r *AgentReconciler) handleFailure(ctx context.Context, agent *corev1alpha1.Agent, job *batchv1.Job) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Check retry policy
	var retryCount int32
	if agent.Annotations != nil {
		if countStr, ok := agent.Annotations[retryCountAnnotation]; ok {
			fmt.Sscanf(countStr, "%d", &retryCount)
		}
	}

	maxRetries := int32(0)
	backoffSeconds := int32(30)
	if agent.Spec.Lifecycle != nil && agent.Spec.Lifecycle.RetryPolicy != nil {
		maxRetries = agent.Spec.Lifecycle.RetryPolicy.MaxRetries
		if agent.Spec.Lifecycle.RetryPolicy.BackoffSeconds > 0 {
			backoffSeconds = agent.Spec.Lifecycle.RetryPolicy.BackoffSeconds
		}
	}

	if retryCount < maxRetries {
		// Retry
		log.Info("Retrying Agent", "retryCount", retryCount+1, "maxRetries", maxRetries)

		// Update retry count
		if agent.Annotations == nil {
			agent.Annotations = make(map[string]string)
		}
		agent.Annotations[retryCountAnnotation] = fmt.Sprintf("%d", retryCount+1)

		// Set phase to Retrying
		agent.Status.Phase = "Retrying"

		if err := r.Update(ctx, agent); err != nil {
			log.Error(err, "Failed to update Agent annotations")
			return ctrl.Result{}, err
		}

		if err := r.Status().Update(ctx, agent); err != nil {
			log.Error(err, "Failed to update Agent status")
			return ctrl.Result{}, err
		}

		// Delete the failed Job so a new one can be created
		if err := r.Delete(ctx, job); err != nil {
			log.Error(err, "Failed to delete failed Job")
			return ctrl.Result{}, err
		}

		// Requeue after backoff
		return ctrl.Result{RequeueAfter: time.Duration(backoffSeconds) * time.Second}, nil
	}

	// Retries exhausted
	log.Info("Retries exhausted")

	r.setCondition(agent, metav1.Condition{
		Type:               "ObjectiveAchieved",
		Status:             metav1.ConditionFalse,
		Reason:             "Failed",
		Message:            fmt.Sprintf("Agent failed after %d retries", retryCount),
		ObservedGeneration: agent.Generation,
	})

	r.setCondition(agent, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "Failed",
		Message:            "Agent execution failed",
		ObservedGeneration: agent.Generation,
	})

	if err := r.Status().Update(ctx, agent); err != nil {
		log.Error(err, "Failed to update Agent status")
		return ctrl.Result{}, err
	}

	// Apply onFailure disposition policy
	disposition := "retain"
	if agent.Spec.Lifecycle != nil && agent.Spec.Lifecycle.Completion.OnFailure != "" {
		disposition = agent.Spec.Lifecycle.Completion.OnFailure
	}

	switch disposition {
	case "delete":
		log.Info("Deleting Pod after failure")
		if err := r.deleteJobPods(ctx, job); err != nil {
			log.Error(err, "Failed to delete Job pods")
			return ctrl.Result{}, err
		}
	case "archive":
		log.Info("Archiving Pod after failure")
		// TODO: Implement archiving logic when archive store is configured
		if err := r.deleteJobPods(ctx, job); err != nil {
			log.Error(err, "Failed to delete Job pods after archive")
			return ctrl.Result{}, err
		}
	case "retain":
		log.Info("Retaining Pod after failure")
		// Do nothing
	}

	return ctrl.Result{}, nil
}

// deleteJobPods deletes all pods owned by a Job
func (r *AgentReconciler) deleteJobPods(ctx context.Context, job *batchv1.Job) error {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(job.Namespace), client.MatchingLabels{
		"job-name": job.Name,
	}); err != nil {
		return err
	}

	for i := range podList.Items {
		if err := r.Delete(ctx, &podList.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// setCondition sets or updates a condition in the Agent status
func (r *AgentReconciler) setCondition(agent *corev1alpha1.Agent, condition metav1.Condition) {
	condition.LastTransitionTime = metav1.Now()
	meta.SetStatusCondition(&agent.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.Agent{}).
		Owns(&batchv1.Job{}).
		Named("agent").
		Complete(r)
}
