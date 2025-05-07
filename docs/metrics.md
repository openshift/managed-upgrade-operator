# managed-upgrade-operator Metrics (MUO)

The Managed Upgrade Operator reports the following metrics via direct instrumention:

## Metrics use to alert SRE

- `upgradeoperator_upgradeconfig_validation_failed`: If failed to validate the upgrade config `value > 0`
- `upgradeoperator_healthcheck_failed`: If failed on the cluster health check step `value > 0`
- `upgradeoperator_scaling_failed`: If failed to scale up extra workers `value > 0`
- `upgradeoperator_controlplane_timeout`: If control plane upgrade timeout `value > 0`
- `upgradeoperator_worker_timeout`: If worker nodes upgrade timeout `value > 0`
- `upgradeoperator_node_drain_timeout`: If node cannot be drained successfully in time `value > 0`
- `upgradeoperator_upgradeconfig_sync_timestamp`: Set a timestamp as the value of the metric if the upgradeconfig sync succeeded
- `upgradeoperator_upgrade_started_timestamp`: Set a timestamp as the value of the metric when the upgrade commenced
- `upgradeoperator_upgrade_completed_timestamp`: Set a timestamp as the value of the metric when the upgrade finished
- `upgradeoperator_controlplane_upgrade_started_timestamp`: Set a timestamp as the value of the metric when the controlplane upgrade commenced
- `upgradeoperator_controlplane_upgrade_completed_timestamp`: Set a timestamp as the value of the metric when the the controlplane upgrade finished
- `upgradeoperator_workernode_upgrade_started_timestamp`: Set a timestamp as the value of the metric when the workerpool upgrade commenced
- `upgradeoperator_workernode_upgrade_completed_timestamp`: Set a timestamp as the value of the metric when the workerpool upgrade finished

## Metrics for fleet-wide monitoring

The following metrics are forwarded to Observatorium-MST for fleetwide monitoring via grafana

- `upgradeoperator_upgrade_result`: Contains results from the previous upgrade. If upgrade fired a paging alert `value == 0` and the `alerts` field contains the name of alerts fired
- `upgradeoperator_healthcheck_failed`: Contains preflight health check results from the previous upgrade. If upgrade preflight health check success `value == 0` otherwise `value == 1`. The `state` field contains in which upgrade state the PHC is performed. The `reason` field indicates which type of health check is performed.

The following metrics are forwarded to Observatorium-MST on the management clusters for fleetwide monitoring via telemetry
- `upgradeoperator_upgrade_started_timestamp`: Set a timestamp as the value of the metric when the upgrade commenced
- `upgradeoperator_upgrade_completed_timestamp`: Set a timestamp as the value of the metric when the upgrade finished
- `upgradeoperator_controlplane_upgrade_started_timestamp`: Set a timestamp as the value of the metric when the controlplane upgrade commenced
- `upgradeoperator_controlplane_upgrade_completed_timestamp`: Set a timestamp as the value of the metric when the the controlplane upgrade finished
- `upgradeoperator_workernode_upgrade_started_timestamp`: Set a timestamp as the value of the metric when the workerpool upgrade commenced
- `upgradeoperator_workernode_upgrade_completed_timestamp`: Set a timestamp as the value of the metric when the workerpool upgrade finished

### How to expose the metric to RHOBS for fleet-wide monitoring

The metrics defined in MUO only live inside the container. Each metric that needs to be sent to RHOBS via remoteWrite should be combined with the `sre:telemetry:managed_labels`. Details could be found in [doc](https://github.com/openshift/managed-cluster-config/blob/master/deploy/sre-prometheus/centralized-observability/README.md). Follow the steps below to set up the rule to expose metric to RHOBS.

- Make sure the metric has label could be used to combine to `sre:telemetry:managed_labels`
- Create/update a record rule in [centralized-observability](https://github.com/openshift/managed-cluster-config/blob/master/deploy/sre-prometheus/centralized-observability)
- Update the [cluster monitoring config](https://github.com/openshift/managed-cluster-config/blob/master/resources/cluster-monitoring-config/config.yaml) to include the new record rule
- Generate the new cluster monitoring config configMap via [script](https://github.com/openshift/managed-cluster-config/blob/master/scripts/generate-cmo-config.py) then update the [managed-cluster-config repo](https://github.com/openshift/managed-cluster-config) with the new record rule, new cluster monitoring config, and updated new configMaps.


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
