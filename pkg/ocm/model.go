package ocm

// All custom types have been migrated to use SDK types from ocm-sdk-go:
// - UpgradePolicyList -> Use *cmv1.UpgradePoliciesListResponse
// - UpgradePolicy -> Use *cmv1.UpgradePolicy
// - ClusterList -> Use *cmv1.ClustersListResponse
// - ClusterInfo -> Use *cmv1.Cluster
// - NodeDrainGracePeriod -> Use cmv1.Value (accessed via cluster.NodeDrainGracePeriod())
// - ClusterVersion -> Use cmv1.Version (accessed via cluster.Version())
// - UpgradePolicyState -> Use *cmv1.UpgradePolicyState
// - UpgradePolicyStateRequest -> Use cmv1.NewUpgradePolicyState() builder
