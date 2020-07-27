#!/bin/bash

# AppSRE team CD

set -exv

CURRENT_DIR=$(dirname "$0")
BASE_IMG="managed-upgrade-operator"
IMG="${BASE_IMG}:latest"

# Validate YAML files
python "$CURRENT_DIR"/validate_yaml.py "$CURRENT_DIR"/../deploy/crds
if [ "$?" != "0" ]; then
    exit 1
fi

# Generate API changes and validate
make generate

MAKE_DIFFERENCE=$(git status --porcelain)

if [ -n "${MAKE_DIFFERENCE}" ]; then
	echo "Pull Request has modified changes. Please run ./hack/app_sre_pr_check.sh locally and commit changes"
	git diff --exit-code
fi

# Run golangci-lint lint test on code
make lint

# Build Docker image
BUILD_CMD="docker build" IMG="$IMG" make docker-build
