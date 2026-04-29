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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/thoughtmesh/thoughtmesh/api/v1alpha1"
)

const (
	agentFinalizerName         = "thoughtmesh.dev/agent-protection"
	defaultRuntimeImage        = "ghcr.io/ufukbombar/thoughtmesh-runtime:latest"
	retryCountAnnotation       = "thoughtmesh.dev/retry-count"
)

// ResolvedAgentSpec contains the final merged spec to be used for Job creation
type ResolvedAgentSpec struct {
	Objective string
	Model     corev1alpha1.ModelConfig
	Image     string
	Tools     []corev1alpha1.Tool
	Context   *corev1alpha1.Context
	Limits    corev1alpha1.Limits
	Lifecycle corev1alpha1.Lifecycle
}

// AgentReconciler reconciles a Agent object
type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agents/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agenttemplates,verbs=get;list;watch
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

	// Validate templateRef and get AgentTemplate
	agentTemplate, result, err := r.validateAndGetTemplate(ctx, agent)
	if err != nil || result != nil {
		return *result, err
	}

	// Resolve final spec
	resolvedSpec := r.resolveSpec(agentTemplate, agent)

	// Check if Job already exists
	job := &batchv1.Job{}
	jobName := fmt.Sprintf("%s-job", agent.Name)
	err = r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: agent.Namespace}, job)
	
	if err != nil && apierrors.IsNotFound(err) {
		// Create new Job
		job, err = r.createJob(ctx, agent, resolvedSpec)
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
	return r.syncJobStatus(ctx, agent, job, resolvedSpec)
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

// validateAndGetTemplate validates the templateRef and returns the AgentTemplate
func (r *AgentReconciler) validateAndGetTemplate(ctx context.Context, agent *corev1alpha1.Agent) (*corev1alpha1.AgentTemplate, *ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AgentTemplate
	agentTemplate := &corev1alpha1.AgentTemplate{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      agent.Spec.TemplateRef.Name,
		Namespace: agent.Namespace,
	}, agentTemplate)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("AgentTemplate not found")
			r.setCondition(agent, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "TemplateNotFound",
				Message:            fmt.Sprintf("AgentTemplate %s not found", agent.Spec.TemplateRef.Name),
				ObservedGeneration: agent.Generation,
			})
			if updateErr := r.Status().Update(ctx, agent); updateErr != nil {
				log.Error(updateErr, "Failed to update status")
				return nil, nil, updateErr
			}
			// Requeue with backoff
			result := ctrl.Result{RequeueAfter: 30 * time.Second}
			return nil, &result, nil
		}
		log.Error(err, "Failed to get AgentTemplate")
		return nil, nil, err
	}

	// Check if AgentTemplate is Ready
	readyCondition := meta.FindStatusCondition(agentTemplate.Status.Conditions, "Ready")
	if readyCondition == nil || readyCondition.Status != metav1.ConditionTrue {
		log.Info("AgentTemplate is not ready")
		r.setCondition(agent, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "TemplateNotReady",
			Message:            fmt.Sprintf("AgentTemplate %s is not ready", agent.Spec.TemplateRef.Name),
			ObservedGeneration: agent.Generation,
		})
		if updateErr := r.Status().Update(ctx, agent); updateErr != nil {
			log.Error(updateErr, "Failed to update status")
			return nil, nil, updateErr
		}
		// Requeue with backoff
		result := ctrl.Result{RequeueAfter: 30 * time.Second}
		return nil, &result, nil
	}

	return agentTemplate, nil, nil
}

