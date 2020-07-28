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
