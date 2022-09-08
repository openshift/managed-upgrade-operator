# Config Map


- [Config Map](#config-map)
  - [About](#about)
  - [How to use it](#how-to-use-it)
    - [For OSD](#for-osd)
    - [For ARO](#for-aro)
  - [Configurable knobs](#configurable-knobs)
    - [configManager](#configmanager)
    - [maintenance](#maintenance)
    - [scale](#scale)
    - [upgradeWindow](#upgradewindow)
    - [nodeDrain](#nodedrain)
    - [healthCheck](#healthcheck)
    - [extDependencyAvailabilityChecks](#extdependencyavailabilitychecks)

## About
The `configmap` which used to tune the `managed-upgrade-operator`. It has various configurable values.

## How to use it

### For OSD
You can update the `configmap` via [`managed-cluster-config`](https://github.com/openshift/managed-cluster-config/tree/master/deploy/managed-upgrade-operator-config).

### For ARO
TBD


## Configurable knobs

#### upgradeType

This defines which upgrader MUO should use to upgrade the cluster.

Valid options are:
- [ARO](https://github.com/openshift/managed-upgrade-operator/blob/master/pkg/upgraders/aroupgrader.go)
- [OSD](https://github.com/openshift/managed-upgrade-operator/blob/master/pkg/upgraders/osdupgrader.go)

If this field is not present or is an empty value, the ARO upgrader is used by default.

#### configManager

Please refer to the doc [`configmanager`](./configmanager.md)

#### validation

| Key        | Description                                                             |
|------------|-------------------------------------------------------------------------|
| cincinnati | Use Cincinnati to validate upgrade hops during UpgradeConfig validation |

Example:
```
    validation:
      cincinnati: true
```

#### environment

| Key     | Description                                |
|---------|--------------------------------------------|
| fedramp | MUO is deployed into a Fedramp environment |

Example:
```
    environment:
      fedramp: true
```

#### maintenance

| Key | Description |
| --- | --- |
| controlPlaneTime | maintenance window created in alertmanager for controlplane upgrade in minutes including all the low and medium alerts, default is 90 |
| ignoredAlerts | a list of particular alerts to silence which are not covered by the default maintenance |
| controlPlaneCriticals | critical alerts might fire during controlplane upgrade |

Example:
```
    maintenance:
      controlPlaneTime: 90
      ignoredAlerts:
        controlPlaneCriticals:
        - ClusterOperatorDown
        - ClusterOperatorDegraded
```

#### scale

| Key | Description |
| --- | --- |
| timeOut | timeout window for the extra workload scale up in minutes, default is 30 |

Example:
```
    scale:
      timeOut: 30
```

#### upgradeWindow

| Key | Description |
| --- | --- |
| delayTrigger | a time window to indicate that the cluster upgrade is delayed against the schedule in minutes, default is 30 |
| timeOut | a time window which the upgrade process should have started before it is considered as "failed". Measured in minutes, default is 120 |

Example:
```
    upgradeWindow:
      delayTrigger: 30
      timeOut: 120
```

#### nodeDrain

| Key | Description |
| --- | --- |
| timeOut | a time window to trigger the force node drain strategy, measured in minutes |
| expectedNodeDrainTime | expected time in minutes for a single node drain to be finished, used to setup the maintenance window |

Example:
```
    nodeDrain:
      timeOut: 45
      expectedNodeDrainTime: 8
```

#### healthCheck

| Key | Description |
| --- | --- |
| ignoredCriticals | a list of critical alerts which need to be ignored in the health check to unblock the upgrade process |
| ignoredNamespaces | a list of namespaces which need to be ignored in the health check to unblock the upgrade process |

Example:
```
    healthCheck:
      ignoredCriticals:
      - DNSErrors05MinSRE
      - FluentdNodeDown
      ignoredNamespaces:
      - openshift-logging
      - openshift-redhat-marketplace
```

#### extDependencyAvailabilityChecks

| Key | Description |
| --- | --- |
| http | a list of external HTTP(s) dependencies to check before the upgrade can be triggered. Dependencies must return a 200 HTTP Status Code to be considered healthy. |

Example:
```
    extDependencyAvailabilityChecks:
      http:
        timeout: 10
        urls:
          - http://www.example.com
```