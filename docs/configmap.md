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

## Practical implementations of the ConfigMap

### Red Hat OSD/ROSA clusters

Maintained in the [managed-cluster-config](https://github.com/openshift/managed-cluster-config) repository:

- https://github.com/openshift/managed-cluster-config/tree/master/deploy/managed-upgrade-operator-config

### Red Hat ARO clusters

Maintained in the [ARO-RP](https://github.com/Azure/ARO-RP) repository:
- https://github.com/Azure/ARO-RP/blob/master/pkg/operator/controllers/muo/staticresources/config.yaml

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
| `cincinnati` | Use Cincinnati to validate upgrade hops during UpgradeConfig validation |

Example:
```
    validation:
      cincinnati: true
```

#### environment

| Key     | Description                                |
|---------|--------------------------------------------|
| `fedramp` | MUO is deployed into a Fedramp environment |

Example:
```
    environment:
      fedramp: true
```

#### maintenance

The `maintenance` section is used to control behaviour of the `managed-upgrade-operator` concerning the creation of AlertManager silences created during the upgrade process.

| Key                                 | Description                                                                                                                                      |
|-------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------|
| `controlPlaneTime`                    | maintenance window created in alertmanager for controlplane upgrade, including all the low and medium alerts. Measured in minutes, default is 90 |
| `ignoredAlerts.controlPlaneCriticals` | a list of particular critical alerts during the controlplane upgrade which are not covered by the default maintenance |

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

The `scale` section is used to control the `managed-upgrade-operator`'s behaviour when performing capacity node scaling.

| Key | Description                                                              |
| --- |--------------------------------------------------------------------------|
| `timeOut` | timeout window for the extra workload scale up in minutes, default is 30 |

Example:
```
    scale:
      timeOut: 30
```

#### upgradeWindow

The `upgradeWindow` section is used to control the `managed-upgrade-operator`'s behaviour in relation to the upgrade window within which an upgrade should take place.   

| Key | Description                                                                                                                                                                                               |
| --- |-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `delayTrigger` | The duration of time following the upgrade scheduled start time, after which MUO will set the upgrade state to "delayed" if the control plane upgrade has not started. Measured in minutes, default is 30 |
| `timeOut` | a time window which the upgrade process should have started before it is considered as "failed". Measured in minutes, default is 120                                                                      |

Example:
```
    upgradeWindow:
      delayTrigger: 30
      timeOut: 120
```

#### nodeDrain

| Key | Description                                                                                           |
| --- |-------------------------------------------------------------------------------------------------------|
| `timeOut` | a time window to trigger the force node drain strategy, measured in minutes                           |
| `expectedNodeDrainTime` | expected time in minutes for a single node drain to be finished, used to setup the maintenance window |
| `disableDrainStrategies` | disable any node drain completion strategies from executing (defaults to false)                       |
| `ignoredNamespacePatterns` | any pods in namespaces matching the regular expressions in this list are ignored from having drain strategies applied to them |

Example:
```
    nodeDrain:
      timeOut: 45
      expectedNodeDrainTime: 8
      disableDrainStrategies: true
      ignoredNamespacePatterns:
      - my-namespace-name
      - example-.+
```

#### healthCheck

The `healthCheck` section is used to control how the `managed-upgrade-operator` handles the pre and post-upgrade health checks.

| Key | Description |
| --- | --- |
| `ignoredCriticals` | a list of critical alerts which need to be ignored in the health check to unblock the upgrade process |
| `ignoredNamespaces` | a list of namespaces which need to be ignored in the health check to unblock the upgrade process |

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
| `http` | a list of external HTTP(s) dependencies to check before the upgrade can be triggered. Dependencies must return a 200 HTTP Status Code to be considered healthy. |

Example:
```
    extDependencyAvailabilityChecks:
      http:
        timeout: 10
        urls:
          - http://www.example.com
```

#### featureGate

| Key | Description |
| --- | --- |
| `enabled` | a list of feature gates to be enabled when managed-upgrade-operator starts |

Currently available featureGates:
- PreHealthCheck

Example:
```yaml
    featureGate:
      enabled:
      - PreHealthCheck
```