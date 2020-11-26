# Create a CatalogSource to test manifest changes

## Create manifests
```bash
make manifests
```

## Create registry

### Create registry Dockerfile
```bash
cat <<EOF > LocalRegistryDockerfile
FROM quay.io/openshift/origin-operator-registry:latest

COPY manifests manifests
RUN initializer --permissive

CMD ["registry-server", "-t", "/tmp/terminate.log"]
EOF
```

### Build and push Dockerfile

```bash
buildah build-using-dockerfile -f ./LocalRegistryDockerfile --tag quay.io/USER/managed-upgrade-operator-registry:latest .
podman push IMAGE_ID docker://quay.io/USER/managed-upgrade-operator-registry:latest
```

## Create CatalogSource

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: managed-upgrade-operator-catalog
spec:
  displayName: Managed Upgrade Operator
  image: 'quay.io/<your_repo>/managed-upgrade-operator-registry:<tag>'
  publisher: Red Hat
  sourceType: grpc
```

__NOTE:__ Ensure `quay.io/<your_repo>/managed-upgrade-operator-registry:<tag>` image is publically available

```bash
oc create -f CatalogSource.yaml
catalogsource.operators.coreos.com/managed-upgrade-operator-catalog created
```

## Setup the subscription of MUO to Manual for Approval by adding installPlanApproval: Manual
```yaml
spec:
  channel: staging
  name: managed-upgrade-operator
  source: managed-upgrade-operator-catalog
  sourceNamespace: my-muo
  installPlanApproval: Manual
```

```bash
oc create -f sub.yaml
```

__NOTE:__ Even after approving the installPlan, it is not proceeding in the cluster web console then you might want to take a look at the operatorgroup which is required in order to watch your NS by OLM.

```bash
oc get operatorgroup -A
```
If you don't see it exists, then try creating one. After that delete and re-create the subscription under your NS.

This time once you approve the installPlan from the cluster web console you will see it progressing within a few seconds.
