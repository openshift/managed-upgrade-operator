# Managed Upgrade Operator

> :warning: **Under Development**

[![Go Report Card](https://goreportcard.com/badge/github.com/openshift/managed-upgrade-operator)](https://goreportcard.com/report/github.com/openshift/managed-upgrade-operator)
[![codecov](https://codecov.io/gh/openshift/managed-upgrade-operator/branch/master/graph/badge.svg)](https://codecov.io/gh/openshift/managed-upgrade-operator)
[![GoDoc](https://godoc.org/github.com/openshift/managed-upgrade-operator?status.svg)](https://pkg.go.dev/mod/github.com/openshift/managed-upgrade-operator)
[![License](https://img.shields.io/:license-apache-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0.html)

----

The _Managed Upgrade Operator_ has been created for the [OpenShift Dedicated Platform](https://docs.openshift.com/dedicated/4/) (OSD) to manage the orchestration of automated in-place cluster upgrades.

Whilst the operator's job is to invoke a cluster upgrade, it does not perform any activities of the cluster upgrade process itself. This remains the responsibility of the OpenShift Container Platform. The operator's goal is to satisfy the operating conditions that a managed cluster must hold, both pre- and post-invocation of the cluster upgrade. 

Examples of activities that are not core to an OpenShift upgrade process but could be handled by the operator include:
 
* Pre and post-upgrade health checks.
* Worker capacity scaling during the upgrade period.
* Alerting silence window management.

If you like to contribute to the Managed Upgrade Operator, please read our [Contribution Policy](./docs/contributing.md) first.

----

* [Info](#info)
   * [Documentation](#documentation)
      * [For Developers](#for-developers)
   * [Workflow - UpgradeConfig](#workflow---upgradeconfig)
      * [Example CR](#example-input-custom-resource)
      
# Info

## Documentation

### For Developers

* [Design](./docs/design.md) -- Describes the interaction between the operator and the custom resource definition.
* [Development](./docs/development.md) -- Instructions for developing and deploying the operator.
* [Metrics](./docs/metrics.md) -- Prometheus metrics produced by the operator. 
* [Testing](./docs/testing.md) -- Instructions for writing tests.

## Workflow - UpgradeConfig

1. The operator watches all namespaces for an `UpgradeConfig` resource.
2. When an `UpgradeConfig` is found or modified, the operator checks the Status History to determine if this upgrade has been applied to the cluster.
     * If the `UpgradeConfig` history indicates that the cluster has been successfully upgraded to the defined version, no further action is taken.
3. If there is no previous history for this `UpgradeConfig`, or if it indicates that the upgrade is New, Pending or Ongoing, the operator creates a [ClusterUpgrader](pkg/cluster_upgrader/cluster_upgrader.go) to either initiate a new upgrade or or maintain an ongoing upgrade.             
4. The ClusterUpgrader runs through an ordered series of upgrade steps, executing them or waiting for them to complete. 
     * As steps are launched or complete, they are added to the `UpgradeConfig`'s Status History. 
5. Once all steps have been completed, the upgrade is considered complete and a Status History entry is written to indicate that the `UpgradeConfig` has been applied.

### Example Input Custom Resource

```yaml
apiVersion: upgrade.managed.openshift.io/v1alpha1
kind: UpgradeConfig
metadata:
  name: example-upgrade-config
spec:
  type: "OSD"
  upgradeAt: "2020-01-01T00:00:00Z"
  proceed: true
  PDBForceDrainTimeout: 120
  desired:
    channel: "fast-4.4"
    force: false
    version: "4.4.6"
```
