# Managed Upgrade Operator - Metrics Tracing Guide

This document provides a comprehensive mapping of all Prometheus metrics defined in `pkg/metrics/metrics.go`, showing where they are triggered throughout the codebase and explaining why each metric fires.

## Table of Contents

1. [Overview](#overview)
2. [Validation Metrics](#validation-metrics)
3. [Scaling Metrics](#scaling-metrics)
4. [Upgrade Window Metrics](#upgrade-window-metrics)
5. [Control Plane Timeout Metrics](#control-plane-timeout-metrics)
6. [Health Check Metrics](#health-check-metrics)
7. [Worker Timeout Metrics](#worker-timeout-metrics)
8. [Node Drain Metrics](#node-drain-metrics)
9. [Notification Metrics](#notification-metrics)
10. [Timestamp Metrics](#timestamp-metrics)
11. [Upgrade Result Metrics](#upgrade-result-metrics)
12. [Reset Operations](#reset-operations)
13. [E2E Test Coverage](#e2e-test-coverage)

## Overview

The Managed Upgrade Operator exposes **17 Prometheus metrics** organized into two categories:

- **Ephemeral Metrics (16)**: Reset when upgrade completes or UpgradeConfig is deleted
- **Persistent Metrics (1)**: Retained across upgrades to track historical results

All metrics use the `upgradeoperator` subsystem prefix (except `upgrade_notification_failed`).

---

## Validation Metrics

### `upgradeoperator_upgradeconfig_validation_failed`

**Metric Type**: GaugeVec
**Labels**: `upgradeconfig_name`
**Values**: `1` = failed, `0` = succeeded

#### Set to 1 (Failed)
- **File**: `controllers/upgradeconfig/upgradeconfig_controller.go:209`
- **Method**: `UpdateMetricValidationFailed(instance.Name)`
- **Trigger**: `validator.IsValidUpgradeConfig()` returns invalid or error
- **Why**: UpgradeConfig CR has validation issues:
  - Invalid `upgradeAt` time format
  - Missing required fields
  - Invalid channel or version
  - Scheduling conflicts

#### Set to 0 (Succeeded)
- **File**: `controllers/upgradeconfig/upgradeconfig_controller.go:213`
- **Method**: `UpdateMetricValidationSucceeded(instance.Name)`
- **Trigger**: Validation passes successfully
- **Why**: UpgradeConfig is valid and ready to schedule

**Alert**: `UpgradeConfigValidationFailedSRE` (paging)

---

## Scaling Metrics

### `upgradeoperator_scaling_failed`

**Metric Type**: GaugeVec
**Labels**: `upgradeconfig_name`
**Values**: `1` = failed, `0` = succeeded

#### Set to 1 (Failed)
- **File**: `pkg/upgraders/scalerstep.go:51`
- **Method**: `UpdateMetricScalingFailed(c.upgradeConfig.Name)`
- **Trigger**: `scaler.EnsureScaleUpNodes()` returns `ScaleTimeOutError`
- **Why**: Pre-upgrade capacity reservation failed
  - Extra worker nodes didn't become Ready in time
  - Timeout defined by `config.Scale.TimeOut`
  - Critical for ensuring customer capacity during upgrades

**Context**: Only applies when `spec.capacityReservation: true`

#### Set to 0 (Succeeded)
- **File**: `pkg/upgraders/scalerstep.go:62`
- **Method**: `UpdateMetricScalingSucceeded(c.upgradeConfig.Name)`
- **Trigger**: `scaler.EnsureScaleUpNodes()` completes successfully
- **Why**: Extra worker nodes scaled up and are Ready

---

## Upgrade Window Metrics

### `upgradeoperator_upgrade_window_breached`

**Metric Type**: GaugeVec
**Labels**: `upgradeconfig_name`
**Values**: `1` = breached, `0` = not breached

#### Set to 1 (Breached)
- **File**: `pkg/upgraders/osdupgrader.go:170`
- **Method**: `UpdateMetricUpgradeWindowBreached(upgradeConfig.Name)`
- **Trigger**: Upgrade didn't complete within maintenance window
- **Why**:
  - Upgrade exceeded `GetUpgradeWindowTimeOutDuration()`
  - Maintenance window closed before upgrade finished
  - SRE intervention likely required

**Alert**: Triggers escalation for incomplete upgrades

#### Set to 0 (Not Breached)
- **File**: `pkg/upgraders/controlplanestep.go:19`
- **Method**: `UpdateMetricUpgradeWindowNotBreached(c.upgradeConfig.Name)`
- **Trigger**: `CommenceUpgrade()` called successfully
- **Why**: Upgrade is proceeding within the allowed window

---

## Control Plane Timeout Metrics

### `upgradeoperator_controlplane_timeout`

**Metric Type**: GaugeVec
**Labels**: `upgradeconfig_name`, `version`
**Values**: `1` = timeout, `0` = no timeout

#### Set to 1 (Timeout)
- **File**: `pkg/upgraders/controlplanestep.go:80`
- **Method**: `UpdateMetricUpgradeControlPlaneTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)`
- **Trigger**: Control plane upgrade exceeds timeout
- **Condition**: `time.Now().After(upgradeStartTime.Add(upgradeTimeout))`
- **Timeout**: `config.Maintenance.GetControlPlaneDuration()`
- **Why**: Control plane components stuck:
  - Cluster Version Operator (CVO) issues
  - Master node upgrades hanging
  - API server availability problems

**Alert**: `UpgradeControlPlaneUpgradeTimeoutSRE` (paging)

#### Set to 0 (Success)
- **File**: `pkg/upgraders/controlplanestep.go:59`
- **Method**: `ResetMetricUpgradeControlPlaneTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)`
- **Trigger**: `cvClient.HasUpgradeCompleted()` returns true for control plane
- **Why**: Masters and CVO upgraded successfully within timeout

---

## Health Check Metrics

### `upgradeoperator_healthcheck_failed`

**Metric Type**: GaugeVec
**Labels**: `upgradeconfig_name`, `state`, `version`, `reason`
**Values**: `1` = failed, `0` = succeeded

**Health Check Reasons** (from `pkg/metrics/metrics.go:58-70`):
- `healthcheck_query_failed` - Cannot query Prometheus
- `critical_alerts_firing` - Critical alerts active
- `cluster_operators_degraded` - ClusterOperators not healthy
- `cluster_operator_status_failed` - Cannot query ClusterOperator status
- `default_worker_machinepool_not_found` - Missing worker MachinePool
- `cluster_node_query_failed` - Cannot query nodes
- `cluster_node_manually_cordoned` - Nodes manually cordoned
- `cluster_node_taint_unschedulable` - Nodes tainted unschedulable
- `cluster_invalid_pdb` - Invalid PodDisruptionBudgets
- `cluster_invalid_pdb_configuration` - PDB config issues
- `pdb_query_failed` - Cannot query PDBs
- `dvo_client_creation_failed` - Cannot create DVO client
- `dvo_metrics_query_failed` - Cannot query DVO metrics

#### Set to 1 (Failed)
- **File**: `pkg/upgraders/healthcheckstep.go` (multiple validators)
- **Methods**: Various health check implementations
- **Trigger**: Any pre-upgrade or post-upgrade health check fails
- **Why**: Cluster state is unhealthy for upgrade
  - Critical alerts prevent safe upgrade
  - Degraded operators may worsen during upgrade
  - Invalid PDBs could block node drains
  - Manual interventions detected 

**Alert**: `UpgradeClusterCheckFailedSRE` (paging)

#### Set to 0 (Succeeded)
- **File**: `pkg/upgraders/healthcheckstep.go` (multiple validators)
- **Trigger**: Health checks pass
- **Why**: Cluster is healthy and ready for upgrade

---

## Worker Timeout Metrics

### `upgradeoperator_worker_timeout`

**Metric Type**: GaugeVec
**Labels**: `upgradeconfig_name`, `version`
**Values**: `1` = timeout, `0` = no timeout

#### Set to 1 (Timeout)
- **File**: `pkg/upgraders/workerstep.go:28`
- **Method**: `UpdateMetricUpgradeWorkerTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)`
- **Trigger**: Workers still upgrading AND no active maintenance window
- **Condition**:
  - `upgradingResult.IsUpgrading == true`
  - `silenceActive == false`
- **Why**: Worker upgrades taking too long outside maintenance
  - Node drain issues
  - Machine rollout problems
  - MachineConfigPool stuck

**Alert**: `UpgradeNodeUpgradeTimeoutSRE` (paging)

#### Set to 0 (No Timeout)
- **File**: `pkg/upgraders/workerstep.go:31`
- **Method**: `ResetMetricUpgradeWorkerTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)`
- **Trigger**: Workers still upgrading BUT maintenance window is active
- **Why**: Upgrade is progressing within allowed timeframe

#### Also Set to 0 (Completed)
- **File**: `pkg/upgraders/workerstep.go:44`
- **Method**: `ResetMetricUpgradeWorkerTimeout(c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version)`
- **Trigger**: All workers upgraded successfully
- **Why**: Worker upgrade phase completed

---

## Node Drain Metrics

### `upgradeoperator_node_drain_timeout`

**Metric Type**: GaugeVec
**Labels**: `node_name`
**Values**: `1` = timeout, `0` = no timeout

#### Set to 1 (Timeout) - NodeKeeper Controller
- **File**: `controllers/nodekeeper/nodekeeper_controller.go:161`
- **Method**: `UpdateMetricNodeDrainFailed(node.Name)`
- **Trigger**: Node drain timed out during upgrade
- **Conditions**:
  - `drainStrategy.HasFailed() == true`
  - `node.DeletionTimestamp == nil`
  - `Machinery.IsNodeUpgrading(node) == true`
- **Why**: Node cannot evict all pods in time
  - PodDisruptionBudget blocking eviction
  - Pods without proper controllers
  - Volume detachment issues
  - Finalizers blocking pod deletion

**Alert**: `UpgradeNodeDrainFailedSRE` (paging)

#### Set to 1 (Timeout) - Scale Down
- **File**: `pkg/upgraders/scalerstep.go:99`
- **Method**: `UpdateMetricNodeDrainFailed(dtErr.GetNodeName())`
- **Trigger**: Extra capacity node failed to drain during scale-down
- **Condition**: `scaler.IsDrainTimeOutError(err) == true`
- **Why**: Cannot remove extra nodes after upgrade

#### Set to 0 (Success/Not Applicable) - NodeKeeper
- **File**: `controllers/nodekeeper/nodekeeper_controller.go:87`
- **Method**: `ResetMetricNodeDrainFailed(node.Name)`
- **Trigger**: Node is not cordoned
- **Why**: Node not undergoing drain operation

- **File**: `controllers/nodekeeper/nodekeeper_controller.go:156`
- **Method**: `ResetMetricNodeDrainFailed(node.Name)`
- **Trigger**: Node has DeletionTimestamp set
- **Why**: Node being deleted, drain metric no longer relevant

- **File**: `controllers/nodekeeper/nodekeeper_controller.go:165`
- **Method**: `ResetMetricNodeDrainFailed(node.Name)`
- **Trigger**: Drain succeeded
- **Why**: Node drained successfully

#### Reset All Nodes
- **File**: `pkg/upgraders/scalerstep.go:106`
- **Method**: `ResetAllMetricNodeDrainFailed()`
- **Trigger**: All extra scaled nodes removed successfully
- **Why**: Scale-down phase completed, clear all drain metrics

---

## Notification Metrics

### `upgrade_notification_failed`

**Metric Type**: GaugeVec
**Labels**: `upgradeconfig_name`, `event`
**Values**: `1` = failed, `0` = succeeded

#### Set to 1 (Failed)
- **File**: `pkg/eventmanager/eventmanager.go:157`
- **Method**: `UpdatemetricUpgradeNotificationFailed(uc.Name, string(state))`
- **Trigger**: `notifier.NotifyState()` returns error
- **Why**: Failed to send notification
  - OCM API unavailable
  - Network connectivity issues
  - Service log API errors
  - Authentication failures

#### Set to 0 (Succeeded)
- **File**: `pkg/eventmanager/eventmanager.go:160`
- **Method**: `UpdatemetricUpgradeNotificationSucceeded(uc.Name, string(state))`
- **Trigger**: Notification sent successfully
- **Why**: Event notification delivered to OCM/ServiceLog

### `upgradeoperator_upgrade_notification`

**Metric Type**: GaugeVec
**Labels**: `upgradeconfig_name`, `event`, `version`
**Values**: `1` = sent

**Event Types** (from `pkg/notifier`):
- `scheduled` - Upgrade scheduled
- `started` - Upgrade started
- `control_plane_started` - Control plane upgrade started
- `control_plane_completed` - Control plane upgrade completed
- `workers_started` - Worker upgrade started
- `workers_completed` - Worker upgrade completed
- `completed` - Upgrade completed
- `delayed` - Node drain delayed
- `skipped` - Upgrade skipped (scaling failed)

#### Set to 1 (Event Sent) - State Notifications
- **File**: `pkg/eventmanager/eventmanager.go:161`
- **Method**: `UpdateMetricNotificationEventSent(uc.Name, string(state), uc.Spec.Desired.Version)`
- **Trigger**: After successful state notification
- **Why**: Tracking which lifecycle events have been sent

#### Set to 1 (Event Sent) - Result Notifications
- **File**: `pkg/eventmanager/eventmanager.go:201`
- **Method**: `UpdateMetricNotificationEventSent(uc.Name, string(state), uc.Spec.Desired.Version)`
- **Trigger**: After sending upgrade result notification
- **Why**: Tracking upgrade outcome notifications

#### Set to 1 (Event Sent) - Delayed Notifications
- **File**: `pkg/drain/nodeDrainStrategy.go:92`
- **Method**: `UpdateMetricNotificationEventSent(ds.uc.Name, string(notifier.MuoStateDelayed), ds.uc.Spec.Desired.Version)`
- **Trigger**: Node drain taking longer than expected
- **Why**: Proactively notify about upgrade delays

**Usage**: Query to check if specific notification already sent to avoid duplicates

---

## Timestamp Metrics

These metrics track the upgrade lifecycle timeline. All values are Unix timestamps.

### `upgradeoperator_upgradeconfig_sync_timestamp`

**Metric Type**: GaugeVec
**Labels**: `upgradeconfig_name`
**Values**: Unix timestamp

- **File**: `pkg/upgradeconfigmanager/upgradeconfigmanager.go:168`
- **Method**: `UpdateMetricUpgradeConfigSyncTimestamp(UPGRADECONFIG_CR_NAME, time.Now())`
- **Trigger**: UpgradeConfig successfully synced from OCM
- **Why**: Track when upgrade policy was last synchronized
- **Purpose**: Monitor sync frequency and detect stale policies

### `upgradeoperator_upgrade_started_timestamp`

**Metric Type**: GaugeVec
**Labels**: `_id` (cluster ID), `upgradeconfig_name`, `version`
**Values**: Unix timestamp

- **File**: `pkg/upgraders/notifierstep.go:32`
- **Method**: `UpdateMetricUpgradeStartedTimestamp(clusterid, c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version, time.Now())`
- **Trigger**: Upgrade officially starts (scheduled → started transition)
- **Why**: Mark beginning of upgrade process
- **Purpose**: Calculate total upgrade duration

### `upgradeoperator_upgrade_completed_timestamp`

**Metric Type**: GaugeVec
**Labels**: `_id` (cluster ID), `upgradeconfig_name`, `version`
**Values**: Unix timestamp

- **File**: `pkg/upgraders/notifierstep.go:45`
- **Method**: `UpdateMetricUpgradeCompletedTimestamp(clusterid, c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version, time.Now())`
- **Trigger**: Entire upgrade completes successfully
- **Why**: Mark end of upgrade process
- **Purpose**: Calculate total upgrade duration

### `upgradeoperator_controlplane_upgrade_started_timestamp`

**Metric Type**: GaugeVec
**Labels**: `_id` (cluster ID), `upgradeconfig_name`, `version`
**Values**: Unix timestamp

- **File**: `pkg/upgraders/controlplanestep.go:35`
- **Method**: `UpdateMetricControlplaneUpgradeStartedTimestamp(clusterid, c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version, time.Now())`
- **Trigger**: Control plane upgrade begins via `CommenceUpgrade()`
- **Why**: Mark when CVO starts upgrading masters
- **Purpose**: Calculate control plane upgrade duration

### `upgradeoperator_controlplane_upgrade_completed_timestamp`

**Metric Type**: GaugeVec
**Labels**: `_id` (cluster ID), `upgradeconfig_name`, `version`
**Values**: Unix timestamp

- **File**: `pkg/upgraders/controlplanestep.go:61`
- **Method**: `UpdateMetricControlplaneUpgradeCompletedTimestamp(clusterid, c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version, time.Now())`
- **Trigger**: Control plane upgrade completes
- **Why**: Mark when all masters and CVO are upgraded
- **Purpose**: Calculate control plane upgrade duration

### `upgradeoperator_workernode_upgrade_started_timestamp`

**Metric Type**: GaugeVec
**Labels**: `_id` (cluster ID), `upgradeconfig_name`, `version`
**Values**: Unix timestamp

- **File**: `pkg/upgraders/controlplanestep.go:62`
- **Method**: `UpdateMetricWorkernodeUpgradeStartedTimestamp(clusterid, c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version, time.Now())`
- **Trigger**: Immediately after control plane completes (same method call)
- **Why**: Mark transition from control plane to worker upgrade phase
- **Purpose**: Calculate worker upgrade duration

### `upgradeoperator_workernode_upgrade_completed_timestamp`

**Metric Type**: GaugeVec
**Labels**: `_id` (cluster ID), `upgradeconfig_name`, `version`
**Values**: Unix timestamp

- **File**: `pkg/upgraders/workerstep.go:42`
- **Method**: `UpdateMetricWorkernodeUpgradeCompletedTimestamp(clusterid, c.upgradeConfig.Name, c.upgradeConfig.Spec.Desired.Version, time.Now())`
- **Trigger**: All worker nodes upgraded
- **Why**: Mark completion of worker node upgrade phase
- **Purpose**: Calculate worker upgrade duration

**Analysis**: Calculate phase durations with Prometheus queries:
```promql
# Total upgrade duration
upgradeoperator_upgrade_completed_timestamp - upgradeoperator_upgrade_started_timestamp

# Control plane duration
upgradeoperator_controlplane_upgrade_completed_timestamp - upgradeoperator_controlplane_upgrade_started_timestamp

# Worker duration
upgradeoperator_workernode_upgrade_completed_timestamp - upgradeoperator_workernode_upgrade_started_timestamp
```

---

## Upgrade Result Metrics

### `upgradeoperator_upgrade_result`

**Metric Type**: GaugeVec (PERSISTENT - not reset between upgrades)
**Labels**: `upgradeconfig_name`, `preceding_version`, `stream`, `version`, `alerts`
**Values**: `1` = success (no alerts), `0` = failure (alerts fired)

- **File**: `controllers/upgradeconfig/upgradeconfig_controller.go:326`
- **Method**: `UpdateMetricUpgradeResult(name, precedingVersion, version, minorUpgrade, upgradeAlerts)`
- **Trigger**: After upgrade completes, when recording final outcome
- **Why**: Permanent record of upgrade result
- **Data Captured**:
  - **preceding_version**: Version before upgrade (e.g., "4.14.0")
  - **version**: Target version (e.g., "4.15.0")
  - **stream**: Upgrade type
    - `"y"` = y-stream (minor version upgrade, e.g., 4.14 → 4.15)
    - `"z"` = z-stream (patch upgrade, e.g., 4.15.1 → 4.15.2)
  - **alerts**: Comma-separated list of paging alerts that fired during upgrade
  - **value**:
    - `1` = clean upgrade (no paging alerts)
    - `0` = problematic upgrade (paging alerts fired)

**Paging Alerts Tracked** (from `pkg/metrics/metrics.go:74-81`):
- `UpgradeConfigValidationFailedSRE`
- `UpgradeClusterCheckFailedSRE`
- `UpgradeControlPlaneUpgradeTimeoutSRE`
- `UpgradeNodeUpgradeTimeoutSRE`
- `UpgradeNodeDrainFailedSRE`

**Purpose**: Historical analysis of upgrade success rates by version and alert patterns

---

## Reset Operations

### `ResetEphemeralMetrics()`

**Method**: Clears ALL ephemeral metrics

- **File**: `controllers/upgradeconfig/upgradeconfig_controller.go:85`
- **Trigger**: When UpgradeConfig CR is deleted
- **Why**: Clean up temporary metrics after upgrade completion
- **Metrics Reset** (16 total):
  - `metricValidationFailed`
  - `metricScalingFailed`
  - `metricUpgradeWindowBreached`
  - `metricUpgradeControlPlaneTimeout`
  - `metricHealthcheckFailed`
  - `metricUpgradeWorkerTimeout`
  - `metricNodeDrainFailed`
  - `metricUpgradeNotification`
  - `metricUpgradeConfigSyncTimestamp`
  - `metricUpgradeNotificationFailed`
  - `upgradeStartedTimestamp`
  - `upgradeCompletedTimestamp`
  - `controlplaneUpgradeStartedTimestamp`
  - `controlplaneUpgradeCompletedTimestamp`
  - `workernodeUpgradeStartedTimestamp`
  - `workernodeUpgradeCompletedTimestamp`

**Not Reset**: `metricUpgradeResult` (persistent metric)

### `ResetFailureMetrics()`

**Method**: Clears failure-indicating metrics before retry

- **File**: `pkg/upgraders/osdupgrader.go:173`
- **Trigger**: When starting a new upgrade attempt
- **Why**: Clear previous failure indicators to allow fresh attempt
- **Metrics Reset** (9 total):
  - `metricValidationFailed`
  - `metricScalingFailed`
  - `metricUpgradeControlPlaneTimeout`
  - `metricHealthcheckFailed`
  - `metricUpgradeWorkerTimeout`
  - `metricNodeDrainFailed`
  - `metricUpgradeNotification`
  - `metricUpgradeNotificationFailed`
  - `upgradeStartedTimestamp`

**Context**: Called when upgrade window is breached, preparing for retry

---

## E2E Test Coverage

### Covered Metrics (1/17)

✅ **`upgradeoperator_upgradeconfig_validation_failed`**
- **Test**: `test/e2e/managed_upgrade_operator_tests.go:138-156`
- **Test Case**: "should raise prometheus metric if start time is invalid"
- **Coverage**:
  - Creates UpgradeConfig with invalid `upgradeAt` value
  - Polls Prometheus to verify metric appears
  - Validates metric value equals 1

### NOT Covered by E2E Tests (16/17)

The following metrics lack e2e test coverage:

**Scaling Metrics**
- ❌ `upgradeoperator_scaling_failed`

**Upgrade Window Metrics**
- ❌ `upgradeoperator_upgrade_window_breached`

**Timeout Metrics**
- ❌ `upgradeoperator_controlplane_timeout`
- ❌ `upgradeoperator_worker_timeout`

**Health Check Metrics**
- ❌ `upgradeoperator_healthcheck_failed`

**Node Drain Metrics**
- ❌ `upgradeoperator_node_drain_timeout`

**Notification Metrics**
- ❌ `upgrade_notification_failed`
- ❌ `upgradeoperator_upgrade_notification`

**Timestamp Metrics**
- ❌ `upgradeoperator_upgradeconfig_sync_timestamp`
- ❌ `upgradeoperator_upgrade_started_timestamp`
- ❌ `upgradeoperator_upgrade_completed_timestamp`
- ❌ `upgradeoperator_controlplane_upgrade_started_timestamp`
- ❌ `upgradeoperator_controlplane_upgrade_completed_timestamp`
- ❌ `upgradeoperator_workernode_upgrade_started_timestamp`
- ❌ `upgradeoperator_workernode_upgrade_completed_timestamp`

**Result Metrics**
- ❌ `upgradeoperator_upgrade_result`

**Note**: Full e2e coverage is challenging because many metrics require actual cluster upgrade execution or failure injection, which is time-consuming and resource-intensive for automated testing.

---

## Quick Reference

### Metric Name to File Mapping

| Metric | Primary Trigger Location |
|--------|-------------------------|
| `upgradeconfig_validation_failed` | `controllers/upgradeconfig/upgradeconfig_controller.go:209` |
| `scaling_failed` | `pkg/upgraders/scalerstep.go:51` |
| `upgrade_window_breached` | `pkg/upgraders/osdupgrader.go:170` |
| `controlplane_timeout` | `pkg/upgraders/controlplanestep.go:80` |
| `healthcheck_failed` | `pkg/upgraders/healthcheckstep.go` (multiple) |
| `worker_timeout` | `pkg/upgraders/workerstep.go:28` |
| `node_drain_timeout` | `controllers/nodekeeper/nodekeeper_controller.go:161` |
| `upgrade_notification_failed` | `pkg/eventmanager/eventmanager.go:157` |
| `upgrade_notification` | `pkg/eventmanager/eventmanager.go:161` |
| `upgradeconfig_sync_timestamp` | `pkg/upgradeconfigmanager/upgradeconfigmanager.go:168` |
| `upgrade_started_timestamp` | `pkg/upgraders/notifierstep.go:32` |
| `upgrade_completed_timestamp` | `pkg/upgraders/notifierstep.go:45` |
| `controlplane_upgrade_started_timestamp` | `pkg/upgraders/controlplanestep.go:35` |
| `controlplane_upgrade_completed_timestamp` | `pkg/upgraders/controlplanestep.go:61` |
| `workernode_upgrade_started_timestamp` | `pkg/upgraders/controlplanestep.go:62` |
| `workernode_upgrade_completed_timestamp` | `pkg/upgraders/workerstep.go:42` |
| `upgrade_result` | `controllers/upgradeconfig/upgradeconfig_controller.go:326` |

### Metrics by Upgrade Phase

**Pre-Upgrade**
- `upgradeconfig_validation_failed`
- `upgradeconfig_sync_timestamp`
- `scaling_failed`
- `healthcheck_failed` (pre-upgrade checks)

**Control Plane Upgrade**
- `upgrade_started_timestamp`
- `controlplane_upgrade_started_timestamp`
- `upgrade_window_breached`
- `controlplane_timeout`
- `controlplane_upgrade_completed_timestamp`
- `workernode_upgrade_started_timestamp`

**Worker Upgrade**
- `worker_timeout`
- `node_drain_timeout`
- `workernode_upgrade_completed_timestamp`
- `healthcheck_failed` (post-upgrade checks)

**Post-Upgrade**
- `upgrade_completed_timestamp`
- `upgrade_result`
- Scaling down extra nodes

**Throughout**
- `upgrade_notification`
- `upgrade_notification_failed`

---

## Related Documentation

- [Metrics Reference](metrics.md) - List of all exposed metrics
- [Alerts Reference](https://github.com/openshift/managed-cluster-config/blob/master/deploy/sre-prometheus/100-managed-upgrade-operator.PrometheusRule.yaml) - Alert definitions
- [Development Guide](development.md) - Setting up local development environment
- [Testing Guide](testing.md) - Running unit and e2e tests

---
