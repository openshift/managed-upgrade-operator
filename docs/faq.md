# FAQ

**What does MUO stand for?**

managed-upgrade-operator

**Does MUO silence alerts?**

Yes. All non-critical alerts are silenced during an upgrade. Specific "noisy" critical alerts are also silenced and can be found [here](https://github.com/openshift/managed-cluster-config/blob/master/deploy/managed-upgrade-operator-config/10-managed-upgrade-operator-configmap.yaml#L12-L20).

**How does MUO determine which alerts to silence?**	

Currently this is a manual process. We are working on dashboards and other metrics to help this become a data driven decision.

**Does MUO reserve compute capacity?**

Yes. MUO will create a temporary +1 to every worker `machinesets` within the cluster. In multi availability zones, this is true for each zone.

**Does MUO maintain correct instance types for each machine pool?**

Yes. MUO creates the extra compute based on the found instance types of the current `machinesets`.

**How does MUO handle [PodDisruptionBudgets](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/#pod-disruption-budgets) that block node draining?**

There is a configurable duration that sets how long MUO should respect a PodDisruptionBudget. Upon reaching this duration MUO will forcefully delete the detected pod. The current default configuration (in minutes) can be seen [here](https://github.com/openshift/managed-upgrade-operator/blob/master/deploy/crds/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml#L8).

**What happens to a node that is failing to drain NOT due a PodDisruptionBudget?**

MUO will forcefully drain these nodes at [this](https://github.com/openshift/managed-cluster-config/blob/master/deploy/managed-upgrade-operator-config/10-managed-upgrade-operator-configmap.yaml#L26) duration that is set by RedHat SRE.

**How to know the usage details for specifying fields in `UpgradeConfig` custom resource?**

The `oc explain` CLI command can be used to easily understand what each field is meant to do.

Example: To understand the API specification `(.spec)` for `UpgradeConfig`, the following command can be used:

```
$ oc explain upgradeconfig.spec
KIND:     UpgradeConfig
VERSION:  upgrade.managed.openshift.io/v1alpha1

RESOURCE: spec <Object>

DESCRIPTION:
     UpgradeConfigSpec defines the desired state of UpgradeConfig and upgrade
     window and freeze window

FIELDS:
   PDBForceDrainTimeout	<integer> -required-
     The maximum grace period granted to a node whose drain is blocked by a Pod
     Disruption Budget, before that drain is forced. Measured in minutes.

   desired	<Object> -required-
     Specify the desired OpenShift release

   subscriptionUpdates	<[]Object>
     This defines the 3rd party operator subscriptions upgrade

   type	<string> -required-
     Type indicates the ClusterUpgrader implementation to use to perform an
     upgrade of the cluster

   upgradeAt	<string> -required-
     Specify the upgrade start time
```

Likewise, the command can be extended to navigate across any of the fields for the custom resource.
