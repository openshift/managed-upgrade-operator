## Locally running e2e test suite
When updating your operator, add e2e tests for new functionality and ensure existing functionality continues to work. The following steps are recommended:

1. Run "make e2e-binary-build"  to make sure e2e tests build
2. Deploy your new version of operator in a test cluster
3. Run "go install github.com/onsi/ginkgo/ginkgo@latest"
4. Get kubeadmin credentials from your cluster using

ocm get /api/clusters_mgmt/v1/clusters/(cluster-id)/credentials | jq -r .kubeconfig > /(path-to)/kubeconfig

5. Run test suite using

DISABLE_JUNIT_REPORT=true KUBECONFIG=/(path-to)/kubeconfig  ./(path-to)/bin/ginkgo  --tags=osde2e -v test/e2e

## Multi-architecture migration

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
