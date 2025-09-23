# bpfman-catalog

OLM catalog used to release the Openshift eBPF Manager Operator.

## Overview

This repository builds the BPFMan OLM catalog used to release new versions of the OpenShift eBPF Manager Operator. It generates operator catalogs from templates and packages them as OCI images for deployment to OpenShift clusters.

## Directory Structure

- `templates/` - Source YAML files defining operator versions and channels
  - `dev.yaml` - Development builds from Konflux CI (default)
  - `y-stream.yaml` - Y-stream minor version releases
  - `z-stream.yaml` - Z-stream patch releases
- `auto-generated/` - Generated catalogs (created by `make generate-catalogs`)
  - `catalog/` - Modern catalog format with bundle-object-to-csv-metadata migration
- `Dockerfile` - Container definition for building catalog images
- `catalog-source.yaml` - CatalogSource resource for deploying to OpenShift

## Common Commands

Run `make` to list all available targets.

### Generate Catalogs

Generate catalogs from templates:
```bash
make generate-catalogs
```

### Build Container Image

Build catalog image (defaults to dev):
```bash
make build-image
```

Build with y-stream releases:
```bash
make build-image BUILD_STREAM=y-stream
```

Build with custom image tag:
```bash
make build-image IMAGE=quay.io/myuser/bpfman-catalog:latest
```

### Deploy to Cluster

Deploy catalog to OpenShift cluster:
```bash
make build-image push-image deploy
```

Individual steps:
```bash
make push-image    # Push built image
make deploy        # Deploy catalog source to cluster
make undeploy      # Remove catalog source from cluster
```

## Development Workflow

1. Modify template files in `templates/` directory
2. Run `make generate-catalogs` to update auto-generated catalogs
3. Build and test with `make build-image`
4. Push image with `make push-image`
5. Deploy to test cluster with `make deploy`

## Configuration Variables

- `IMAGE` - Target image name (default: quay.io/$USER/bpfman-operator-catalog:latest)
- `BUILD_STREAM` - Which template to use (default: dev, options: dev, y-stream, z-stream)
- `OCI_BIN` - Container runtime (docker or podman, auto-detected)

## Development vs Release Workflows

### For Daily Development
Use the default `dev` template which points to your latest CI builds from Konflux:
```bash
# Update templates/dev.yaml with your latest bundle SHA from Konflux
$EDITOR templates/dev.yaml
make generate-catalogs
make build-image
make push-image
make deploy
```

### For Y-Stream Releases
Use the `y-stream` template for minor version releases:
```bash
make generate-catalogs
make build-image BUILD_STREAM=y-stream
make push-image BUILD_STREAM=y-stream
make deploy BUILD_STREAM=y-stream
```

### For Z-Stream Releases
Use the `z-stream` template for patch releases:
```bash
make generate-catalogs
make build-image BUILD_STREAM=z-stream
make push-image BUILD_STREAM=z-stream
make deploy BUILD_STREAM=z-stream
```
