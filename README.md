# BPFMan Operator Catalog

This repository builds the BPFMan OLM catalog used to release new versions of the OpenShift eBPF Manager Operator.

To generate all catalogs from template:
```bash
make generate
```

To build the catalog with released version:
```bash
podman build -t catalog .
```

To use upstream opm:
```bash
podman build -t catalog -f upstream.Dockerfile .
```

To use a different generated catalog:
```bash
# For development/y-stream
podman build --build-arg INDEX_FILE=./auto-generated/catalog/y-stream.yaml -t catalog .

# For released catalog
podman build --build-arg INDEX_FILE=./auto-generated/catalog/released.yaml -t catalog .
```

One-step, build and test on a cluster:
```bash
make build-image push-image deploy
```
