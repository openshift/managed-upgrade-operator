#!/usr/bin/env bash

OPERATOR_NAME=managed-upgrade-operator
GIT_HASH=$(git rev-parse --short=7 HEAD)
GIT_NUM_COMMITS=$(git rev-list $(git rev-list --max-parents=0 HEAD)..HEAD --count)
OUTPUT_DIR=./manifests/$OPERATOR_NAME
VERSION=0.1.$GIT_NUM_COMMITS-$GIT_HASH

mkdir -p $OUTPUT_DIR
./hack/generate-operator-bundle.py $OUTPUT_DIR "" $GIT_NUM_COMMITS $GIT_HASH quay.io/app-sre/$OPERATOR_NAME:latest

# create package yaml
cat <<EOF > $OUTPUT_DIR/$OPERATOR_NAME.package.yaml
packageName: ${OPERATOR_NAME}
channels:
- name: staging
  currentCSV: ${OPERATOR_NAME}.v${VERSION}
EOF
