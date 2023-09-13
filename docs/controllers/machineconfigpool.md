# MachineConfigPool controller

## About

The MachineConfigPool controller is used to monitor the state of worker `machineconfigpool`s in order to track the time at which they commence and complete upgrading.

These time metrics are recorded into the `UpgradeConfig`'s status history and used by MUO's metrics collector to report in upgrade metrics. The following is an example of the UpgradeConfig status history:

```
status:
  history:
  - phase: Upgraded
    workerCompleteTime: "2021-08-17T01:13:35Z"
    workerStartTime: "2021-08-17T00:44:50Z"
```

## How it works

```mermaid
graph TD;

reconcile(Reconcile 'worker' MachineConfigPool)
loaduc(Load UpgradeConfig)
isuc{Is there an UpgradeConfig?}
isupgrading{Is the cluster upgrading?}
startmc{Is the MCP starting an update?}
finishmc{Has the MCP finished its update?}
recordstart(Record start time in UpgradeConfig status)
recordend(Record end time in UpgradeConfig status)
done(Done)


reconcile --> loaduc
loaduc --> isuc
isuc --> |yes| isupgrading
isuc --> |no| done
isupgrading --> |no| done
isupgrading --> |yes| startmc
startmc --> |yes| recordstart
startmc --> |no| finishmc
recordstart --> finishmc
finishmc --> |yes| recordend
finishmc --> |no| done
recordend --> done
```
