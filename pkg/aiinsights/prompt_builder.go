package aiinsights

import (
	"encoding/json"
	"fmt"
	"strings"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// PromptBuilder constructs prompts for LLM analysis
type PromptBuilder struct{}

// NewPromptBuilder creates a new prompt builder
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

// BuildPromptForInsights constructs a comprehensive prompt for the requested insight types
func (pb *PromptBuilder) BuildPromptForInsights(
	uc *upgradev1alpha1.UpgradeConfig,
	events []corev1.Event,
	logs map[string][]string,
	insightTypes []upgradev1alpha1.InsightType,
) string {
	var sb strings.Builder

	sb.WriteString("# OpenShift Upgrade Analysis Request\n\n")
	sb.WriteString("You are an expert OpenShift upgrade analyst. Analyze the provided data and respond with ONLY valid JSON.\n\n")

	// Add UpgradeConfig context
	pb.addUpgradeConfigContext(&sb, uc)

	// Add events if available
	if len(events) > 0 {
		pb.addEventsContext(&sb, events)
	}

	// Add logs if available
	if len(logs) > 0 {
		pb.addLogsContext(&sb, logs)
	}

	// Add insight type instructions
	pb.addInsightTypeInstructions(&sb, insightTypes)

	// Add JSON schema
	pb.addJSONSchema(&sb, insightTypes)

	return sb.String()
}

func (pb *PromptBuilder) addUpgradeConfigContext(sb *strings.Builder, uc *upgradev1alpha1.UpgradeConfig) {
	sb.WriteString("## UpgradeConfig Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Name**: %s\n", uc.Name))
	sb.WriteString(fmt.Sprintf("- **Target Version**: %s\n", uc.Spec.Desired.Version))
	sb.WriteString(fmt.Sprintf("- **Channel**: %s\n", uc.Spec.Desired.Channel))
	sb.WriteString(fmt.Sprintf("- **Upgrade Type**: %s\n", uc.Spec.Type))
	sb.WriteString(fmt.Sprintf("- **Scheduled At**: %s\n", uc.Spec.UpgradeAt))
	sb.WriteString(fmt.Sprintf("- **PDB Force Drain Timeout**: %d minutes\n", uc.Spec.PDBForceDrainTimeout))

	if len(uc.Status.History) > 0 {
		latest := uc.Status.History[0]
		sb.WriteString(fmt.Sprintf("- **Current Phase**: %s\n", latest.Phase))
		sb.WriteString(fmt.Sprintf("- **Previous Version**: %s\n", latest.PrecedingVersion))

		if len(latest.Conditions) > 0 {
			sb.WriteString("\n**Recent Conditions**:\n")
			for i, cond := range latest.Conditions {
				if i >= 5 { // Limit to 5 most recent
					break
				}
				sb.WriteString(fmt.Sprintf("- %s: %s", cond.Type, cond.Status))
				if cond.Message != "" {
					sb.WriteString(fmt.Sprintf(" (%s)", cond.Message))
				}
				sb.WriteString("\n")
			}
		}
	}
	sb.WriteString("\n")
}

