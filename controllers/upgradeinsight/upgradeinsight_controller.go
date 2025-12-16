package upgradeinsight

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/aiinsights"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName("controller_upgradeinsight")

const (
	// Default requeue interval
	requeueInterval = 5 * time.Minute

	// Secret name containing LLM configuration
	llmSecretName = "ai-insights-llm-config"

	// Secret keys
	secretKeyProvider    = "provider"
	secretKeyAPIEndpoint = "apiEndpoint"
	secretKeyAPIKey      = "apiKey"
	secretKeyModel       = "model"
	secretKeyProjectID   = "projectID"
	secretKeyLocation    = "location"
	secretKeyModelID     = "modelID"
)

// ReconcileUpgradeInsight reconciles an UpgradeInsight object
type ReconcileUpgradeInsight struct {
	Client        client.Client
	Scheme        *runtime.Scheme
	LLMAdapter    aiinsights.LLMAdapter
	PromptBuilder *aiinsights.PromptBuilder
	Log           logr.Logger
}

// Reconcile reads the UpgradeInsight object and performs AI analysis
func (r *ReconcileUpgradeInsight) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling UpgradeInsight")

	// Fetch the UpgradeInsight instance
	instance := &upgradev1alpha1.UpgradeInsight{}
	err := r.Client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Skip if disabled
	if instance.Spec.Disabled {
		reqLogger.Info("UpgradeInsight is disabled, skipping")
		return reconcile.Result{}, nil
	}

	// Initialize LLM adapter if needed
	if r.LLMAdapter == nil {
		adapter, err := r.initializeLLMAdapter(ctx, request.Namespace)
		if err != nil {
			reqLogger.Error(err, "Failed to initialize LLM adapter")
			return r.updateStatus(ctx, instance, "", err.Error())
		}
		r.LLMAdapter = adapter
	}

	// Fetch the referenced UpgradeConfig
	uc := &upgradev1alpha1.UpgradeConfig{}
	ucKey := types.NamespacedName{
		Name:      instance.Spec.Target.Name,
		Namespace: request.Namespace,
	}
	err = r.Client.Get(ctx, ucKey, uc)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to fetch UpgradeConfig: %v", err)
		reqLogger.Error(err, errMsg)
		return r.updateStatus(ctx, instance, "", errMsg)
	}

	// Collect events
	events, err := r.collectRelevantEvents(ctx, request.Namespace, uc, instance.Spec.Options)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to collect events: %v", err)
		reqLogger.Error(err, errMsg)
		return r.updateStatus(ctx, instance, "", errMsg)
	}
	reqLogger.Info("Collected events", "count", len(events))

	// Collect logs if log-tail-analysis is requested
	logs := make(map[string][]string)
	for _, insightType := range instance.Spec.InsightTypes {
		if insightType == upgradev1alpha1.InsightTypeLogTailAnalysis {
			logs, err = r.collectPodLogs(ctx, request.Namespace, instance.Spec.Options)
			if err != nil {
				reqLogger.Error(err, "Failed to collect pod logs, continuing without them")
			}
			break
		}
	}

	// Build prompt
	prompt := r.PromptBuilder.BuildPromptForInsights(uc, events, logs, instance.Spec.InsightTypes)
	reqLogger.Info("Built prompt", "length", len(prompt))

	// Call LLM with timeout
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	rawResponse, err := r.LLMAdapter.Analyze(callCtx, prompt)
	if err != nil {
		errMsg := fmt.Sprintf("LLM call failed: %v", err)
		reqLogger.Error(err, errMsg)
		return r.updateStatus(ctx, instance, "", errMsg)
	}

	// Parse JSON response (strip markdown code blocks if present)
	cleanedResponse := stripMarkdownCodeBlocks(rawResponse)
	var insightResult upgradev1alpha1.InsightResult
	if err := json.Unmarshal(cleanedResponse, &insightResult); err != nil {
		errMsg := fmt.Sprintf("Failed to parse LLM response as JSON: %v", err)
		reqLogger.Error(err, errMsg, "response", string(rawResponse))
		return r.updateStatus(ctx, instance, "", errMsg)
	}

	// Update status with insights
	now := metav1.Now()
	instance.Status.ObservedAt = &now
	instance.Status.ModelName = r.LLMAdapter.Name()
	instance.Status.Insights = insightResult
	instance.Status.Error = ""

	if err := r.Client.Status().Update(ctx, instance); err != nil {
		reqLogger.Error(err, "Failed to update UpgradeInsight status")
		return reconcile.Result{}, err
	}

	reqLogger.Info("Successfully updated UpgradeInsight with AI analysis")
	return reconcile.Result{RequeueAfter: requeueInterval}, nil
}

