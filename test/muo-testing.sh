#!/bin/bash

# Usage: ./muo-testing.sh <cluster-internal-id> <muo_image>

set -e

NAMESPACE=openshift-managed-upgrade-operator
POD_NAME=managed-upgrade-operator
TEST_POD_NAME=managed-upgrade-operator-test
MUO_DEPLOYMENT_YAML=muo-test-deployment.yaml

#Input variables
CLUSTERID=$1
QUAY_USER=$2
IMAGE_SHA_ID=$3

logged_in_ocm(){
  # Logged into ocm?
  if ! ocm whoami > /dev/null ; then
    fail_exit "Must login to ocm api first"
  fi
}

fail_exit(){
  message="${1}"

  echo -e "${message}\n" 2>&1
  exit 1
}

for param in "$@"; do
    if [ -z "$param" ]; then
        echo "Usage: ./muo_manual_testing.sh <CLUSTER_INTERNAL_ID> <QUAY_USER> <IMAGE_SHA_ID>"
        exit 1
    fi
done


 #login to ocm stage
ocm login --use-auth-code --url stage > /dev/null
logged_in_ocm


#pause hive sync
HIVE=$(ocm describe cluster "${CLUSTERID}"  | grep Shard | cut -d "." -f 2)
CLUSTER_DEPLOYMENT=$(ocm describe cluster "${CLUSTERID}" --json | jq -r '.name')


# Annotation to pause sync
ANNOTATION='hive.openshift.io/syncset-pause'
CLUSTER_DEPLOYMENT_NAMESPACE=uhc-staging-${CLUSTERID}

echo $ANNOTATION
echo $CLUSTER_DEPLOYMENT_NAMESPACE

# login to hive cluster
ocm login --use-auth-code --url prod > /dev/null
ocm backplane login "${HIVE}"

oc annotate clusterdeployment "${CLUSTER_DEPLOYMENT}" -n "${CLUSTER_DEPLOYMENT_NAMESPACE}" "${ANNOTATION}"="true"  || fail_exit "Something failed attempting to annotate clusterdeployment"
ocm backplane logout

 #Scale down the pod
ocm backplane login "${CLUSTERID}"
oc -n "$NAMESPACE" scale --replicas=0 deployment/"$POD_NAME" 

NEW_IMAGE="quay.io\/${QUAY_USER}\/managed-upgrade-operator\@sha256:${IMAGE_SHA_ID}"
echo "Updated Image name : $NEW_IMAGE" 

# Perform the substitution using sed in muo-test-deployment.yaml
sed -i "" "/^\([[:space:]]*image: \).*/s//\1$NEW_IMAGE/" $MUO_DEPLOYMENT_YAML

echo "Image name updated to $NEW_IMAGE in muo-test-deployment.yaml"

oc create -f $MUO_DEPLOYMENT_YAML --as backplane-cluster-admin

# echo "MUO test deployment completed."