// resolveSpec merges AgentTemplate spec with Agent-level overrides
func (r *AgentReconciler) resolveSpec(template *corev1alpha1.AgentTemplate, agent *corev1alpha1.Agent) ResolvedAgentSpec {
	resolved := ResolvedAgentSpec{
		Objective: template.Spec.Objective,
		Model:     template.Spec.Model,
		Image:     template.Spec.Image,
		Tools:     template.Spec.Tools,
		Context:   template.Spec.Context,
		Limits:    template.Spec.Limits,
		Lifecycle: template.Spec.Lifecycle,
	}

	// Apply overrides if present
	if agent.Spec.Overrides != nil {
		overrides := agent.Spec.Overrides

		if overrides.Model != nil {
			resolved.Model = *overrides.Model
		}

		if overrides.Image != nil {
			resolved.Image = *overrides.Image
		}

		if overrides.Tools != nil {
			resolved.Tools = overrides.Tools
		}

		if overrides.Context != nil {
			resolved.Context = overrides.Context
		}

		if overrides.Limits != nil {
			resolved.Limits = *overrides.Limits
		}

		if overrides.Lifecycle != nil {
			resolved.Lifecycle = *overrides.Lifecycle
		}
	}

	// Use default image if not specified
	if resolved.Image == "" {
		resolved.Image = defaultRuntimeImage
	}

	return resolved
}

// createJob creates a Kubernetes Job for the Agent
func (r *AgentReconciler) createJob(ctx context.Context, agent *corev1alpha1.Agent, spec ResolvedAgentSpec) (*batchv1.Job, error) {
	log := logf.FromContext(ctx)

	// Parse timeout duration
	timeout, err := time.ParseDuration(spec.Limits.Timeout)
	if err != nil {
		log.Error(err, "Failed to parse timeout duration")
		return nil, err
	}
	activeDeadlineSeconds := int64(timeout.Seconds())

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
			ActiveDeadlineSeconds: &activeDeadlineSeconds,
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
							Image:   spec.Image,
							Env:     envVars,
							EnvFrom: envFrom,
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

// syncJobStatus syncs the Job status to Agent status
func (r *AgentReconciler) syncJobStatus(ctx context.Context, agent *corev1alpha1.Agent, job *batchv1.Job, spec ResolvedAgentSpec) (ctrl.Result, error) {
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
		return r.handleSuccess(ctx, agent, job, spec)
	} else if agent.Status.Phase == "Failed" && previousPhase != "Failed" {
		return r.handleFailure(ctx, agent, job, spec)
	}

	// If still running or pending, continue monitoring
	if agent.Status.Phase == "Running" || agent.Status.Phase == "Pending" {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// handleSuccess handles successful Job completion
func (r *AgentReconciler) handleSuccess(ctx context.Context, agent *corev1alpha1.Agent, job *batchv1.Job, spec ResolvedAgentSpec) (ctrl.Result, error) {
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
	disposition := spec.Lifecycle.Completion.OnSuccess
	if disposition == "" {
		disposition = "retain"
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
func (r *AgentReconciler) handleFailure(ctx context.Context, agent *corev1alpha1.Agent, job *batchv1.Job, spec ResolvedAgentSpec) (ctrl.Result, error) {
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
	if spec.Lifecycle.RetryPolicy != nil {
		maxRetries = spec.Lifecycle.RetryPolicy.MaxRetries
		if spec.Lifecycle.RetryPolicy.BackoffSeconds > 0 {
			backoffSeconds = spec.Lifecycle.RetryPolicy.BackoffSeconds
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
	disposition := spec.Lifecycle.Completion.OnFailure
	if disposition == "" {
		disposition = "retain"
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
		Watches(
			&corev1alpha1.AgentTemplate{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForTemplate),
		).
		Named("agent").
		Complete(r)
}

// findAgentsForTemplate finds all Agents that reference an AgentTemplate
func (r *AgentReconciler) findAgentsForTemplate(ctx context.Context, template client.Object) []reconcile.Request {
	agentList := &corev1alpha1.AgentList{}
	if err := r.List(ctx, agentList, client.InNamespace(template.GetNamespace())); err != nil {
		return []reconcile.Request{}
	}

	var requests []reconcile.Request
	for _, agent := range agentList.Items {
		if agent.Spec.TemplateRef.Name == template.GetName() {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      agent.Name,
					Namespace: agent.Namespace,
				},
			})
		}
	}
	return requests
}