// initializeLLMAdapter creates an LLM adapter from secret configuration
func (r *ReconcileUpgradeInsight) initializeLLMAdapter(ctx context.Context, namespace string) (aiinsights.LLMAdapter, error) {
	// Fetch the secret
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      llmSecretName,
		Namespace: namespace,
	}

	err := r.Client.Get(ctx, secretKey, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM config secret: %w", err)
	}

	// Determine provider (defaults to "openai" for backward compatibility)
	provider := "openai"
	if p, ok := secret.Data[secretKeyProvider]; ok {
		provider = string(p)
	}

	// Extract base configuration
	config := aiinsights.DefaultLLMConfig()
	config.Provider = provider

	if apiKey, ok := secret.Data[secretKeyAPIKey]; ok {
		config.APIKey = string(apiKey)
	}

	if model, ok := secret.Data[secretKeyModel]; ok {
		config.Model = string(model)
	}

	// Create adapter based on provider type
	var adapter aiinsights.LLMAdapter

	switch provider {
	case "claude-vertex", "vertex":
		// Claude on Google Vertex AI
		vertexConfig := aiinsights.VertexLLMConfig{
			LLMConfig: config,
		}

		if projectID, ok := secret.Data[secretKeyProjectID]; ok {
			vertexConfig.ProjectID = string(projectID)
		} else {
			return nil, fmt.Errorf("vertex provider requires projectID in secret")
		}

		if location, ok := secret.Data[secretKeyLocation]; ok {
			vertexConfig.Location = string(location)
		} else {
			vertexConfig.Location = "us-east5" // Default location
		}

		if modelID, ok := secret.Data[secretKeyModelID]; ok {
			vertexConfig.ModelID = string(modelID)
		} else {
			vertexConfig.ModelID = "claude-3-5-sonnet-v2@20241022" // Default model
		}

		adapter = aiinsights.NewClaudeVertexLLMAdapter(vertexConfig)
		r.Log.Info("Initialized Claude Vertex AI adapter",
			"project", vertexConfig.ProjectID,
			"location", vertexConfig.Location,
			"model", vertexConfig.ModelID)

	case "openai", "http":
		// OpenAI-compatible API (default)
		if endpoint, ok := secret.Data[secretKeyAPIEndpoint]; ok {
			config.APIEndpoint = string(endpoint)
		} else {
			return nil, fmt.Errorf("openai/http provider requires apiEndpoint in secret")
		}
		adapter = aiinsights.NewHTTPLLMAdapter(config)
		r.Log.Info("Initialized HTTP/OpenAI LLM adapter", "model", config.Model, "endpoint", config.APIEndpoint)

	default:
		return nil, fmt.Errorf("unsupported provider: %s (supported: openai, claude-vertex)", provider)
	}

	return adapter, nil
}

// collectRelevantEvents collects Kubernetes events relevant to the upgrade
func (r *ReconcileUpgradeInsight) collectRelevantEvents(
	ctx context.Context,
	namespace string,
	uc *upgradev1alpha1.UpgradeConfig,
	options upgradev1alpha1.InsightOptions,
) ([]corev1.Event, error) {
	// Set defaults
	maxEvents := 200
	if options.MaxEvents > 0 {
		maxEvents = options.MaxEvents
	}

	timeWindow := 60
	if options.IncludeTimeWindowMinutes > 0 {
		timeWindow = options.IncludeTimeWindowMinutes
	}

	// Get all events across all namespaces
	eventList := &corev1.EventList{}
	err := r.Client.List(ctx, eventList)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	// Filter and sort events
	cutoffTime := time.Now().Add(-time.Duration(timeWindow) * time.Minute)
	var relevantEvents []corev1.Event

	for _, event := range eventList.Items {
		// Filter by time window
		if event.LastTimestamp.Time.Before(cutoffTime) {
			continue
		}

		// Sanitize event message
		event.Message = aiinsights.SanitizeEventMessage(event.Message)

		relevantEvents = append(relevantEvents, event)
	}

	// Sort by timestamp (most recent first)
	sort.Slice(relevantEvents, func(i, j int) bool {
		return relevantEvents[i].LastTimestamp.Time.After(relevantEvents[j].LastTimestamp.Time)
	})

	// Limit to maxEvents
	if len(relevantEvents) > maxEvents {
		relevantEvents = relevantEvents[:maxEvents]
	}

	return relevantEvents, nil
}

