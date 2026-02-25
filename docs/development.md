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
  - [Maintenance](#maintenance)

This document should entail all you need to develop this operator locally.

## Development Environment Setup

### golang

A recent Go distribution (>=1.23) with enabled Go modules.

```shell
$ go version
go version go1.23.9 linux/amd64
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

The operator uses the official OpenShift Cluster Manager SDK (`github.com/openshift-online/ocm-sdk-go`) for all OCM API interactions. Custom OCM model structs have been replaced with typed SDK models from the `clustersmgmt/v1` and `servicelogs/v1` packages.

In order to install the tool dependencies locally, run `make tools` at the root of the cloned repository, which will fetch the tools for you and install the binaries at location `$GOPATH/bin` by default:

```shell
$ make tools
```

This will make sure that the installed binaries are always as per the required version mentioned in `go.mod` file. If the version of the module is changed, need to run the command again locally to have new version of tools.

---

**NOTE**

If any of the dependencies are failing to install due to checksum mismatch, try setting `GOPROXY` env variable using `export GOPROXY="https://proxy.golang.org"` and run `make tools` again

---

## Proxy Configuration

The operator supports proxy environments through standard Go HTTP proxy environment variables:

| Variable | Description | Example |
| --- | --- | --- |
| `HTTP_PROXY` | Proxy URL for HTTP requests | `http://proxy.example.com:8080` |
| `HTTPS_PROXY` | Proxy URL for HTTPS requests | `https://proxy.example.com:8080` |
| `NO_PROXY` | Comma-separated list of hosts to bypass proxy | `localhost,127.0.0.1,.cluster.local` |

The proxy configuration is automatically applied to:
- OCM API client (for external OpenShift Cluster Manager communication)
- DVO client (for Deployment Validation Operator communication in Routes mode)
- AlertManager client (for alert management)
- Metrics client (for Prometheus metrics)

**Note**: The OCM Agent client (local cluster service) does not use proxy configuration as it communicates with local services only.

### HTTP Client Configuration

The operator uses the OCM SDK with enhanced timeout and retry configuration:

- **Connection timeout**: 30 seconds (TCP connection establishment)
- **TLS handshake timeout**: 10-30 seconds (depending on service)
- **Keep-alive interval**: 30 seconds (TCP keep-alive probes)
- **Retry configuration**: 5 maximum retries with 2-second initial delay and 30% jitter

These settings ensure reliable communication in high-latency and proxy environments.

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
### Note for macOS Developers:
When building on macOS, the boilerplate container-based build approach `make container-build` is not supported due to limitations with running Podman-in-Podman in a cross-platform environment. Instead, macOS users should use `make go-mac-build`
to build arm64 image `podman build --platform=linux/amd64 -t quay.io/[$PERSONAL_REPO]/managed-upgrade-operator:[$IMAGE_VERSION] -f build/Dockerfile .`

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

- Make sure you have the [operator-sdk](#operator-sdk) in your `$PATH`.
- Install an OSD cluster using [staging](https://console.redhat.com/openshift/create?env=staging) or [integration](https://console.redhat.com/openshift/create?env=integration) environment. Make sure to use CCS option.
- Once the cluster installs, create a user with `cluster-admin` role and log in using `oc` client.
- Scale down existing MUO deployment and delete the leader lease:
```
oc scale deployment managed-upgrade-operator -n openshift-managed-upgrade-operator --replicas=0
oc delete lease managed-upgrade-operator-lock -n openshift-managed-upgrade-operator
```
- Apply the LOCAL mode ConfigMap. This prevents OCM from deleting manually-created UpgradeConfigs and ignores the `UpgradeConfigSyncFailureOver4HrSRE` alert that fires when the deployed MUO is scaled down:
```
oc apply -f test/deploy/managed-upgrade-operator-config.yaml -n openshift-managed-upgrade-operator
```
- Switch to the MUO service account. The operator must run as this account to bypass webhooks that block UpgradeConfig modifications:
```
oc login $(oc get infrastructures cluster -o json | jq -r '.status.apiServerURL') --token=$(oc create token managed-upgrade-operator -n openshift-managed-upgrade-operator)
```
- Now you can run the operator with [service](#Run using internal cluster services) or [routes](#Run using cluster routes)

#### Run using internal cluster services

The use of internal cluster services requires some changes locally to your environment:

1. Add entries for prometheus and alertmanager to `/etc/hosts` that resolve to `127.0.0.1`
2. Create port-forwards for each service

You can run this script to set this up for you (Requires `sudo` as it writes to `/etc/hosts`). The script accepts an optional `--context`. This enables setting up of the port-forwards with your personal user in the OpenShift cluster while running the operator locally using the available MUO service account for production like replication.

```
$ ./development/port-forwards -h
Setup port forwarding for local development using internal svc
usage ./development/port-forwards context (to execute oc commands as). This is useful if you are running the actual operator the MUO service account [OPTIONAL]
example: ./development/port-forwards $CONTEXT
$ ./development/port-forwards $CONTEXT
```

The operator can then be ran as follows. 

```
$ oc login $(oc get infrastructures cluster -o json | jq -r '.status.apiServerURL') --token $(oc -n openshift-managed-upgrade-operator serviceaccounts get-token managed-upgrade-operator)

Logged into "https://$API_URL:6443" as "system:serviceaccount:openshift-managed-upgrade-operator:managed-upgrade-operator" using the token provided.

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

#### Run using cluster routes

Run locally using standard namespace and cluster routes. Both `ROUTES=true` and `OSDK_FORCE_RUN_MODE=local` are required for the operator to use cluster routes instead of internal service addresses.

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
$ make docker-build IMG=quay.io/<QUAY_USERNAME>/managed-upgrade-operator:latest
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

## Maintenance

We can leverage the script for maintenance:
```bash
python hack/maintenance-update.py
```

Overall the script does the following:
- By default, update the common dependencies like `github.com/openshift-online/ocm-sdk-go`, `github.com/openshift/osde2e-common`
- If specified then update dependencies as per required OpenShift version (Eg. release-4.19)
- Run go mod tidy on the changes
- Validate the changes with boilerplate validations
- Review and add/commit the changes if all seems good so far
- Update boilerplate
- Perform validations again
- Review and add/commit the changes if all seems good so far

For the periodic maintenance, can leverage the script as follows:
```bash
# To update existing deps to latest version
python hack/maintenance-update.py

# To update deps to a specific Openshift release
python hack/maintenance-update.py --release release-4.19
```
