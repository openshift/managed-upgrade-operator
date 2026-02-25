# Design

## Resources and components

This document describes the design of the Managed Upgrade Operator and the resources it uses.

The following diagram illustrates the main resources that the Managed Upgrade Operator interacts with.  

![Managed Upgrade Operator](images/managed-upgrade-operator-design.svg)

The operator is primarily driven through an `UpgradeConfig` custom resource, which defines the version of OpenShift that the cluster should be running at.

The `UpgradeConfig` can be created directly on the cluster which is not managed via Hive. Also this method can be used for development/testing purposes

For OpenShift Dedicated cluster you should not create `UpgradeConfig` directly. The process for scheduling an upgrade is automated and achieved using the [upgrade_policies API](https://api.openshift.com/#/default/get_api_clusters_mgmt_v1_clusters__cluster_id__upgrade_policies). 

The process for creating `upgrade-policy` is documented in [SOPs](https://github.com/openshift/ops-sop/blob/master/v4/howto/managed-upgrade.md).

## Custom Resource Definitions

### UpgradeConfig

#### Configuration

The `UpgradeConfig` Custom Resource Definition (CRD) defines the version of OpenShift Container Platform that the cluster should be upgraded to, when conditions allow.

For the purpose of upgrading a cluster, an `UpgradeConfig` resource _must_ be configured with the following properties:

| Item | Definition | Example |
| ---- | ---------- | ------- |
| `type` | The cluster upgrader to use when upgrading (valid values: `OSD`, `ARO`)| `OSD` |  
| `upgradeAt` | Timestamp indicating when the upgrade can commence (ISO-8601)| `2020-05-01T12:00:00Z` |
| `PDBForceDrainTimeout` | Duration in minutes that a PDB-blocked node is allowed to drain before a drain is forced | `120` |
| `desired.version` | The desired OCP release to upgrade to | `4.4.6` |
| `desired.channel` | The [channel](https://github.com/openshift/cincinnati/blob/master/docs/design/openshift.md#Channels) the Cluster Version Operator should be using to validate update versions | `fast-4.4` |
| `desired.image`   | The image digest that CVO should use to upgrade cluster.| quay.io/openshift-release-dev/ocp-release@sha256:783a2c963f35ccab38e82e6a8c7fa954c3a4551e07d2f43c06098828dd986ed4 |
| `capacityReservation` | If extra worker node(s) are needed during the upgrade to hold the customer workload | `true` |

A populated `UpgradeConfig` example is presented below:

```yaml
apiVersion: upgrade.managed.openshift.io/v1alpha1
kind: UpgradeConfig
metadata:
  name: managed-upgrade-config
spec:
  type: "OSD"
  upgradeAt: "2020-06-20T12:00:00Z"
  PDBForceDrainTimeout: 120
  capacityReservation: true
  desired:
    channel: "fast-4.4"
    version: "4.4.6"
```

The CRD is available to [view in the repository](../deploy/crds/upgrade.managed.openshift.io_upgradeconfigs_crd.yaml).

#### Status

The Managed Upgrade Operator will record the history of its efforts to apply the desired upgrade within the `UpgradeConfig`'s `status` section. Data within this section can be used to determine the operator's progress to apply the upgrade.

At the top level, the following fields are defined in a list, each list element representing a unique cluster version:

| Item | Definition | Example |
| ---- | ---------- | ------- |
| `version` | The cluster version that the operator events related to | `4.4.6` |
| `precedingVersion` | The cluster version that the cluster is upgrading from. Determined from the ClusterVersion resource's version history. | `4.4.5` |
| `startTime` | The ISO-8601 timestamp at which the upgrade commenced. | `2020-07-05T01:35:36Z` |
| `completeTime` | The ISO-8601 timestamp at which the upgrade completed. | `2020-07-05T01:35:36Z` |
| `workerStartTime` | The ISO-8601 timestamp at which worker node upgrades began, set by the MachineConfigPool controller when it detects worker pool updating. | `2020-07-05T02:10:00Z` |
| `workerCompleteTime` | The ISO-8601 timestamp at which all worker node upgrades finished, set by the MachineConfigPool controller when the worker pool reports updated. | `2020-07-05T03:05:00Z` |
| `phase` | The current phase of the upgrade's application | `New`, `Pending`, `Upgrading`, `Upgraded`, `Failed`, `Unknown` |
| `conditions` | Data pertaining to a particular upgrade step that the operator performs | - |

Within `conditions`, each upgrade step can record its own individual status. These conditions are similar to [Pod conditions](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/), but relate to upgrade steps.

| Item | Definition | Example |
| ---- | ---------- | ------- |
| `type` | The type of upgrade step being performed | `ClusterHealthyBeforeUpgrade` |
| `startTime` | The ISO-8601 timestamp at which the step commenced. | `2020-07-05T01:35:36Z` |
| `completeTime` | The ISO-8601 timestamp at which the step completed. | `2020-07-05T01:35:36Z` |
| `lastProbeTime` | The last time this step's condition was last probed | `2020-07-05T01:35:36Z` |
| `lastTransitionTime` | The last time this step transitioend from one status to another | `2020-07-05T01:35:36Z` |
| `message` | Human-readable details indicating details about the transition | `ClusterHealthyBeforeUpgrade is completed` |
| `reason` | Human-readable details about why the transition has occurred | `Cluster has critical alerts` |
| `status` | Status of the condition | `True`, `False`, `Unknown` |

A fully-populated example of an `UpgradeConfig` status is included below:

```yaml
  status:
    history:
    - phase: Upgraded
      version: 4.3.26
      precedingVersion: 4.3.25
      startTime: "2020-07-05T01:35:36Z"
      completeTime: "2020-07-05T03:15:37Z"
      workerStartTime: "2020-07-05T02:10:00Z"
      workerCompleteTime: "2020-07-05T03:05:00Z"                                                     
      conditions:            
      - completeTime: "2020-07-05T03:15:36Z"
        lastProbeTime: "2020-07-05T03:15:36Z"
        lastTransitionTime: "2020-07-05T03:15:36Z"                                             
        message: ComputeCapacityReserved is completed
        reason: ComputeCapacityReserved done
        startTime: "2020-07-05T03:15:36Z"
        status: "True"
        type: ComputeCapacityReserved
      - completeTime: "2020-07-05T03:15:36Z"
        lastProbeTime: "2020-07-05T03:15:36Z"
        lastTransitionTime: "2020-07-05T03:15:36Z"                                             
        message: ClusterHealthyBeforeUpgrade is completed
        reason: ClusterHealthyBeforeUpgrade done
        startTime: "2020-07-05T03:15:36Z"
        status: "True"
        type: ClusterHealthyBeforeUpgrade
```

## Config Managers

The `managed-upgrade-operator` provides a configurable mechanism for retrieving and storing an `UpgradeConfig`
Custom Resource that can be reconciled against by the `UpgradeConfig` controller.

For more information, see the dedicated section on this topic: [UpgradeConfig Managers](configmanager.md)

## External Service Clients

The operator communicates with several external services using HTTP clients with standardized configuration:

### OCM Client

The OCM client uses the official OpenShift Cluster Manager SDK (`github.com/openshift-online/ocm-sdk-go`) to interact with:
- Cluster information API (`/api/clusters_mgmt/v1/clusters/{cluster_id}`)
- Upgrade policies API (`/api/clusters_mgmt/v1/clusters/{cluster_id}/upgrade_policies`)
- Service logs API (`/api/service_logs/v1/clusters/{cluster_id}/service_logs`)

**Features:**
- Typed SDK models (`cmv1.Cluster`, `cmv1.UpgradePolicy`, `cmv1.UpgradePolicyState`, `servicelogsv1.LogEntry`)
- Automatic retry with exponential backoff (5 retries, 2-second initial delay, 30% jitter)
- Proxy support via environment variables (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`)
- Configurable timeouts (30s connection, 10s TLS handshake, 30s keep-alive)

**Implementation**: See `pkg/ocm/client.go` and `pkg/ocm/builder.go`

### OCM Agent Client

For clusters with the OCM Agent operator deployed, the operator can communicate with a local OCM Agent service instead of the external OCM API. The OCM Agent client:
- Uses the same OCM SDK interfaces as the OCM client
- Communicates with local service URL: `http://ocm-agent.openshift-ocm-agent-operator.svc.cluster.local:8081`
- Does not use proxy configuration (local cluster communication only)
- Uses custom authentication transport with cluster access token

**Implementation**: See `pkg/ocmagent/client.go` and `pkg/ocmagent/builder.go`

### Other External Clients

- **DVO Client** (`pkg/dvo/client.go`): Deployment Validation Operator client with proxy support
- **AlertManager Client** (`pkg/maintenance/alertmanagerMaintenance.go`): Alert silencing with proxy support
- **Metrics Client** (`pkg/metrics/metrics.go`): Prometheus metrics with proxy support

All external clients support proxy configuration and use enhanced timeout settings for reliable communication in various network environments.

## Controllers

The `managed upgrade operator` provided upgrade process revolves around multiple Controllers. Alongside the above mentioned `UpgradeConfig` controller, the `NodeKeeper` controller works simultaneously in an upgrade process towards the state of nodes in the cluster.

The `NodeKeeper` controller keeps a track of the upgrading worker nodes during an upgrade and seeks to ensure their timely and eventual upgrade.

If an upgrading worker node is experiencing difficulty draining due to conditions such as [Pod Disruption Budgets](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/#pod-disruption-budgets) or stuck finalizers, the `NodeKeeper` controller will perform remediation strategies to ensure the node's eventual drain and subsequent upgrade continuation.
The `NodeKeeper` controller will flag through metrics any worker node that continue to unsuccessfully drain in spite of the remediation strategies.

## Upgrade Process
 
### Cluster Upgrader

The steps performed by the Managed Upgrade Operator are carried out by implementations of the [clusterUpgrader](../pkg/upgraders/upgrader.go) interface.

Each `clusterUpgrader` implementation must define an ordered series of [UpgradeSteps](../pkg/upgradesteps/runner.go), which represent idempotent conditions that together form the procedure of a managed cluster upgrade.

`UpgradeStep`s are homogeneous functions of code that carry out a part of the upgrade process. If the step has completed, it will return `true`. If the step has not completed, it will return `false`. If the step has failed, it will return an `false` and an accompanying error.

When actively performing a cluster upgrade, the operator will follow the process below during each iteration of the controller reconcile loop:
- Get the first step in the ordered list.
- Execute the step.
  - If the step returns `true` indicating it has successfully completed, move to the next step.
  - If the step returns `false` indicating it has not successfully completed, the operator will stop processing steps and restart on the next reconcile loop.
  - If the step returns an error, the error will be passed back up to the controller and logged, and the current step execution will be restarted on the next reconcile loop.

Steps should generally be idempotent in nature; if they have already run and completed during an upgrade, they should return `true` for subsequent calls and not attempt to re-perform the same action. An example of this is the `ControlPlaneMaintWindow` step to create a maintenance window.

This overall process of executing Upgrade Steps is illustrated below.

![Managed Upgrade Operator](images/upgradecluster-flow.svg)

To define a new custom procedure for performing a cluster upgrade, a developer should:
- Create a new implementation of the `clusterUpgrader` that defines a unique order of `UpgradeStep`s.
- Implement any missing or new `UpgradeStep`s that need to be performed.

#### OSD Upgrader Steps

The [osdUpgrader](../pkg/upgraders/osdupgrader.go) defines the following ordered steps (17 total):

| # | Condition Type | Function | Description |
| --- | --- | --- | --- |
| 1 | `StartedNotificationSent` | `SendStartedNotification` | Notify external systems that upgrade has started |
| 2 | `StartedNotificationSent` | `UpgradeDelayedCheck` | Check if upgrade is delayed past the configured `delayTrigger` window |
| 3 | `IsClusterUpgradable` | `IsUpgradeable` | Verify the ClusterVersion `Upgradeable` condition permits the upgrade |
| 4 | `ClusterHealthyBeforeUpgrade` | `PreUpgradeHealthCheck` | Pre-upgrade health checks (alerts, operators, nodes, PDBs) |
| 5 | `ExternalDependenciesAvailable` | `ExternalDependencyAvailabilityCheck` | Validate external HTTP dependencies are reachable |
| 6 | `ComputeCapacityReserved` | `EnsureExtraUpgradeWorkers` | Scale up extra worker node(s) if `capacityReservation` is enabled |
| 7 | `ControlPlaneMaintenanceWindowCreated` | `CreateControlPlaneMaintWindow` | Create AlertManager silence for control plane upgrade |
| 8 | `UpgradeCommenced` | `CommenceUpgrade` | Update ClusterVersion to trigger CVO upgrade |
| 9 | `ControlPlaneUpgraded` | `ControlPlaneUpgraded` | Wait for control plane upgrade to complete |
| 10 | `ControlPlaneMaintenanceWindowRemoved` | `RemoveControlPlaneMaintWindow` | Remove control plane AlertManager silence |
| 11 | `WorkersMaintenanceWindowCreated` | `CreateWorkerMaintWindow` | Create AlertManager silence for worker node upgrades |
| 12 | `WorkerNodesUpgraded` | `AllWorkersUpgraded` | Wait for all worker nodes to finish upgrading |
| 13 | `ComputeCapacityRemoved` | `RemoveExtraScaledNodes` | Scale down extra worker node(s) added in step 6 |
| 14 | `WorkersMaintenanceWindowRemoved` | `RemoveMaintWindow` | Remove worker AlertManager silence |
| 15 | `ClusterHealthyAfterUpgrade` | `PostUpgradeHealthCheck` | Post-upgrade health checks (alerts, operators) |
| 16 | `PostUpgradeTasksCompleted` | `PostUpgradeProcedures` | FedRAMP-specific post-upgrade tasks (File Integrity Operator re-init) |
| 17 | `CompletedNotificationSent` | `SendCompletedNotification` | Notify external systems that upgrade has completed |

Note: Step 2 (`UpgradeDelayedCheck`) reuses the `StartedNotificationSent` condition type rather than tracking under its own condition.

The OSD upgrader also enforces an upgrade failure policy: if the control plane upgrade has not commenced within the configured `upgradeWindow.timeOut` duration, the upgrade is marked as `Failed`, extra scaled nodes are removed, and a failure notification is sent.

#### ARO Upgrader Steps

The [aroUpgrader](../pkg/upgraders/aroupgrader.go) defines a subset of the OSD steps (14 total):

| # | Condition Type | Function |
| --- | --- | --- |
| 1 | `StartedNotificationSent` | `SendStartedNotification` |
| 2 | `ClusterHealthyBeforeUpgrade` | `PreUpgradeHealthCheck` |
| 3 | `ExternalDependenciesAvailable` | `ExternalDependencyAvailabilityCheck` |
| 4 | `ComputeCapacityReserved` | `EnsureExtraUpgradeWorkers` |
| 5 | `ControlPlaneMaintenanceWindowCreated` | `CreateControlPlaneMaintWindow` |
| 6 | `UpgradeCommenced` | `CommenceUpgrade` |
| 7 | `ControlPlaneUpgraded` | `ControlPlaneUpgraded` |
| 8 | `ControlPlaneMaintenanceWindowRemoved` | `RemoveControlPlaneMaintWindow` |
| 9 | `WorkersMaintenanceWindowCreated` | `CreateWorkerMaintWindow` |
| 10 | `WorkerNodesUpgraded` | `AllWorkersUpgraded` |
| 11 | `ComputeCapacityRemoved` | `RemoveExtraScaledNodes` |
| 12 | `WorkersMaintenanceWindowRemoved` | `RemoveMaintWindow` |
| 13 | `ClusterHealthyAfterUpgrade` | `PostUpgradeHealthCheck` |
| 14 | `CompletedNotificationSent` | `SendCompletedNotification` |

Compared to the OSD upgrader, the ARO upgrader omits:
- `UpgradeDelayedCheck` — no upgrade window delay monitoring
- `IsClusterUpgradable` — no explicit upgradeable condition check
- `PostUpgradeProcedures` — no FedRAMP post-upgrade tasks
- Upgrade failure timeout policy — upgrades are not failed if they don't commence within a time window

### Ready to upgrade criteria

The `UpgradeConfig` controller will only attempt to perform an upgrade if the current system time is later than the `upgradeAt` timestamp specified in the `UpgradeConfig` CR.

For example:

| `upgradeAt` time | Current time | Commence Upgrade? |
| --- | --- | --- |
| `2020-05-01 12:00:00` | `2020-05-01 11:50:00` | No, it is not yet 12:00 |
| `2020-05-01 12:00:00` | `2020-05-01 12:15:00` | Yes, an upgrade can commence |

Specific `clusterUpgrader`s can incorporate additional ready-to-upgrade criteria in their `UpgradeCluster()` implementation.

#### OSD Upgrade Failure Policy

The `osdUpgrader` enforces an upgrade window timeout. Before executing any upgrade steps, it checks whether the control plane upgrade has commenced (via the ClusterVersion resource). If the upgrade has **not** commenced and the current time exceeds `startTime + upgradeWindow.timeOut`, the upgrade is treated as failed.

The failure procedure ([`performUpgradeFailure`](../pkg/upgraders/osdupgrader.go)) carries out the following actions:

1. **Scale down** any extra machinesets created for capacity reservation
2. **Send failure notification** to external systems via the event manager
3. **Flag the `UpgradeWindowBreached` metric** in Prometheus
4. **Reset failure metrics** from any prior upgrade attempts
5. **Set the upgrade phase to `Failed`** with a `FailedUpgrade` condition

Once the control plane upgrade has commenced, the failure policy no longer applies — the upgrade cannot be rolled back.

The timeout duration is configured via `upgradeWindow.timeOut` in the operator [ConfigMap](configmap.md#upgradewindow) (default: 120 minutes). The ARO upgrader does not implement this policy.

### Validating upgrade versions

The following checks are made against the desired version in the `UpgradeConfig` to assert that it is a valid version to upgrade to.

* The version to upgrade to is greater than the currently-installed version (rollbacks are not supported)
* The [Cluster Version Operator](https://github.com/openshift/cluster-version-operator) reports it as an available version to upgrade to.

### Feature Gates

The operator supports feature gates that can be enabled via the [ConfigMap](configmap.md#featuregate) to selectively activate optional behavior. Feature gates are defined in [`upgradeconfig_types.go`](../api/v1alpha1/upgradeconfig_types.go) and are disabled by default.

| Feature Gate | Description |
| --- | --- |
| `PreHealthCheck` | When enabled, runs extended pre-upgrade health checks (critical alerts, cluster operators, capacity reservation, cordoned nodes, node taints, PDB validation) during the `New` phase if the upgrade is scheduled more than two hours away. Health checks during the `Upgrading` phase always run regardless of this gate. |
| `ServiceLogNotification` | When enabled, sends additional service log notifications to OCM during upgrade stages (control plane start/finish, worker plane finish, health check failures). |

Feature gates are configured in the operator ConfigMap:

```yaml
featureGate:
  enabled:
  - PreHealthCheck
  - ServiceLogNotification
```
