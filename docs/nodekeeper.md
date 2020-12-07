# Nodekeeper Controller

## About
The `managed-upgrade-operator` provides a mechanism for keeping track of upgrading worker nodes and seeks to ensure their timely and eventual upgrade.

The `Nodekeeper` controller makes sure that if an upgrading worker node is facing difficulty draining due to conditions such as [Pod Disruption Budgets](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/#pod-disruption-budgets) or stuck finalizers, the `Nodekeeper` controller will perform strategies for draining the nodes properly and ensuring subsequent upgrade continuation.
The `Nodekeeper` controller will set the `upgradeoperator_node_drain_timeout` metric in Prometheus in the event that any worker node continues to unsuccessfully drain in spite of `NodeDrain` strategies.

## Inside the Nodekeeper Controller
- The `Nodekeeper` controller mechanism starts with setting the controller with `Add()` function which add sets up the controller for the operator manager - setting up a watch on the resource. For the `Nodekeeper` controller the mechanism is setting up a watch on the resource of `Node`.

- Now as already specified, the `Nodekeeper` controller works only towards the worker nodes, so for excluding the master nodes an [IgnoreMasterPredicate](https://github.com/openshift/managed-upgrade-operator/blob/master/pkg/controller/nodekeeper/ignoremaster_predicate.go) is used, which makes sure that the controller only targets worker nodes in it's mechanism.
The `IgnoreMasterPredicate` works on the basis of cache, so it considers all the nodes at first run and re-reconciles  at the next run and starts ignoring the Master nodes.

- The `Reconcile()` function is the main and most important part of the controller, it starts with creating `UpgradeConfigManager` to check if we are in an upgrading stage by specifically checking the `MachineConfig` through `IsUpgrading()` and also checks the history using `GetHistory()` based on the `UpgradeConfig` and get all the nodes from the `Kube-Client`. If the cluster is not detected as currently upgrading, the reconciler does not proceed further.

- For each reconciled node, the controller checks if it is cordoned using `IsNodeCordoned()`, which checks for `Unschedulable` and `Tainted` nodes (specifically for nodes with the `TaintEffectNoSchedule` taint). If the node is found to be cordoned, the controller performs a series of [drain strategies](##drain-strategies)  for the node and - if those strategies have failed to fix the node within a timeout period - sets the `upgradeoperator_node_drain_timeout` gauge metric. If however, the node is no longer cordoned, the reconciler assumes the drain and subsequent upgrade has succeeded, and so resets the metric.

## Drain strategies

The `NodeDrainStrategy` consists of:
- a set of predicates which define the conditions that a pod must be in in order to be considered for a node drain strategy; and
- a set of timed drain strategies, which perform the steps to address the detected conditions. The `timed` nature of the strategy means that the strategy is only initiated after a set period of time (measured from when the node was first detected as cordoned) has elapsed.

Following is the list of predicates used in this mechanism :
- `defaultOsdPodPrediate` : Used for any pod but not a `DaemonSet`.
- `isNotPdbPod` : If there's not a Pod Disruption Budget associated with the concerned pod.
- `isPdbPod` : If there's a Pod Disruption Budget associated with the concerned pod.

### Pod Disruption Budgets (PDBs)
This strategy handles workloads which are disrupting a node drain due to [Pod Disruption Budgets](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/#pod-disruption-budgets), which would be violated if the pod were to be evicted.

Pods which are protected by Pod Disruption Budgets are respected until the `pdbNodeDrainTimeout` period of the `UpgradeConfig` has elapsed. At that point, if a pod is still not draining due to the presence of a PDB, the pods will be forcefully deleted in order to progress the worker node drain.

### Finalizers 
This strategy handles workloads which are disrupting a node drain due to a finalizer which may be preventing the pod from deleting. Pods are given until `NodeDrain.Timeout` to drain from the node before this strategy is considered. At that point, if a pod is still running on the node due to the presence of a finalizer, the finalizers will be removed from the Pod spec.

### Stuck pods
This strategy handles workloads which are disrupting a node drain for any reason. Pods are given until `NodeDrain.Timeout` to drain from the node before this strategy is considered. At that point, if a pod is still running on the node, it is forcefully deleted.