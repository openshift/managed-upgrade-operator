# Development

- [Development](#development)
  - [Development Environment Setup](#development-environment-setup)
    - [golang](#golang)
    - [operator-sdk](#operator-sdk)
  - [Dependencies](#dependencies)
  - [Linting](#linting)
  - [Testing](#testing)
  - [Building](#building)
  - [Build using boilerplate container](#build-using-boilerplate-container)
  - [How to run](#how-to-run)
    - [Locally](#locally)
    - [Remotely](#remotely)
  - [Monitoring ongoing upgrade](#monitoring-ongoing-upgrade)

This document should entail all you need to develop this operator locally.

## Development Environment Setup

### golang

A recent Go distribution (>=1.13) with enabled Go modules.

```shell
$ go version
go version go1.16.3 linux/amd64
```

### operator-sdk

The Operator is being developed based on the [Operators SDK](https://github.com/operator-framework/operator-sdk).
Ensure this is installed and available in your `$PATH`.

[v0.18.2](https://github.com/operator-framework/operator-sdk/releases/tag/v0.18.2) is being used for `managed-upgrade-operator` development.

```shell
$ operator-sdk version
operator-sdk version: "v0.18.2", commit: "f059b5e17447b0bbcef50846859519340c17ffad", kubernetes version: "v1.18.2", go version: "go1.13.10 linux/amd64"
```

## Dependencies

The tool dependencies that are required locally to be present are all part of [tools.go](https://github.com/openshift/managed-upgrade-operator/blob/master/tools.go) file. This file will refer the version of the required module from [go.mod](https://github.com/openshift/managed-upgrade-operator/blob/master/go.mod) file.

In order to install the tool dependencies locally, run `make tools` at the root of the cloned repository, which will fetch the tools for you and install the binaries at location `$GOPATH/bin` by default:

```shell
$ make tools
```

This will make sure that the installed binaries are always as per the required version mentioned in `go.mod` file. If the version of the module is changed, need to run the command again locally to have new version of tools.

---

**NOTE**

If any of the dependencies are failing to install due to checksum mismatch, try setting `GOPROXY` env variable using `export GOPROXY="https://proxy.golang.org"` and run `make tools` again

---

## Linting

To run lint locally, call `make lint`

```shell
$ make lint
```

## Testing

To run unit tests locally, call `make test`

```shell
$ make test
```

## Building

To run go build locally, call `make go-build`

```shell
$ make go-build
```

## Build using boilerplate container

To run lint, test and build in `app-sre/boilerplate` container, call `boilerplate/_lib/container-make`. This will call `make` inside the `app-sre/boilerplate` container.

```shell
$ boilerplate/_lib/container-make
```

## How to run

Regardless of how you choose to run the operator, before doing so ensure the `UpgradeConfig` CRD is present on your cluster:

```shell
$ oc create -f deploy/crds/upgrade.managed.openshift.io_upgradeconfigs_crd.yaml
```

### Locally

- Make sure you have the [operator-sdk](#operator-sdk) `$PATH`.

- If you are not using an account that has `cluster-admin` privileges, you will need to [elevate permissions](https://github.com/openshift/ops-sop/blob/master/v4/knowledge_base/manage-privileges.md) to possess them.

- Create a project for the operator to run inside of:

```
$ oc new-project test-managed-upgrade-operator
```

- Run the operator via the Operator SDK:

```
$ OPERATOR_NAMESPACE=test-managed-upgrade-operator operator-sdk run --local --watch-namespace=""
```

(`make run` will also work here)

- Trigger a reconcile loop by applying an `upgradeconfig` CR with your desired specs:

```shell
$ oc apply -f test/deploy/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml
```

### Remotely

- Build the image. In this example, we will use [Quay](http://quay.io/) as the container registry for our image:

```shell
$ operator-sdk build quay.io/<QUAY_USERNAME>/managed-upgrade-operator:latest
```

- Setup [quay](./quay.md) registry and push the image:

```shell
podman push quay.io/<QUAY_USERNAME>/managed-upgrade-operator:latest
```

- Login to `oc` [as admin](https://github.com/openshift/ops-sop/blob/master/v4/howto/break-glass-kubeadmin.md#for-clusters-with-public-api) 
  
- Ensure no other instances of managed-upgrade-operator are actively running on your cluster, as they may conflict. If MUO is already deployed on the cluster scale the deployment down to 0:

```shell
oc scale deployment managed-upgrade-operator -n <EXISTING_MUO_NAMESPACE> --replicas=0
```

- Create a project for the operator to run inside of:

```shell
$ oc new-project test-managed-upgrade-operator
```

- Deploy the service account, clusterrole, clusterrolebinding and ConfigMap on your target cluster:

```shell
oc create -f deploy/cluster_role.yaml
oc create -f test/deploy/cluster_role_binding.yaml
oc create -f test/deploy/service_account.yaml
oc create -f test/deploy/managed-upgrade-operator-config.yaml
```

- Set `test/deploy/operator.yaml` to use `quay.io/<QUAY_USERNAME>/managed-upgrade-operator:latest` container image and create deployment configuration

```shell
oc create -f test/deploy/operator.yaml
```

- Trigger a reconcile loop by applying an [upgradeconfig](../test/deploy/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml) CR with your desired specs:

```shell
oc create -f - <<EOF
apiVersion: upgrade.managed.openshift.io/v1alpha1
kind: UpgradeConfig
metadata:
  name: managed-upgrade-config
spec:
  type: "OSD"
  upgradeAt: "2020-01-01T00:00:00Z"
  PDBForceDrainTimeout: 60
  desired:
    channel: "fast-4.7"
    version: "4.7.18"
EOF
```

---

**NOTE**

Refer to [fast-4.7](https://access.redhat.com/labs/ocpupgradegraph/update_channel?channel=fast-4.7&arch=x86_64&is_show_hot_fix=false) for currently available versions in `fast-4.7` channel.

---

## Monitoring ongoing upgrade

- After applying an `upgradeConfig`, you should see your upgrade progressing

```shell
oc get upgrade -n test-managed-upgrade-operator
``` 

- Inspect `upgradeConfig`:

```shell
oc describe upgrade -n test-managed-upgrade-operator managed-upgrade-config 
```

- It can be useful to monitor the events in `test-managed-upgrade-operator` namespace during the upgrade:

```shell
oc get event -n test-managed-upgrade-operator -w
```

- To see messages from MUO, inspect `MUO container` logs in `test-managed-upgrade-operator` namespace:

```shell
oc logs <MUO POD> -n test-managed-upgrade-operator -f
```

- To follow upgrade status, get `clusterversion`:

```shell
oc get clusterversion -w
```

- To follow upgrade messages, inspect `cluster-version-operator` pod logs in `openshift-cluster-version namespace`:
```shell
oc logs cluster-version-operator-<POD-ID> -n  openshift-cluster-version -f
```
