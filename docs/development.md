# Development

This document should entail all you need to develop this operator locally. 

## Development Environment Setup

### golang

A recent Go distribution (>=1.13) with enabled Go modules.

```
$ go version
go version go1.13.15 linux/amd64
$
```

### operator-sdk

The Operator is being developed based on the [Operators SDK](https://github.com/operator-framework/operator-sdk). 
Ensure this is installed and available in your `$PATH`.  

Presently, `v0.17.0` is being used as the baseline for `managed-upgrade-operator` development.  

```
# operator-sdk version
operator-sdk version: "v0.17.0", commit: "2fd7019f856cdb6f6618e2c3c80d15c3c79d1b6c", kubernetes version: "unknown", go version: "go1.13.10 linux/amd64"
```

## Dependencies

The tool dependencies that are required locally to be present are all part of [tools.go](https://github.com/openshift/managed-upgrade-operator/blob/master/tools.go) file. This file will refer the version of the required module from [go.mod](https://github.com/openshift/managed-upgrade-operator/blob/master/go.mod) file.

In order to install the tool dependencies locally, simply run the below command which will fetch the tools for you and install the binaries at location `$GOPATH/bin` by default:

```
$ make tools
```

This will make sure that the installed binaries are always as per the required version mentioned in `go.mod` file. If the version of the module is changed, need to run the command again locally to have new version of tools.

## How to run

Regardless of how you choose to run the operator, before doing so you will need to install the `UpgradeConfig` CRD on your cluster:

```bash
$ oc create -f deploy/crds/upgrade.managed.openshift.io_upgradeconfigs_crd.yaml
```

### Locally

* Make sure you have the [operator-sdk](https://github.com/operator-framework/operator-sdk/releases) binary in your `$PATH`.

* If you are not using an account that has `cluster-admin` privileges, you will need to [elevate permissions](https://github.com/openshift/ops-sop/blob/master/v4/howto/manage-privileges.md) to possess them.

* Create a project for the operator to run inside of.

```
$ oc new-project managed-upgrade-operator
```

* Run the operator via the Operator SDK:

```
$ OPERATOR_NAMESPACE=managed-upgrade-operator operator-sdk run --local --watch-namespace=""
``` 

(`make run` will also work here)

* Trigger a reconcile loop by applying an `upgradeconfig` CR with your desired specs. 

```
oc apply -f test/deploy/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml
```

### Remotely

* Create a project for the operator to run inside of.

```
$ oc new-project managed-upgrade-operator
```

* Build the image. In this example, we will use [Quay](http://quay.io/) as the container registry for our image.

```bash
$ operator-sdk build quay.io/myuser/managed-upgrade-operator:latest 
``` 

* Push the image to the registry.

```bash
podman push quay.io/myuser/managed-upgrade-operator:latest
```

* Deploy the service account, clusterrole, clusterrolebinding and ConfigMap on your target cluster.

```bash
oc create -f deploy/cluster_role.yaml
oc create -f deploy/cluster_role_binding.yaml
oc create -f test/deploy/service_account.yaml
oc create -f test/deploy/managed-upgrade-operator-config.yaml
```

* Edit the `deploy/operator.yaml` file to represent the path to your image, and deploy it:

```yaml
spec:
  template:
    spec:
      containers:
        - name: managed-upgrade-operator
          image: quay.io/myuser/managed-upgrade-operator:latest
``` 

```bash
$ oc create -f deploy/operator.yaml
```

* Trigger a reconcile loop by applying an `upgradeconfig` CR with your desired specs. 

```bash
$ oc create -f test/deploy/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml
```