// collectPodLogs collects log tails from relevant pods
func (r *ReconcileUpgradeInsight) collectPodLogs(
	ctx context.Context,
	namespace string,
	options upgradev1alpha1.InsightOptions,
) (map[string][]string, error) {
	logs := make(map[string][]string)

	// Set defaults
	_ = 100 // maxLogLines would be used in actual log collection
	if options.MaxLogLines > 0 {
		_ = options.MaxLogLines
	}

	// List pods based on selector if provided
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{}

	if options.TargetPods != nil {
		selector, err := metav1.LabelSelectorAsSelector(options.TargetPods)
		if err != nil {
			return nil, fmt.Errorf("invalid pod selector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	// Look for upgrade-related pods in common namespaces
	namespaces := []string{
		"openshift-managed-upgrade-operator",
		"openshift-cluster-version",
		"openshift-machine-api",
		"openshift-machine-config-operator",
	}

	for _, ns := range namespaces {
		listOpts := append(listOpts, client.InNamespace(ns))
		if err := r.Client.List(ctx, podList, listOpts...); err != nil {
			r.Log.Error(err, "Failed to list pods in namespace", "namespace", ns)
			continue
		}

		for _, pod := range podList.Items {
			// In this POC, simulate log collection since actual log streaming
			// requires pods/log subresource access and streaming
			// In a real implementation, replace:
			// r.Client.CoreV1().Pods(ns).GetLogs(podName, &corev1.PodLogOptions{TailLines: &maxLines})

			// For now, just record that we found relevant pods
			podKey := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
			logs[podKey] = []string{
				fmt.Sprintf("Pod %s in phase %s", pod.Name, pod.Status.Phase),
				"[LOG COLLECTION PLACEHOLDER - Implement with pods/log subresource in production]",
			}
		}
	}

	return logs, nil
}

// updateStatus updates the UpgradeInsight status with an error
func (r *ReconcileUpgradeInsight) updateStatus(
	ctx context.Context,
	instance *upgradev1alpha1.UpgradeInsight,
	modelName string,
	errorMsg string,
) (reconcile.Result, error) {
	instance.Status.Error = errorMsg
	if modelName != "" {
		instance.Status.ModelName = modelName
	}
	if err := r.Client.Status().Update(ctx, instance); err != nil {
		r.Log.Error(err, "Failed to update status")
		return reconcile.Result{}, err
	}
	return reconcile.Result{RequeueAfter: requeueInterval}, nil
}

// stripMarkdownCodeBlocks removes markdown code block formatting from LLM responses
// LLMs often wrap JSON in ```json ... ``` blocks, this function extracts the JSON content
func stripMarkdownCodeBlocks(data []byte) []byte {
	content := string(data)
	content = strings.TrimSpace(content)

	// Check if wrapped in markdown code blocks
	if strings.HasPrefix(content, "```") {
		// Find the first newline after opening ```
		firstNewline := strings.Index(content, "\n")
		if firstNewline == -1 {
			return data // No newline found, return as-is
		}

		// Find the closing ```
		closingMarker := strings.LastIndex(content, "```")
		if closingMarker == -1 || closingMarker <= firstNewline {
			return data // No valid closing marker, return as-is
		}

		// Extract content between markers
		content = content[firstNewline+1 : closingMarker]
		content = strings.TrimSpace(content)
	}

	return []byte(content)
}

// SetupWithManager sets up the controller with the Manager
func (r *ReconcileUpgradeInsight) SetupWithManager(mgr ctrl.Manager) error {
	if r.PromptBuilder == nil {
		r.PromptBuilder = aiinsights.NewPromptBuilder()
	}
	if r.Log.GetSink() == nil {
		r.Log = log
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&upgradev1alpha1.UpgradeInsight{}).
		Complete(r)
}
