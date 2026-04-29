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
	"fmt"
	"time"

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
	agentTemplateFinalizerName = "thoughtmesh.dev/agent-template-protection"
)

// AgentTemplateReconciler reconciles a AgentTemplate object
type AgentTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agenttemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agenttemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agenttemplates/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.thoughtmesh.dev,resources=agents,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AgentTemplate object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *AgentTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AgentTemplate instance
	agentTemplate := &corev1alpha1.AgentTemplate{}
	if err := r.Get(ctx, req.NamespacedName, agentTemplate); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("AgentTemplate resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get AgentTemplate")
		return ctrl.Result{}, err
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(agentTemplate, agentTemplateFinalizerName) {
		log.Info("Adding finalizer to AgentTemplate")
		controllerutil.AddFinalizer(agentTemplate, agentTemplateFinalizerName)
		if err := r.Update(ctx, agentTemplate); err != nil {
			log.Error(err, "Failed to add finalizer to AgentTemplate")
			return ctrl.Result{}, err
		}
	}

	// Handle deletion
	if !agentTemplate.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, agentTemplate)
	}

	// Validate the spec
	if err := r.validateSpec(ctx, agentTemplate); err != nil {
		log.Info("Validation failed", "error", err.Error())
		r.setCondition(agentTemplate, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ValidationFailed",
			Message:            err.Error(),
			ObservedGeneration: agentTemplate.Generation,
		})
		if updateErr := r.Status().Update(ctx, agentTemplate); updateErr != nil {
			log.Error(updateErr, "Failed to update status")
			return ctrl.Result{}, updateErr
		}
		// Do not requeue on validation failure - wait for user to fix the spec
		return ctrl.Result{}, nil
	}

	// Set Ready condition on success
	r.setCondition(agentTemplate, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "ValidationSucceeded",
		Message:            "AgentTemplate is valid and ready to be referenced",
		ObservedGeneration: agentTemplate.Generation,
	})
	if err := r.Status().Update(ctx, agentTemplate); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// handleDeletion handles the deletion of an AgentTemplate resource
func (r *AgentTemplateReconciler) handleDeletion(ctx context.Context, agentTemplate *corev1alpha1.AgentTemplate) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(agentTemplate, agentTemplateFinalizerName) {
		return ctrl.Result{}, nil
	}

	// List all Agent objects in the same namespace
	agentList := &corev1alpha1.AgentList{}
	if err := r.List(ctx, agentList, client.InNamespace(agentTemplate.Namespace)); err != nil {
		log.Error(err, "Failed to list Agent resources")
		return ctrl.Result{}, err
	}

	// Check if any Agent references this AgentTemplate
	referencingAgents := 0
	for _, agent := range agentList.Items {
		if agent.Spec.TemplateRef.Name == agentTemplate.Name {
			referencingAgents++
		}
	}

	if referencingAgents > 0 {
		log.Info("Deletion blocked", "referencingAgents", referencingAgents)
		r.setCondition(agentTemplate, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "DeletionBlocked",
			Message:            fmt.Sprintf("AgentTemplate is referenced by %d active Agent(s)", referencingAgents),
			ObservedGeneration: agentTemplate.Generation,
		})
		if err := r.Status().Update(ctx, agentTemplate); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
		// Requeue to check again later
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// No references exist, remove the finalizer and allow deletion
	log.Info("Removing finalizer from AgentTemplate")
	controllerutil.RemoveFinalizer(agentTemplate, agentTemplateFinalizerName)
	if err := r.Update(ctx, agentTemplate); err != nil {
		log.Error(err, "Failed to remove finalizer from AgentTemplate")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// validateSpec validates the AgentTemplate spec
func (r *AgentTemplateReconciler) validateSpec(ctx context.Context, agentTemplate *corev1alpha1.AgentTemplate) error {
	spec := &agentTemplate.Spec

	// Validate provider
	validProviders := map[string]bool{
		"anthropic":    true,
		"openai":       true,
		"ollama":       true,
		"vertex":       true,
		"azure-openai": true,
		"mistral":      true,
	}
	if !validProviders[spec.Model.Worker.Provider] {
		return fmt.Errorf("unknown provider: %s", spec.Model.Worker.Provider)
	}

	// Validate apiName
	if spec.Model.Worker.ModelName == "" {
		return fmt.Errorf("model.worker.apiName must not be empty")
	}

	// Validate timeout
	if _, err := time.ParseDuration(spec.Limits.Timeout); err != nil {
		return fmt.Errorf("invalid timeout duration: %s", spec.Limits.Timeout)
	}

	// Validate completion condition
	validConditions := map[string]bool{
		"objective-achieved": true,
		"max-steps":          true,
		"timeout":            true,
	}
	if !validConditions[spec.Lifecycle.Completion.Condition] {
		return fmt.Errorf("invalid completion condition: %s", spec.Lifecycle.Completion.Condition)
	}

	// Validate onSuccess
	validDispositions := map[string]bool{
		"delete":  true,
		"retain":  true,
		"archive": true,
		"":        true, // empty is allowed (will use default)
	}
	if !validDispositions[spec.Lifecycle.Completion.OnSuccess] {
		return fmt.Errorf("invalid onSuccess disposition: %s", spec.Lifecycle.Completion.OnSuccess)
	}

	// Validate onFailure
	if !validDispositions[spec.Lifecycle.Completion.OnFailure] {
		return fmt.Errorf("invalid onFailure disposition: %s", spec.Lifecycle.Completion.OnFailure)
	}

	// Validate ConfigMap references
	if spec.Context != nil {
		for _, cmName := range spec.Context.ConfigMapRefs {
			cm := &corev1.ConfigMap{}
			if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentTemplate.Namespace}, cm); err != nil {
				if apierrors.IsNotFound(err) {
					return fmt.Errorf("referenced ConfigMap not found: %s", cmName)
				}
				return fmt.Errorf("failed to get ConfigMap %s: %w", cmName, err)
			}
		}

		// Validate Secret references
		for _, secretName := range spec.Context.SecretRefs {
			secret := &corev1.Secret{}
			if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: agentTemplate.Namespace}, secret); err != nil {
				if apierrors.IsNotFound(err) {
					return fmt.Errorf("referenced Secret not found: %s", secretName)
				}
				return fmt.Errorf("failed to get Secret %s: %w", secretName, err)
			}
		}
	}

	return nil
}

