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
