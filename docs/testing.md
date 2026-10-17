# Testing

Tests are playing a primary role and we take them seriously.
It is expected from PRs to add, modify or delete tests on case by case scenario.
To contribute you need to be familiar with:

* [Ginkgo](https://github.com/onsi/ginkgo) - BDD Testing Framework for Go
* [Gomega](https://onsi.github.io/gomega/) - Matcher/assertion library

## Prerequisites

Make sure that the [tool dependencies](https://github.com/openshift/managed-upgrade-operator/blob/master/docs/development.md#dependencies) are already in place. The `ginkgo` and `mockgen` binaries that are required for testing will be installed as part of tool dependencies.

## Bootstrapping the tests
```
$ cd pkg/maintenance
$ ginkgo bootstrap
$ ginkgo generate maintenance.go

find .
./maintenance.go
./maintenance_suite_test.go
./maintenance_test.go
```

## How to run the tests

* You can run the tests using `make test` or `go test ./...`

## Multi-architecture migration E2E test

The `architecture-migration` test creates a `managed-upgrade-config` with
`architecture: Multi` at the cluster's current version. It checks the history
created by the controller, the resulting ClusterVersion migration request, and
completion reported by both CVO and MUO. It does not patch ClusterVersion itself.

This test starts a real migration and is disabled unless
`RUN_ARCHITECTURE_MIGRATION_E2E=true`. Use a disposable, healthy single-architecture
cluster with the updated MUO and CRD installed, a multi-architecture release
available in its current channel, and no existing UpgradeConfig or rollout.
Configure MUO's `configManager.source` as `LOCAL` with
`localConfigName: managed-upgrade-config` before running it. The test does not
change the operator configuration or bypass its health checks.

```sh
RUN_ARCHITECTURE_MIGRATION_E2E=true \
DISABLE_JUNIT_REPORT=true \
KUBECONFIG=/path/to/test-cluster/kubeconfig \
go test -tags=osde2e ./test/e2e -timeout 3h30m \
  -args -ginkgo.label-filter=architecture-migration -ginkgo.v
```

The test deletes its UpgradeConfig after successful completion. If the migration
fails or times out, it leaves the resource for MUO to continue reconciling and for
diagnosis. It does not roll back the cluster architecture.

## Writing tests

### Mocking interfaces

#### Basic Mock knowledge and process
This project makes use of [`GoMock`](https://github.com/golang/mock) to mock service interfaces. This comes with the `mockgen` utility which can be used to generate or re-generate mock interfaces that can be used to simulate the behaviour of an external dependency.

Once installed, an interface can be mocked by running:

```
mockgen -s=/path/to/file_containing_interface.go > /path/to/output_mock_file.go
```

However, it is considered good practice to include a [go generate](https://golang.org/pkg/cmd/go/internal/generate/) directive above the interface which defines the specific `mockgen` command that will generate your mocked interface.

Internal interfaces including `pkg/maintenance/maintenance.go` and `pkg/controller/upgradeconfig/cluster_upgrader.go` are mocked using this method. When making changes to these packages, you should re-generate the mocks to ensure they too are updated. This can be performed manually by running `go generate /path/to/file.go` or for the whole project via `make generate`.

- Mocks might need to be regenerated upon dependency updates, e.g. k8s/controller-runtime

To regenerate controller-runtime mock (cr-client.go), run

```
mockgen -package mocks -destination=util/mocks/cr-client.go sigs.k8s.io/controller-runtime/pkg/client Client,StatusWriter,Reader,Writer
```
#### Update Mock via boilerplate container

The easiest and recommend way to make an update on the Mock when there is any interface update is using boilerplate-container. Because boilerplate container installs all the dependencies and has all the environment setup. Most important, boilerplate container resolves a lot of platform, arch and version impacts while generate the mocks manually.

To regenerate the mocks by using boilerplate container, simply run the following command
```
./boilerplate/_lib/container-make generate
```