// setCondition sets or updates a condition in the AgentTemplate status
func (r *AgentTemplateReconciler) setCondition(agentTemplate *corev1alpha1.AgentTemplate, condition metav1.Condition) {
	condition.LastTransitionTime = metav1.Now()
	meta.SetStatusCondition(&agentTemplate.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.AgentTemplate{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentTemplatesForConfigMap),
		).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentTemplatesForSecret),
		).
		Watches(
			&corev1alpha1.Agent{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentTemplatesForAgent),
		).
		Named("agenttemplate").
		Complete(r)
}

// findAgentTemplatesForConfigMap finds all AgentTemplates that reference a ConfigMap
func (r *AgentTemplateReconciler) findAgentTemplatesForConfigMap(ctx context.Context, configMap client.Object) []reconcile.Request {
	agentTemplateList := &corev1alpha1.AgentTemplateList{}
	if err := r.List(ctx, agentTemplateList, client.InNamespace(configMap.GetNamespace())); err != nil {
		return []reconcile.Request{}
	}

	var requests []reconcile.Request
	for _, template := range agentTemplateList.Items {
		if template.Spec.Context != nil {
			for _, cmName := range template.Spec.Context.ConfigMapRefs {
				if cmName == configMap.GetName() {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      template.Name,
							Namespace: template.Namespace,
						},
					})
					break
				}
			}
		}
	}
	return requests
}

// findAgentTemplatesForSecret finds all AgentTemplates that reference a Secret
func (r *AgentTemplateReconciler) findAgentTemplatesForSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	agentTemplateList := &corev1alpha1.AgentTemplateList{}
	if err := r.List(ctx, agentTemplateList, client.InNamespace(secret.GetNamespace())); err != nil {
		return []reconcile.Request{}
	}

	var requests []reconcile.Request
	for _, template := range agentTemplateList.Items {
		if template.Spec.Context != nil {
			for _, secretName := range template.Spec.Context.SecretRefs {
				if secretName == secret.GetName() {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      template.Name,
							Namespace: template.Namespace,
						},
					})
					break
				}
			}
		}
	}
	return requests
}

// findAgentTemplatesForAgent finds all AgentTemplates that are referenced by an Agent
func (r *AgentTemplateReconciler) findAgentTemplatesForAgent(ctx context.Context, agent client.Object) []reconcile.Request {
	agentObj, ok := agent.(*corev1alpha1.Agent)
	if !ok {
		return []reconcile.Request{}
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      agentObj.Spec.TemplateRef.Name,
				Namespace: agentObj.Namespace,
			},
		},
	}
}
