# FAQ

**What does MUO stand for?**

managed-upgrade-operator

**Does MUO silence alerts?**

Yes. All non-critical alerts are silenced during an upgrade. Specific "noisy" critical alerts are also silenced and can be found the [maintenance](./configmap.md#maintenance) section of the ConfigMap.

**How does MUO determine which alerts to silence?**	

Currently this is a manual process. We are working on dashboards and other metrics to help this become a data driven decision.

**Does MUO reserve compute capacity?**

Yes, if `capacityReservation` in the upgradeconfig CR is set to `true`. MUO creates a new `upgrade` worker machineset for each availability zone with a size of 1 worker node.

> **_NOTE:_** `spec.capacityReservation` is an optional field in the upgradeconfig CR. If this is not defined in the upgradeconfig CR the default value is set to true for OCM provider and false for LOCAL provider.

**Does MUO maintain correct instance types for each machine pool?**

Yes, if `capacityReservation` in the upgradeconfig CR is set to `true`. MUO creates the extra compute based on the found instance types of the current `machinesets`.

**How does MUO handle [PodDisruptionBudgets](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/#pod-disruption-budgets) that block node draining?**

There is a configurable duration that sets how long MUO should respect a PodDisruptionBudget. Upon reaching this duration MUO will forcefully delete the detected pod. This configuration is performed using the `spec.PDBForceDrainTimeout` field of the `UpgradeConfig` CR and is measured in minutes:

**What happens to a node that is failing to drain NOT due a PodDisruptionBudget?**

MUO will forcefully drain these nodes at [this](https://github.com/openshift/managed-cluster-config/blob/master/deploy/managed-upgrade-operator-config/10-managed-upgrade-operator-configmap.yaml#L26) duration that is set by RedHat SRE.