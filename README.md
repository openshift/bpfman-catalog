# bpfman-catalog

OLM catalog used to release the OpenShift eBPF Manager Operator.

## Overview

This repository builds and publishes OLM catalogs for releasing the OpenShift eBPF Manager Operator. Catalogs are generated from curated templates that define specific operator versions and upgrade paths, then packaged as OCI images for deployment to OpenShift clusters.

The repository also includes a CLI tool (`bpfman-catalog`) for rapid deployment and testing of catalogs, supporting catalog.yaml files, catalog images, and bundle images with automated deploy/undeploy workflows.

## Quick Start

### Releases (Primary Use Case)

```bash
# Edit templates/y-stream.yaml or templates/z-stream.yaml to update operator versions.
# Then regenerate catalogs.
make generate-catalogs

# Commit the updated catalogs.
git add auto-generated/catalog/
git commit -m "Update catalog for new release"
```

The generated catalogs are then built and deployed through CI/CD pipelines.

### Development Testing

#### Testing Pre-built Catalog Images

For deploying and testing pre-built catalog images:

```bash
# Build, push, and deploy catalog image.
make build-image push-image deploy

# Install operator via OpenShift console UI.
# Navigate to Operators → OperatorHub and install the operator.
```

This uses `catalog-source.yaml` as a template to deploy the catalog image. Operator installation is manual via the console.

#### Testing Individual Bundles (CLI Tool)

For testing individual bundle images with automated deployment:

```bash
# Build the CLI tool.
make build-cli

# Generate catalog from bundle with full automation.
./bin/bpfman-catalog prepare-catalog-build-from-bundle \
  quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest

# Build, push, and auto-subscribe to operator.
make -C auto-generated/artefacts all

# Clean up when done.
make -C auto-generated/artefacts undeploy
```

The CLI tool provides complete automation including namespace creation, IDMS configuration, and automatic subscription to the operator.

## Directory Structure

- `templates/` - Curated catalog templates for releases
  - `y-stream.yaml` - Y-stream minor version releases
  - `z-stream.yaml` - Z-stream patch releases
- `auto-generated/catalog/` - Generated catalogs from templates
- `cmd/bpfman-catalog/` - CLI tool source code
- `pkg/` - Go packages for catalog operations
- `Dockerfile` - Container definition for building catalog images
- `catalog-source.yaml` - CatalogSource resource template

## Configuration

Environment variables and Make variables:

- `IMAGE` - Target image name (default: `quay.io/$USER/bpfman-operator-catalog:latest`)
- `BUILD_STREAM` - Template to use (default: `y-stream`, options: `y-stream`, `z-stream`)
- `BPFMAN_CATALOG_QUAY_USER` - Override username for Quay.io image references (takes precedence over `$USER`)
- `OCI_BIN` - Container runtime (`docker` or `podman`, auto-detected)
- `LOG_LEVEL` - CLI logging level (default: `info`, options: `debug`, `info`, `warn`, `error`)
- `LOG_FORMAT` - CLI log format (default: `text`, options: `text`, `json`)

## CLI Tool Workflows (Development)

The `bpfman-catalog` CLI tool provides three workflows for ephemeral testing during development:

### Building the CLI

```bash
make build-cli
```

The tool will be available at `./bin/bpfman-catalog`. Run `./bin/bpfman-catalog --help` for detailed usage information.

### 1. Build catalog from a bundle image

Generates complete build artefacts from a bundle.

```bash
# Generates: Dockerfile, catalog.yaml, Makefile.
./bin/bpfman-catalog prepare-catalog-build-from-bundle \
  quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest

# Builds image, pushes to registry, deploys to cluster with auto-subscribe.
make -C auto-generated/artefacts all
```

### 2. Build catalog from catalog.yaml

Wraps an existing or modified catalog.yaml with build artefacts.

```bash
# Edit the catalog YAML if needed.
$EDITOR auto-generated/catalog/y-stream.yaml

# Generates: Dockerfile, Makefile.
./bin/bpfman-catalog prepare-catalog-build-from-yaml auto-generated/catalog/y-stream.yaml

# Builds image, pushes to registry, deploys to cluster with auto-subscribe.
make -C auto-generated/artefacts all
```

### 3. Deploy existing catalog image

Generates Kubernetes manifests to deploy a catalog to a cluster.

```bash
# Produces: CatalogSource, Namespace, IDMS, Subscription.
./bin/bpfman-catalog prepare-catalog-deployment-from-image \
  quay.io/redhat-user-workloads/ocp-bpfman-tenant/catalog-ystream:latest

# Deploy catalog to cluster with auto-subscribe.
kubectl apply -f auto-generated/manifests/
```
