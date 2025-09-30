# All Build arguments (ARGS) without defaults - values MUST be
# provided at build time. Local builds: Makefile provides defaults
# (see make build-image). Konflux/Tekton: Values come from pipeline
# parameters and Dockerfile-args files

ARG BASE_IMAGE
FROM ${BASE_IMAGE}

ARG COMMIT
ARG BUILDVERSION
ARG INDEX_FILE
COPY $INDEX_FILE /configs/bpfman-operator/index.yaml

ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs", "--cache-dir=/tmp/cache"]

RUN ["/bin/opm", "serve", "/configs", "--cache-dir=/tmp/cache", "--cache-only"]

# Indicate where the catalog configuration is located in the image.
LABEL operators.operatorframework.io.index.configs.v1=/configs

LABEL com.redhat.component="bpfman-operator-catalog-container"
LABEL name="bpfman-operator-catalog"
LABEL io.k8s.display-name="eBPF Manager Operator Catalog"
LABEL io.k8s.description="eBPF Manager Operator Catalog"
LABEL summary="eBPF Manager Operator Catalog"
LABEL maintainer="support@redhat.com"
LABEL io.openshift.tags="bpfman-operator-catalog"
LABEL upstream-vcs-ref="$COMMIT"
LABEL upstream-vcs-type="git"
LABEL description="eBPF Manager operator for OpenShift."
LABEL version="$BUILDVERSION"
