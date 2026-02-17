# Managed Upgrade Operator - Claude Code Development Guide

## Overview

The **Managed Upgrade Operator** is a Kubernetes operator written in Go that manages automated in-place cluster upgrades for OpenShift Dedicated Platform (OSD) and Azure Red Hat OpenShift (ARO). It orchestrates the upgrade process through pre-upgrade validation, capacity management, maintenance windows, and post-upgrade verification.

## Architecture & Technology Stack

### Core Technologies
- **Language**: Go 1.23.0 (with toolchain go1.23.9)
- **Framework**: Kubernetes Operator built with `operator-sdk v1.21.0`
- **Controller Framework**: `sigs.k8s.io/controller-runtime v0.20.0`
- **Testing**: Ginkgo v2 (BDD testing framework) + Gomega (matcher/assertion library)
- **Mocking**: GoMock for interface mocking
- **Container Base**: UBI9 minimal (registry.access.redhat.com/ubi9/ubi-minimal:9.6-1755695350)

### Key Dependencies
- **OpenShift APIs**: `github.com/openshift/api`, `github.com/openshift/client-go`
- **Cluster Version Operator**: `github.com/openshift/cluster-version-operator`
- **OCM SDK**: `github.com/openshift-online/ocm-sdk-go v0.1.494+` (official SDK for managed cluster integration)
  - Uses typed models from `clustersmgmt/v1` and `servicelogs/v1` packages
  - Replaces legacy custom OCM model structs
- **Prometheus**: Metrics collection and alerting management
- **Kubernetes APIs**: `k8s.io/api`, `k8s.io/client-go`, `k8s.io/apimachinery`

## Directory Structure

```
managed-upgrade-operator/
├── api/v1alpha1/              # Custom Resource Definitions (CRDs)
│   └── upgradeconfig_types.go # UpgradeConfig CRD definition
├── controllers/               # Kubernetes controllers
│   ├── upgradeconfig/         # Main upgrade orchestration controller
│   ├── nodekeeper/            # Node upgrade tracking controller
│   └── machineconfigpool/     # MachineConfigPool controller
├── pkg/                       # Core business logic packages
│   ├── upgraders/             # Cluster upgrader implementations (OSD, ARO)
│   ├── upgradesteps/          # Individual upgrade step implementations
│   ├── clusterversion/        # CVO interaction utilities
│   ├── drain/                 # Node draining strategies
│   ├── scaler/                # Node capacity scaling
│   ├── maintenance/           # Maintenance window management
│   ├── alertmanager/          # Alert silencing management
│   ├── notifier/              # Notification systems
│   ├── validation/            # Pre/post upgrade validations
│   ├── scheduler/             # Upgrade scheduling logic
│   ├── metrics/               # Prometheus metrics
│   └── configmanager/         # Configuration management
├── test/                      # Test infrastructure
│   ├── deploy/                # Test deployment manifests
│   └── e2e/                   # End-to-end tests
├── deploy/                    # Production deployment manifests
├── docs/                      # Documentation
├── boilerplate/               # Shared build tooling
├── build/                     # Container build files
│   └── Dockerfile             # Multi-stage container build
└── hack/                      # Build and maintenance scripts
```

## Core Concepts

### UpgradeConfig Custom Resource
The operator is driven by `UpgradeConfig` CRs that define:
- Target OpenShift version (`spec.desired.version`)
- Upgrade channel (`spec.desired.channel`)
- Upgrade start time (`spec.upgradeAt`)
- Drain timeout (`spec.PDBForceDrainTimeout`)
- Capacity reservation needs (`spec.capacityReservation`)
- Upgrade type: OSD or ARO (`spec.type`)

### External Service Integration

