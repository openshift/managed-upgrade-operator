# FAQ

**What does MUO stand for?**

managed-upgrade-operator

**Does MUO silence alerts?**

Yes. All non-critical alerts are silenced during an upgrade. Specific "noisy" critical alerts are also silenced and can be found [here](https://github.com/openshift/managed-cluster-config/blob/ba87886f4d096f5760e907878afe127860318792/deploy/managed-upgrade-operator-config/10-managed-upgrade-operator-configmap.yaml#L12-L20).

**How does MUO determine which alerts to silence?**	

Currently this is a manual process. We are working on dashboards and other metrics to help this become a data driven decision.

**Does MUO reserve compute capacity?**

Yes. MUO will create a temporary +1 to every worker and infra `machinesets` within the cluster.

**Does MUO maintain correct instance types for each machine pool?**

Yes. MUO creates the extra compute based on the found instance types of the current `machinesets`.

**How does MUO handle [PodDisruptionBudgets](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/#pod-disruption-budgets) that block node draining?**

There is a configurable duration that sets how long MUO should respect a PodDisruptionBudget. Upon reaching this duration MUO will forcefully drain the effected node. The current default configuratoin can be seen [here](https://github.com/openshift/managed-cluster-config/blob/ba87886f4d096f5760e907878afe127860318792/deploy/managed-upgrade-operator-config/10-managed-upgrade-operator-configmap.yaml#L26).

**What happens to a node that is failing to drain NOT due a PodDisruptionBudget?**

MUO will forcefully drain these nodes at [this](https://github.com/openshift/managed-cluster-config/blob/master/deploy/managed-upgrade-operator-config/10-managed-upgrade-operator-configmap.yaml#L26) duration that is set by RedHat SRE.
