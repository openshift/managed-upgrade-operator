# Development

- [Development](#development)
  - [Development Environment Setup](#development-environment-setup)
    - [golang](#golang)
    - [operator-sdk](#operator-sdk)
  - [Makefile](#makefile)
  - [Dependencies](#dependencies)
  - [Linting](#linting)
  - [Testing](#testing)
  - [Building](#building)
  - [Build using boilerplate container](#build-using-boilerplate-container)
  - [How to run](#how-to-run)
    - [Locally](#locally)
    - [Run using internal cluster services](#run-using-internal-cluster-services)
    - [Run using cluster routes](#run-using-cluster-routes)
    - [Remotely](#remotely)
    - [Trigger Reconcile](#trigger-reconcile)
  - [Monitoring ongoing upgrade](#monitoring-ongoing-upgrade)

This document should entail all you need to develop this operator locally.

## Development Environment Setup

### golang

A recent Go distribution (>=1.17) with enabled Go modules.

```shell
$ go version
go version go1.18.4 linux/amd64
```

### operator-sdk

The Operator is being developed based on the [Operators SDK](https://github.com/operator-framework/operator-sdk).
Ensure this is installed and available in your `$PATH`.

[v1.21.0](https://github.com/operator-framework/operator-sdk/releases/tag/v1.21.0) is being used for `managed-upgrade-operator` development.

```shell
$ operator-sdk version
operator-sdk version: "v1.21.0", commit: "89d21a133750aee994476736fa9523656c793588", kubernetes version: "1.23", go version: "go1.17.10", GOOS: "linux", GOARCH: "amd64"
```

## Makefile

All available standardized commands for the `Makefile` are available via:

```shell
$ make
Usage: make <OPTIONS> ... <TARGETS>

Available targets are:

go-build                         Build binary
go-check                         Golang linting and other static analysis
go-test                          runs go test across operator
boilerplate-update               Make boilerplate update itself
help                             Show this help screen.
run                              Run operator locally against the configured Kubernetes cluster in ~/.kube/config
tools                            Install local go tools for MUO
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
$ make go-test
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

MUO by defaults uses in the internal services to contact prometheus and alertmanager. This enables the use of a firewall to prevent egress calls however increases local development complexity slightly. 

There are now three main modes that MUO can be ran in. 

1. Run in a container in cluster. 
2. Run locally using port-forwards and `/etc/hosts` entries to replicate production environment. 
3. Run locally using Routes to access services. This is not true production however is the most simple for local development. 

Modes 2 and 3 can be executed via the `Makefile` optionally setting the `$OPERATOR_NAMESPACE` as explored in the next section. 

```
run                              Wrapper around operator sdk run. Requires OPERATOR_NAMESPACE to be set. See run-standard for defaults.
run-standard                     Run locally with openshift-managed-upgrade-operator as OPERATOR_NAMESPACE.
run-routes                       Same as `run`, however will use ROUTE objects to contact prometheus and alertmanager. Use of routes is non-standard but convenient for local development. Requires OPERATOR_NAMESPACE to be set.
run-standard-routes              Run locally with openshift-managed-upgrade-operator as OPERATOR_NAMESPACE and use of non-standard routes.
```


### Locally

- Make sure you have the [operator-sdk](#operator-sdk) `$PATH`.

- You will need to be logged in with an account that meets the [RBAC requirements](https://github.com/openshift/managed-upgrade-operator/blob/master/deploy/cluster_role.yaml) for the MUO service account.

- OPTIONAL: Create a project for the operator to run inside of.

```
$ oc new-project test-managed-upgrade-operator
```

### Run using internal cluster services

The use of internal cluster services requires some changes locally to your environment:

1. Add entries for prometheus and alertmanager to `/etc/hosts` that resolve to `127.0.0.1`
2. Create port-forwards for each service

You can run this script to set this up for you (Requires `sudo` as it writes to `/etc/hosts`). The script accepts an optional `--context`. This enables setting up of the port-forwards with your personal user in the OpenShift cluster while running the operator locally using the available MUO service account for production like replication.

```
$ ./development/port-forwards -h
Setup port forwarding for local development using internal svc
usage ./development/port-forwards context (to execute oc commands as). This is useful if you are running the actual operator the MUO service account [OPTIONAL]
example: ./development/port-forwards default/dofinn-20210802/dofinn.openshift
$ ./development/port-forwards default/dofinn-20210802/dofinn.openshift
```

The operator can then be ran as follows. 

```
$ oc login $(oc get infrastructures cluster -o json | jq -r '.status.apiServerURL') --token $(oc -n openshift-managed-upgrade-operator serviceaccounts get-token managed-upgrade-operator)

Logged into "https://api.dofinn-20210802.8dqo.s1.devshift.org:6443" as "system:serviceaccount:openshift-managed-upgrade-operator:managed-upgrade-operator" using the token provided.

You don't have any projects. Contact your system administrator to request a project.
```

Then if you are using the standard namespace

```
$ make run-standard
```

Else you can provide your own. 


```
$ OPERATOR_NAMESPACE=managed-upgrade-operator make run
```

### Run using cluster routes

Run locally using standard namespace and cluster routes. 

```
$ make run-standard-routes
```

Run locally using custom namespace and cluster routes. 

```
$ OPERATOR_NAMESPACE=managed-upgrade-operator make run-routes
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
oc create -f test/deploy/managed_upgrade_role.yaml
oc create -f deploy/prometheus_role.yaml
oc create -f test/deploy/cluster_role_binding.yaml
oc create -f test/deploy/managed_upgrade_rolebinding.yaml
oc create -f test/deploy/prometheus_rolebinding.yaml
oc create -f test/deploy/monitoring_reader_role.yaml
oc create -f test/deploy/pullsecret_reader_role.yaml
oc create -f test/deploy/monitoring_reader_rolebinding.yaml
oc create -f test/deploy/pullsecret_reader_rolebinding.yaml
oc create -f test/deploy/service_account.yaml
oc create -f test/deploy/managed-upgrade-operator-config.yaml
```

- Set `test/deploy/operator.yaml` to use `quay.io/<QUAY_USERNAME>/managed-upgrade-operator:latest` container image and create deployment configuration by updating the `image` field:

```bash
      containers:
        - name: managed-upgrade-operator
          # Update the line below
          image: quay.io/<QUAY_USERNAME>/managed-upgrade-operator:latest
```

- Then create the `Deployment` resource:

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

### Trigger Reconcile

- Trigger a reconcile loop by applying an `upgradeconfig` CR with your desired specs.

```shell
$ oc apply -f test/deploy/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml
```

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
