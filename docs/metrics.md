# managed-upgrade-operator Metrics (MUO)

The Managed Upgrade Operator reports the following metrics via direct instrumention:

## Metrics use to alert SRE

- `upgradeoperator_upgradeconfig_validation_failed`: If failed to validate the upgrade config `value > 0`
- `upgradeoperator_cluster_check_failed`: If failed on the cluster check step `value > 0`
- `upgradeoperator_scaling_failed`: If failed to scale up extra workers `value > 0`
- `upgradeoperator_controlplane_timeout`: If ontrol plane upgrade timeout `value > 0`
- `upgradeoperator_worker_timeout`: If worker nodes upgrade timeout `value > 0`
- `upgradeoperator_node_drain_timeout`: If node cannot be drained successfully in time `value > 0`
- `upgradeoperator_upgradeconfig_synced`: If upgradeConfig has not been synced in time `value > 0`

## Metrics for upgrade conditions and state

There is now a Collector that creates metrics strictly based on the UpgradeConfig CR. 

```
# HELP managed_upgrade_condition_capacity_added_timestamp Unix Timestamp indicating time of additional compute added
# TYPE managed_upgrade_condition_capacity_added_timestamp gauge
managed_upgrade_condition_capacity_added_timestamp

# HELP managed_upgrade_condition_capacity_removed_timestamp Unix Timestamp indicating time of addtional compute removed
# TYPE managed_upgrade_condition_capacity_removed_timestamp gauge
managed_upgrade_condition_capacity_removed_timestamp

# HELP managed_upgrade_condition_control_plane_completion_timestamp Unix Timestamp indicating completion of upgrade upgrade
# TYPE managed_upgrade_condition_control_plane_completion_timestamp gauge
managed_upgrade_condition_control_plane_completion_timestamp

# HELP managed_upgrade_condition_control_plane_maint_start_timestamp Unix Timestamp indicating start time of control plane maintenance
# TYPE managed_upgrade_condition_control_plane_maint_start_timestamp gauge
managed_upgrade_condition_control_plane_maint_start_timestamp

# HELP managed_upgrade_condition_control_plane_removed_start_timestamp Unix Timestamp indicating removal of control plane maintenance window
# TYPE managed_upgrade_condition_control_plane_removed_start_timestamp gauge
managed_upgrade_condition_control_plane_maint_removed_timestamp

# HELP managed_upgrade_condition_control_plane_upgrade_start_timestamp Unix Timestamp indicating start time of upgrade
# TYPE managed_upgrade_condition_control_plane_upgrade_start_timestamp gauge
managed_upgrade_condition_control_plane_upgrade_start_timestamp

# HELP managed_upgrade_condition_control_plane_upgrade_completion_timestamp Unix Timestamp indicating completion of upgrade
# TYPE managed_upgrade_condition_control_plane_upgrade_completion_timestamp gauge
managed_upgrade_condition_control_plane_upgrade_completion_timestamp

# HELP managed_upgrade_condition_external_dep_check_timestamp Unix Timestamp indicating time of cluster health check
# TYPE managed_upgrade_condition_external_dep_check_timestamp gauge
managed_upgrade_condition_external_dep_check_timestamp

# HELP managed_upgrade_condition_health_check_timestamp Unix Timestamp indicating time of cluster health check
# TYPE managed_upgrade_condition_health_check_timestamp gauge
managed_upgrade_condition_health_check_timestamp

# HELP managed_upgrade_condition_notification_start_timestamp Unix Timestamp indicating time of start upgrade notification event
# TYPE managed_upgrade_condition_notification_start_timestamp gauge
managed_upgrade_condition_notification_start_timestamp

# HELP managed_upgrade_condition_notification_complete_timestamp Unix Timestamp indicating time of complete upgrade notification event
# TYPE managed_upgrade_condition_notification_complete_timestamp gauge
managed_upgrade_condition_notification_complete_timestamp

# HELP managed_upgrade_upgrade_pdb_timeout_minutes Int indicating when the value of PDB timeout in minutes
# TYPE managed_upgrade_upgrade_pdb_timeout_minutes gauge
managed_upgrade_upgrade_pdb_timeout_minutes

# HELP managed_upgrade_upgrade_scheduled Unix Timestamp indicating when the upgrade will execute
# TYPE managed_upgrade_upgrade_scheduled gauge
managed_upgrade_upgrade_scheduled

# HELP managed_upgrade_upgrade_start_timestamp Timestamp of when an upgrade starts
# TYPE managed_upgrade_upgrade_start_timestamp gauge
managed_upgrade_upgrade_start_timestamp

# HELP managed_upgrade_upgrade_complete_timestamp Timestamp of when an upgrade completes entirely
# TYPE managed_upgrade_upgrade_complete_timestamp gauge
managed_upgrade_upgrade_complete_timestamp

# HELP managed_upgrade_condition_workers_maint_start_timestamp Unix Timestamp indicating end of workers maintenace
# TYPE managed_upgrade_condition_workers_maint_start_timestamp gauge
managed_upgrade_condition_workers_maint_start_timestamp

# HELP managed_upgrade_condition_workers_maint_removed_timestamp Unix Timestamp indicating end of workers maintenace
# TYPE managed_upgrade_condition_workers_maint_removed_timestamp gauge
managed_upgrade_condition_workers_maint_removed_timestamp

# HELP managed_upgrade_condition_workers_upgraded_timestamp Unix Timestamp indicating all worker nodes have upgraded
# TYPE managed_upgrade_condition_workers_upgraded_timestamp gauge
managed_upgrade_condition_workers_upgraded_timestamp

# HELP managed_upgrade_condition_post_upgrade_healthcheck_timestamp Unix Timestamp indicating time of post cluster health check
# TYPE managed_upgrade_condition_post_upgrade_healthcheck_timestamp gauge
managed_upgrade_condition_post_upgrade_healthcheck_timestamp

```