# UpgradeConfig Managers

## About

The `managed-upgrade-operator` provides a configurable mechanism for retrieving and storing an `UpgradeConfig` 
Custom Resource that can be reconciled against by the `UpgradeConfig` controller.

Currently, the following sources are supported:

| Source | Description |
| --- | --- |
| `OCM` | Retrieve an UpgradeConfig from the OpenShift Cluster Manager [`upgrade_policies`](https://api.openshift.com/#/default/get_api_clusters_mgmt_v1_clusters__cluster_id__upgrade_policies) API |
| `LOCAL` | Using UpgradeConfig CR locally on the OpenShift Cluster|

## Configuring an UpgradeConfig Manager

The UpgradeConfig Manager is configured in a `configManager` block in the `managed-upgrade-operator-config` ConfigMap.

If this block is not present, the operator will not create a manager at all, but will still run.

For source-specific configuration, see the correpsonding section below.

### OCM UpgradeConfig Manager

The following configuration fields must be set:

| Field | Description | Example |
| --- | --- | --- |
| `source` | Indicates the type of config manager being used | `OCM` |
| `ocmBaseUrl` | Base URL of the OpenShift Cluster Manager API | https://api.openshift.com/ |
| `watchInterval` | Frequency* in minutes with which the API will be polled | 60 |

The OCM UpgradeConfig Manager will intentionally apply a jitter factor of 10% to the watch interval, so the precise frequency may not always be the value specified.

Complete example:
```yaml
configManager:
  source: OCM
  ocmBaseUrl: https://api.openshift.com
  watchInterval: 60
```

### LOCAL UpgradeConfig Manager

The following configuration fields must be set:

| Field | Description | Example |
| --- | --- | --- |
| `source` | Indicates the type of config manager being used | `LOCAL` |
| `LocalConfigName` | Name of the Local config being used | `managed-upgrade-config` |
| `watchInterval` | Frequency* in minutes with which UpgradeConfig CR name being looked | 60 |