**OCM API Integration**:
- Uses official `ocm-sdk-go` with typed models
- Supports proxy environments (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`)
- Automatic retry with exponential backoff (5 retries, 2s initial delay, 30% jitter)
- Enhanced timeouts for high-latency environments (30s connection, 10s TLS handshake)

**Client Implementations**:
- `pkg/ocm`: External OCM API client (api.openshift.com) with proxy support
- `pkg/ocmagent`: Local OCM Agent client (cluster service) without proxy
- `pkg/dvo`: Deployment Validation Operator client with proxy support
- `pkg/maintenance`: AlertManager client with proxy support
- `pkg/metrics`: Prometheus metrics client with proxy support

### Controllers Architecture
1. **UpgradeConfig Controller**: Main orchestrator that processes UpgradeConfig CRs
2. **NodeKeeper Controller**: Monitors and remediates stuck node upgrades
3. **MachineConfigPool Controller**: Tracks MCP upgrade progress

### Upgrade Process Flow
1. **Pre-upgrade**: Health checks, capacity reservation, maintenance windows
2. **Control Plane Upgrade**: Triggers CVO to upgrade control plane
3. **Worker Node Upgrade**: Manages worker node drain and upgrade
4. **Post-upgrade**: Cleanup, health validation, notifications

## Build Commands

### Essential Development Commands
```bash
# Install tool dependencies
make tools

# Build the operator binary
make go-build

# Run tests
make go-test

# Run linting
make go-check

# Generate code (mocks, CRDs)
make generate

# Update boilerplate
make boilerplate-update

# Run operator locally (requires cluster access)
make run

# Build container image
make docker-build IMG=quay.io/<username>/managed-upgrade-operator:latest
```

### Development Environment Setup
```bash
# Install required tools
make tools

# For macOS developers (cross-compile for Linux)
make go-mac-build

# Run with boilerplate container (recommended for consistency)
./boilerplate/_lib/container-make

# Run locally with standard namespace
make run-standard

# Run locally using cluster routes (easier for development)
make run-standard-routes
```

## Testing Framework

### Unit Tests
- Uses **Ginkgo v2** BDD framework with **Gomega** assertions
- Tests located alongside source code with `_test.go` suffix
- Run with: `make go-test` or `go test ./...`

### Mock Generation
```bash
# Regenerate all mocks (recommended approach)
./boilerplate/_lib/container-make generate

# Manual mock generation example
mockgen -package mocks -destination=util/mocks/cr-client.go sigs.k8s.io/controller-runtime/pkg/client Client,StatusWriter,Reader,Writer
```

### E2E Tests
- Located in `test/e2e/`
- Uses Ginkgo for E2E test orchestration
- Deployment manifests in `test/deploy/`

## Key Configuration

### Environment Variables
- `OPERATOR_NAMESPACE`: Namespace where operator runs (default: "openshift-managed-upgrade-operator")
- `WATCH_NAMESPACE`: Namespaces to watch for resources (default: all namespaces)

### RBAC Requirements
The operator requires extensive cluster-level permissions defined in:
- `deploy/cluster_role.yaml`
- `test/deploy/managed_upgrade_role.yaml`
- Various monitoring and pull-secret reader roles

## CI/CD Pipeline

### Build System
- **Boilerplate Framework**: Uses `app-sre/boilerplate` for standardized builds
- **Tekton Pipelines**: `.tekton/` directory contains pipeline definitions
- **CI Operator**: `.ci-operator.yaml` configures OpenShift CI
- **Container Registry**: Builds push to Quay.io

### Quality Gates
- **golangci-lint**: Static analysis (config in `.golangci.yml`)
- **Unit Tests**: Ginkgo test suite
- **E2E Tests**: Full cluster upgrade testing
- **Security**: FIPS-enabled builds (`FIPS_ENABLED=true`)

## Development Workflow

### Local Development
1. **Setup**: Run `make tools` to install dependencies
2. **Code**: Implement changes in appropriate `pkg/` subdirectories
3. **Test**: Add/update tests using Ginkgo framework
4. **Validate**: Run `make go-check` for linting and `make go-test` for tests
5. **Build**: Use `make go-build` to compile
6. **Deploy**: Use local deployment with `make run-standard-routes`

### Testing an Upgrade
```bash
# Create UpgradeConfig CR
oc apply -f - <<EOF
apiVersion: upgrade.managed.openshift.io/v1alpha1
kind: UpgradeConfig
metadata:
  name: managed-upgrade-config
spec:
  type: "OSD"
  upgradeAt: "2024-01-01T12:00:00Z"
  PDBForceDrainTimeout: 60
  desired:
    channel: "fast-4.14"
    version: "4.14.15"
EOF

# Monitor upgrade progress
oc get upgradeconfig -w
oc describe upgradeconfig managed-upgrade-config
```

### Adding New Features
1. **Interfaces**: Define interfaces in appropriate `pkg/` subdirectory
2. **Implementation**: Implement concrete types
3. **Tests**: Add comprehensive unit tests
4. **Mocks**: Generate mocks using `make generate`
5. **Integration**: Wire into controller or upgrade step chain

## Important Files & Entry Points

### Main Entry Point
- `main.go`: Operator bootstrap, controller setup, metrics configuration

### Core Interfaces
- `pkg/upgraders/upgrader.go`: ClusterUpgrader interface
- `pkg/upgradesteps/runner.go`: UpgradeStep interface
- `pkg/configmanager/configmanager.go`: Configuration management

### Key Controllers
- `controllers/upgradeconfig/upgradeconfig_controller.go`: Main reconciler
- `controllers/nodekeeper/nodekeeper_controller.go`: Node management
- `controllers/machineconfigpool/machineconfigpool_controller.go`: MCP tracking

## Monitoring & Observability

### Metrics
- Prometheus metrics defined in `pkg/metrics/`
- Custom metrics service and ServiceMonitor auto-created
- Exposes upgrade progress, duration, success/failure rates

### Alerting
- Integrates with AlertManager for alert silencing during upgrades
- Custom alerts for stuck upgrades and operator health

## Security & FIPS

- **FIPS Compliance**: Builds with FIPS-enabled Go toolchain
- **Security Context**: Runs as non-root user (UID 1001)
- **RBAC**: Minimal required permissions with separate roles for different functions
- **Container Security**: Uses Red Hat UBI minimal base images

## Documentation

### Developer Resources
- `docs/development.md`: Detailed development setup and workflows
- `docs/testing.md`: Comprehensive testing guide
- `docs/design.md`: Architecture and design documentation
- `docs/metrics.md`: Prometheus metrics reference
- `README.md`: Project overview and basic usage

This operator represents a production-grade Kubernetes operator with sophisticated upgrade orchestration, comprehensive testing, and enterprise security requirements. The codebase follows cloud-native best practices and integrates deeply with OpenShift's upgrade infrastructure.