func (pb *PromptBuilder) addEventsContext(sb *strings.Builder, events []corev1.Event) {
	sb.WriteString(fmt.Sprintf("## Kubernetes Events (%d events)\n\n", len(events)))
	sb.WriteString("Recent cluster events that may be relevant to the upgrade:\n\n")

	for i, event := range events {
		if i >= 100 { // Safety limit
			sb.WriteString(fmt.Sprintf("... and %d more events (truncated)\n", len(events)-i))
			break
		}

		sb.WriteString(fmt.Sprintf("- **%s** [%s/%s] %s: %s",
			event.LastTimestamp.Format("15:04:05"),
			event.InvolvedObject.Kind,
			event.InvolvedObject.Name,
			event.Type,
			event.Reason,
		))

		if event.Message != "" {
			// Truncate very long messages
			msg := event.Message
			if len(msg) > 200 {
				msg = msg[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf(" - %s", msg))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

func (pb *PromptBuilder) addLogsContext(sb *strings.Builder, logs map[string][]string) {
	sb.WriteString(fmt.Sprintf("## Pod Logs (%d pods)\n\n", len(logs)))
	sb.WriteString("Recent log tail from relevant pods:\n\n")

	for podName, logLines := range logs {
		sb.WriteString(fmt.Sprintf("### Pod: %s\n", podName))
		sb.WriteString("```\n")
		for i, line := range logLines {
			if i >= 50 { // Limit lines per pod
				sb.WriteString(fmt.Sprintf("... (%d more lines)\n", len(logLines)-i))
				break
			}
			sb.WriteString(line + "\n")
		}
		sb.WriteString("```\n\n")
	}
}

func (pb *PromptBuilder) addInsightTypeInstructions(sb *strings.Builder, insightTypes []upgradev1alpha1.InsightType) {
	sb.WriteString("## Analysis Instructions\n\n")
	sb.WriteString("Generate insights for the following types:\n\n")

	for _, iType := range insightTypes {
		switch iType {
		case upgradev1alpha1.InsightTypeReadiness:
			sb.WriteString("- **Readiness**: Assess cluster readiness for upgrade. Identify blockers like problematic PDBs, degraded operators, resource constraints.\n")
		case upgradev1alpha1.InsightTypeEventClassification:
			sb.WriteString("- **Event Classification**: Classify events as noise vs concerning. Identify patterns indicating failures, timeouts, or issues.\n")
		case upgradev1alpha1.InsightTypePostUpgradeDebug:
			sb.WriteString("- **Post-Upgrade Debug**: Identify post-upgrade issues, degraded components, and provide actionable recommendations.\n")
		case upgradev1alpha1.InsightTypeLogTailAnalysis:
			sb.WriteString("- **Log Tail Analysis**: Analyze logs for error patterns, anomalies, and concerning trends.\n")
		}
	}
	sb.WriteString("\n")
}

func (pb *PromptBuilder) addJSONSchema(sb *strings.Builder, insightTypes []upgradev1alpha1.InsightType) {
	sb.WriteString("## Required Output Format\n\n")
	sb.WriteString("Respond with ONLY valid JSON matching this structure:\n\n")
	sb.WriteString("```json\n")

	// Build schema based on requested insight types
	schema := make(map[string]interface{})

	for _, iType := range insightTypes {
		switch iType {
		case upgradev1alpha1.InsightTypeReadiness:
			schema["readiness"] = map[string]interface{}{
				"summary":   "Human-readable summary of cluster readiness",
				"riskScore": 0,
				"blockers": []map[string]string{
					{
						"resource": "resource identifier",
						"reason":   "why this blocks upgrade",
						"severity": "high|medium|low",
					},
				},
			}
		case upgradev1alpha1.InsightTypeEventClassification:
			schema["eventClassification"] = map[string]interface{}{
				"noiseCount": 0,
				"concerning": []map[string]interface{}{
					{
						"type":        "event type or category",
						"explanation": "why this is concerning",
						"count":       1,
					},
				},
			}
		case upgradev1alpha1.InsightTypePostUpgradeDebug:
			schema["postUpgrade"] = map[string]interface{}{
				"issues": []map[string]string{
					{
						"component":   "affected component",
						"description": "issue description",
						"impact":      "user-facing impact",
					},
				},
				"recommendations": []string{"actionable recommendation 1", "actionable recommendation 2"},
			}
		case upgradev1alpha1.InsightTypeLogTailAnalysis:
			schema["logTailAnalysis"] = map[string]interface{}{
				"summary": "Overview of log analysis",
				"errorPatterns": []map[string]interface{}{
					{
						"pattern":     "detected pattern",
						"occurrences": 0,
						"explanation": "context for pattern",
					},
				},
				"anomalousLogs": []map[string]string{
					{
						"pod":     "pod name",
						"message": "log message",
						"reason":  "why anomalous",
					},
				},
			}
		}
	}

	schemaBytes, _ := json.MarshalIndent(schema, "", "  ")
	sb.Write(schemaBytes)
	sb.WriteString("\n```\n\n")

	sb.WriteString("**IMPORTANT**: \n")
	sb.WriteString("- Return ONLY the JSON object, no markdown code blocks\n")
	sb.WriteString("- Do not include explanations outside the JSON structure\n")
	sb.WriteString("- Do not suggest kubectl commands or actions\n")
	sb.WriteString("- Focus on analysis and insights only\n")
}

// SanitizeEventMessage removes potentially sensitive information from event messages
func SanitizeEventMessage(msg string) string {
	// Simple sanitization - can be enhanced
	msg = strings.ReplaceAll(msg, "password=", "password=***")
	msg = strings.ReplaceAll(msg, "token=", "token=***")
	msg = strings.ReplaceAll(msg, "secret=", "secret=***")
	msg = strings.ReplaceAll(msg, "apiKey=", "apiKey=***")
	return msg
}

// SanitizeLogLine removes sensitive information from log lines
func SanitizeLogLine(line string) string {
	// Simple sanitization - can be enhanced
	lower := strings.ToLower(line)
	if strings.Contains(lower, "password") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "apikey") {
		// If line contains sensitive keywords, redact the value part
		return "[REDACTED: potentially sensitive content]"
	}
	return line
}
