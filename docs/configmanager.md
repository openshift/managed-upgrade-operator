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

The OCM UpgradeConfig Manager uses the official OpenShift Cluster Manager SDK (`ocm-sdk-go`) to communicate with the OCM API. The SDK provides typed interfaces for clusters, upgrade policies, and service logs.

The following configuration fields must be set:

| Field | Description | Example |
| --- | --- | --- |
| `source` | Indicates the type of config manager being used | `OCM` |
| `ocmBaseUrl` | Base URL of the OpenShift Cluster Manager API | https://api.openshift.com/ |
| `watchInterval` | Frequency* in minutes with which the API will be polled | 60 |

The OCM UpgradeConfig Manager will intentionally apply a jitter factor of 10% to the watch interval, so the precise frequency may not always be the value specified.

**OCM SDK Features:**
- **Typed API**: Uses SDK types (`cmv1.Cluster`, `cmv1.UpgradePolicy`, `cmv1.UpgradePolicyState`) instead of custom structs
- **Automatic Retry**: Configured with 5 retry attempts for 503, 429, and network errors
- **Proxy Support**: Automatically respects `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` environment variables
- **Enhanced Timeouts**: 30-second connection timeout and 10-second TLS handshake timeout for reliable communication

Complete example:
```yaml
configManager:
  source: OCM
  ocmBaseUrl: https://api.openshift.com
  watchInterval: 60
```

If we set the OCM base URL to the URL of the local OCM agent service (`http://ocm-agent.openshift-ocm-agent-operator.svc.cluster.local:8081`) we will activate the `ocmAgent` client handler, which will use the OCM agent endpoints set out in the [OCM Agent router](https://github.com/openshift/ocm-agent/blob/master/pkg/cli/serve/serve.go). The OCM Agent client does not use proxy configuration as it communicates with local cluster services only.

### LOCAL UpgradeConfig Manager

The following configuration fields must be set:

| Field | Description | Example |
| --- | --- | --- |
| `source` | Indicates the type of config manager being used | `LOCAL` |
| `localConfigName` | Name of the Local config being used | `managed-upgrade-config` |
| `watchInterval` | Frequency* in minutes with which UpgradeConfig CR name being looked | 60 |

Complete example:
```yaml
configManager:
  source: LOCAL
  localConfigName: managed-upgrade-config
  watchInterval: 60
```
