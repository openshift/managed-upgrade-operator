# AI Insights POC for Managed Upgrade Operator

## Overview

The AI Insights POC introduces an **UpgradeInsight** controller that provides AI-powered analysis of OpenShift cluster upgrades without taking any actions. It serves as an advisory companion to MUO, helping SREs understand complex upgrade scenarios through natural language summaries.

This POC is a proof of concept and is not intended for production use.


## Installation & Testing the POC

### 1. Install the CRD

```bash
oc apply -f deploy/crds/upgradeinsight_crd.yaml
```

### 2. Create LLM Configuration Secret

Edit [deploy/examples/ai-insights-secret.yaml](../deploy/examples/ai-insights-secret.yaml) with your LLM details and then apply:

```bash
oc apply -f deploy/examples/ai-insights-secret.yaml
```

### 3. Scale down the running MUO deployment to zero replicas

```bash 
oc scale deployment managed-upgrade-operator -n openshift-managed-upgrade-operator --replicas=0
```
### 4. Create a new MUO deployment for testing

```bash
oc apply -f deploy/examples/ai-inisghts-muo-deployment-example.yaml
```
Note - You can build your own image with `podman build --platform=linux/amd64 -t quay.io/<your_username>/managed-upgrade-operator:test-latest -f build/Dockerfile .` and use it for the deployment image. Ensure to replace the image name in `deploy/examples/ai-inisghts-muo-deployment-example.yaml`

### 5. Update RBAC

The controller needs additional permissions for UpgradeInsights and Secrets:

```bash
oc apply -f deploy/cluster_role.yaml
```

### 6. Create an Example UpgradeInsight

```bash
oc apply -f deploy/examples/ai-insights-upgradeinsight-example.yaml
```

### 7. Create an Example UpgradeConfig

```bash
oc apply -f deploy/examples/ai-insights-upgradeconfig-example.yaml
```
Note - Ensure to update the `upgradeAt` field with a future date-time.

### 8. Deploy/Restart the Operator

If the operator is already running, restart it to pick up the new changes:

```bash
oc rollout restart deployment managed-upgrade-operator -n openshift-managed-upgrade-operator
```

### Check Status

```bash
# View all insights
oc get upgradeinsights

# Get detailed status
oc get upgradeinsight example-upgrade-insight -o yaml
```

### Example Output

The controller updates the `.status.insights` field with AI-generated analysis:

```yaml
status:
  observedAt: "2025-11-21T12:00:00Z"
  modelName: "gpt-4"
  insights:
    readiness:
      summary: "Cluster is mostly ready. Two PDBs may slow worker node draining."
      riskScore: 35
      blockers:
        - resource: "pdb/frontend-pdb"
          reason: "minAvailable=2 prevents draining during upgrade"
          severity: "medium"
    eventClassification:
      noiseCount: 142
      concerning:
        - type: "NodeDrainTimeout"
          explanation: "Node failed to drain due to unmovable pod"
          count: 3
    postUpgrade:
      issues:
        - component: "openshift-authentication"
          description: "OAuth pods restarting frequently"
          impact: "Users may experience intermittent login failures"
      recommendations:
        - "Check oauth-openshift pod logs for errors"
        - "Verify OIDC provider connectivity"
    logTailAnalysis:
      summary: "Multiple error patterns detected in operator pods"
      errorPatterns:
        - pattern: "connection refused"
          occurrences: 12
          explanation: "Likely transient network issues during control plane upgrade"
```

## Insight Types

### 1. `readiness`
Pre-upgrade cluster readiness assessment:
- Identifies potential blockers (PDBs, degraded operators, resource constraints)
- Provides risk score (0-100)
- Lists blockers with severity

### 2. `event-classification`
Event analysis:
- Classifies events as noise vs concerning
- Identifies patterns (timeouts, failures, restarts)
- Groups similar events

### 3. `post-upgrade-debug`
Post-upgrade troubleshooting:
- Identifies degraded components
- Explains failure chains
- Provides actionable recommendations

### 4. `log-tail-analysis`
Pod log analysis:
- Detects error patterns
- Highlights anomalous log entries
- Correlates errors across pods

## Configuration Options

| Field | Default | Description |
|-------|---------|-------------|
| `maxEvents` | 200 | Maximum events to include in analysis |
| `includeTimeWindowMinutes` | 60 | Time window for event collection |
| `maxLogLines` | 100 | Log lines to analyze per pod |
| `targetPods` | (all) | Label selector for pods to analyze |



## Troubleshooting
Common errors:
- "Failed to fetch UpgradeConfig" - Target UpgradeConfig doesn't exist
- "LLM call failed" - API endpoint unreachable or invalid credentials
- "Failed to parse LLM response" - LLM returned invalid JSON