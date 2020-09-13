# Development

This document should entail all you need to develop this operator locally. 

## Development Environment Setup

### golang

A recent Go distribution (>=1.13) with enabled Go modules.

```
$ go version
$ export GO111MODULE=on
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

### GoMock

[`GoMock`](https://github.com/golang/mock) is used for building or re-building mock interfaces used in [testing](./testing.md). If you are undertaking development which may involve the (re-)creation of mocked interfaces, it will be required.

`GO111MODULE=on go get github.com/golang/mock/mockgen@latest`

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
oc apply -f deploy/crds/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml
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
oc create -f deploy/role_binding.yaml
oc create -f deploy/service_account.yaml
oc create -f deploy/managed-upgrade-operator-config.yaml
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
$ oc create -f deploy/crds/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml
```
