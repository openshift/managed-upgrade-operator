# Design

## Resources and components

This document describes the design of the Managed Upgrade Operator and the resources it uses.

The following diagram illustrates the main resources that the Managed Upgrade Operator interacts with.  

![Managed Upgrade Operator](images/managed-upgrade-operator-design.svg)

The operator is primarily driven through an `UpgradeConfig` custom resource, which defines the version of OpenShift that the cluster should be running at.

The `UpgradeConfig` can be created directly on the cluster for development/testing purposes. 

For production OpenShift Dedicated deployments, the `UpgradeConfig` is distributed via a [Hive SelectorSyncSet](https://github.com/openshift/hive/blob/master/docs/syncset.md) and managed by OpenShift SRE.

The process for SRE to manage the creation and distribution of `UpgradeConfig` custom resources is documented in [SOPs](https://github.com/openshift/ops-sop/blob/master/v4/howto/upgrade.md). 
 
## Custom Resource Definitions

### UpgradeConfig

#### Configuration 

The `UpgradeConfig` Custom Resource Definition (CRD) defines the version of OpenShift Container Platform that the cluster should be upgraded to, when conditions allow.

For the purpose of upgrading a cluster, an `UpgradeConfig` resource _must_ be configured with the following properties:

| Item | Definition | Example |
| ---- | ---------- | ------- |
| `version` | The desired OCP release to upgrade to | `4.4.6` |
| `channel` | The [channel](https://github.com/openshift/cincinnati/blob/master/docs/design/openshift.md#Channels) the Cluster Version Operator should be using to validate update versions | `fast-4.4` |
| `force` | Whether to force an update | `false` |

A populated `UpgradeConfig` example is presented below:

```yaml
apiVersion: upgrade.managed.openshift.io/v1alpha1
kind: UpgradeConfig
metadata:
  name: example-upgrade-config
spec:
  desired:
    channel: "fast-4.4"
    force: false
    version: "4.4.6"
```

The CRD is available to [view in the repository](../deploy/crds/upgrade.managed.openshift.io_upgradeconfigs_crd.yaml). 

#### Status

The Managed Upgrade Operator will record the history of its efforts to apply the desired upgrade within the `UpgradeConfig`'s `status` section. Data within this section can be used to determine the operator's progress to apply the upgrade.

At the top level, the following fields are defined in a list, each list element representing a unique cluster version:

| Item | Definition | Example |
| ---- | ---------- | ------- |
| `version` | The cluster version that the operator events related to | `4.4.6` |
| `startTime` | The ISO-8601 timestamp at which the upgrade commenced. | `2020-07-05T01:35:36Z` |
| `completeTime` | The ISO-8601 timestamp at which the upgrade completed. | `2020-07-05T01:35:36Z` |
| `phase` | The current phase of the upgrade's application | `New`, `Pending`, `Upgrading`, `Upgraded`, `Failed`, `Unknown` |
| `conditions` | Data pertaining to a particular upgrade step that the operator performs | - |
 
Within `conditions`, each upgrade step can record its own individual status. These conditions are similar to [Pod conditions](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/), but relate to upgrade steps.

| Item | Definition | Example |
| ---- | ---------- | ------- |
| `type` | The type of upgrade step being performed | `PreHealthCheck` |
| `startTime` | The ISO-8601 timestamp at which the step commenced. | `2020-07-05T01:35:36Z` |
| `completeTime` | The ISO-8601 timestamp at which the step completed. | `2020-07-05T01:35:36Z` |
| `lastProbeTime` | The last time this step's condition was last probed | `2020-07-05T01:35:36Z` |
| `lastTransitionTime` | The last time this step transitioend from one status to another | `2020-07-05T01:35:36Z` |
| `message` | Human-readable details indicating details about the transition | `PreHealthCheck succeed` |
| `reason` | Human-readable details about why the transition has occurred | `Cluster has critical alerts` |
| `status` | Status of the condition | `True`, `False`, `Unknown` |

A fully-populated example of an `UpgradeConfig` status is included below:

```yaml
  status:
    history:
    - phase: Upgraded
      version: 4.3.26              
      startTime: "2020-07-05T01:35:36Z"      
      completeTime: "2020-07-05T03:15:37Z"                                                     
      conditions:            
      - completeTime: "2020-07-05T03:15:36Z"
        lastProbeTime: "2020-07-05T03:15:36Z"
        lastTransitionTime: "2020-07-05T03:15:36Z"                                             
        message: ScaleUpExtraNodes succeed                                                     
        reason: ScaleUpExtraNodes succeed  
        startTime: "2020-07-05T03:15:36Z" 
        status: "True"                   
        type: ScaleUpExtraNodes
      - completeTime: "2020-07-05T03:15:36Z"
        lastProbeTime: "2020-07-05T03:15:36Z"
        lastTransitionTime: "2020-07-05T03:15:36Z"                                             
        message: PreHealthCheck succeed                                                        
        reason: PreHealthCheck succeed                                                         
        startTime: "2020-07-05T03:15:36Z"                                                      
        status: "True"                   
        type: PreHealthCheck
```

## Upgrade Process

### Cluster Upgrader

The steps performed by the Managed Upgrade Operator are carried out by implementations of the [ClusterUpgrader](../pkg/cluster_upgrader/cluster_upgrader.go) interface.

Each `ClusterUpgrader` implementation must define an ordered series of `UpgradeSteps`, which represents the runbook of the implementation when conducting a cluster upgrade.

`UpgradeStep`s are homogeneous functions of code that carry out a part of the upgrade process. If the step has completed, it will return `true`. If the step has not completed, it will return `false`. If the step has failed, it will return an error.

When actively performing a cluster upgrade, the operator will follow the process below during each iteration of the controller reconcile loop:
- Get the first step in the ordered list.
- Check if the `UpgradeConfig`'s status history indicates the step has already completed.
  - If the step has already completed, move to the step.
- If the step has not already completed, execute the step.
  - If the step returns `true` indicating it has successfully completed, move to the next step.
  - If the step returns `false` indicating it has not successfully completed, the operator will check again on the next reconcile loop.
  - If the step returns an error, the operator will log this, and try to execute the step again on the next reconcile loop.
   
Steps should generally be idempotent in nature; if they have already run and completed during an upgrade, they should return `true` for subsequent calls and not attempt to re-perform the same action. An example of this is the `ControlPlaneMaintWindow` step to create a maintenance window.

This overall process of executing Upgrade Steps is illustrated below.

![Managed Upgrade Operator](images/upgradecluster-flow.svg)

To define a new custom procedure for performing a cluster upgrade, a developer should:
- Create a new implementation of the `ClusterUpgrader` that defines a unique order of `UpgradeStep`s.
- Implement any missing or new `UpgradeStep`s that need to be performed.  
