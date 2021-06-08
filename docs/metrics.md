# managed-upgrade-operator Metrics (MUO)

The Managed Upgrade Operator reports the following metrics:

## Metrics about managed upgrades

- `upgrade_at`: The desired start time of the upgrade
- `upgrade_start`: The actual start time of the managed upgrade
- `upgrade_control_plane_start`: The start time of control plane upgrades
- `upgrade_control_plane_completion`: The completion time of control plane upgrades
- `upgrade_worker_start`: The start time of the worker upgrades
- `upgrade_worker_completion`: The completion time of the worker upgrades
- `upgrade_complete`: The completion time of the managed upgrade

## Metrics use to alert SRE

- `upgradeoperator_upgradeconfig_validation_failed`: If failed to validate the upgrade config `value > 0`
- `upgradeoperator_cluster_check_failed`: If failed on the cluster check step `value > 0`
- `upgradeoperator_scaling_failed`: If failed to scale up extra workers `value > 0`
- `upgradeoperator_controlplane_timeout`: If ontrol plane upgrade timeout `value > 0`
- `upgradeoperator_worker_timeout`: If worker nodes upgrade timeout `value > 0`
- `upgradeoperator_node_drain_timeout`: If node cannot be drained successfully in time `value > 0`
- `upgradeoperator_upgradeconfig_synced`: If upgradeConfig has not been synced in time `value > 0`