# Create a CatalogSource to test manifest changes

## Create manifests/registry

This is done via [boilerplate](https://github.com/openshift/boilerplate/blob/343c71de5ca9d97876727b4842ee8bbf66eb11d7/boilerplate/openshift/golang-osd-operator/app-sre.md)

## Create or edit CatalogSource

Update the image to your custom image built in previous step. 

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

## OPTIONAL: Setup the subscription of MUO to Manual for Approval by adding installPlanApproval: Manual
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
