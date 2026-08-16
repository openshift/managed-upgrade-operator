# Development

- [Development Environment Setup](#development-environment-setup)
  - [golang](#golang)
  - [operator-sdk](#operator-sdk)
- [Makefile](#makefile)
- [Dependencies](#dependencies)
- [Linting](#linting)
- [Testing](#testing)
- [Building](#building)
- [Build using boilerplate container](#build-using-boilerplate-container)
- [Running Locally](#running-locally)
- [Running Remotely](#running-remotely)
- [Monitoring ongoing upgrade](#monitoring-ongoing-upgrade)
- [Maintenance](#maintenance)

Everything you need to develop this operator locally.

## Development Environment Setup

### golang

A recent Go distribution (>=1.23) with enabled Go modules.

```shell
go version
go version go1.23.9 linux/amd64
```

### operator-sdk

The Operator is being developed based on the [Operators SDK](https://github.com/operator-framework/operator-sdk).
Ensure this is installed and available in your `$PATH`.

[v1.21.0](https://github.com/operator-framework/operator-sdk/releases/tag/v1.21.0) is being used for `managed-upgrade-operator` development.

```shell
operator-sdk version
operator-sdk version: "v1.21.0", commit: "89d21a133750aee994476736fa9523656c793588", kubernetes version: "1.23", go version: "go1.17.10", GOOS: "linux", GOARCH: "amd64"
```

## Makefile

All available standardized commands for the `Makefile` are available via:

```shell
make
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
make tools
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
make lint
```

## Testing

To run unit tests locally, call `make go-test`

```shell
make go-test
```

## Building

To run go build locally, call `make go-build`

```shell
make go-build
```

## Build using boilerplate container

To run lint, test and build in `app-sre/boilerplate` container, call `boilerplate/_lib/container-make`. This will call `make` inside the `app-sre/boilerplate` container.

```shell
boilerplate/_lib/container-make
```
### Note for macOS Developers:
When building on macOS, the boilerplate container-based build approach `make container-build` is not supported due to limitations with running Podman-in-Podman in a cross-platform environment. Instead, macOS users should use `make go-mac-build`
to build arm64 image `podman build --platform=linux/amd64 -t quay.io/[$PERSONAL_REPO]/managed-upgrade-operator:[$IMAGE_VERSION] -f build/Dockerfile .`

## Running Locally

- Make sure you have the [operator-sdk](#operator-sdk) in your `$PATH`.
- Install an OSD cluster using [staging](https://console.redhat.com/openshift/create?env=staging) or [integration](https://console.redhat.com/openshift/create?env=integration) environment. Make sure to use CCS option.
- Once the cluster installs, create a user with `cluster-admin` role and log in using `oc` client.
- Scale down existing MUO deployment and delete the leader lease:
```shell
oc scale deployment managed-upgrade-operator -n openshift-managed-upgrade-operator --replicas=0
oc delete lease managed-upgrade-operator-lock -n openshift-managed-upgrade-operator
```
- Apply the LOCAL mode ConfigMap. This prevents OCM from deleting manually-created UpgradeConfigs and ignores the `UpgradeConfigSyncFailureOver4HrSRE` alert that fires when the deployed MUO is scaled down:
```shell
oc apply -f test/deploy/managed-upgrade-operator-config.yaml -n openshift-managed-upgrade-operator
```
- Switch to the MUO service account. The operator must run as this account to bypass webhooks that block UpgradeConfig modifications:
```shell
oc login $(oc get infrastructures cluster -o json | jq -r '.status.apiServerURL') \
  --token=$(oc create token managed-upgrade-operator -n openshift-managed-upgrade-operator)
```
- Apply an UpgradeConfig CR. Edit `test/deploy/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml` with your desired version and channel, then apply it:
```shell
oc apply -f test/deploy/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml
```
- Run the operator. `ROUTES=true` tells MUO to use OpenShift Routes to reach Prometheus and Alertmanager, which is the simplest option for local development:
```shell
ROUTES=true OSDK_FORCE_RUN_MODE=local make run
```

> **Note:** If you prefer using internal cluster services instead of routes, see the [`development/port-forwards`](../development/port-forwards) script. This approach requires `/etc/hosts` entries and port-forwards but replicates the production network path.

## Running Remotely

- Build the image. In this example, we will use [Quay](http://quay.io/) as the container registry for our image:

```shell
make docker-build IMG=quay.io/<QUAY_USERNAME>/managed-upgrade-operator:latest
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
oc new-project test-managed-upgrade-operator
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
