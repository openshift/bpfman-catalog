ARG OPM_IMAGE=brew.registry.redhat.io/rh-osbs/openshift-ose-operator-registry-rhel9:v4.17
FROM $OPM_IMAGE

ARG COMMIT
ARG BUILDVERSION="1.9.0"
ARG INDEX_FILE=./auto-generated/catalog/released.yaml
#This files will be copied twice but it is not possible to COPY if not empty
COPY $INDEX_FILE /configs/bpfman-operator/index.yaml
RUN ls -R /configs

# Configure the entrypoint and command
ENTRYPOINT ["opm"]
CMD ["serve", "/configs", "--cache-dir=/tmp/cache"]

RUN opm serve /configs --cache-dir=/tmp/cache --cache-only

# Set DC-specific label for the location of the DC root directory
# in the image
LABEL operators.operatorframework.io.index.configs.v1=/configs

LABEL com.redhat.component="network-observability-operator-catalog-container"
LABEL name="network-observability-operator-catalog"
LABEL io.k8s.display-name="Network Observability Operator Catalog"
LABEL io.k8s.description="Network Observability Operator Catalog"
LABEL summary="Network Observability Operator Catalog"
LABEL maintainer="support@redhat.com"
LABEL io.openshift.tags="network-observability-operator-catalog"
LABEL upstream-vcs-ref="$COMMIT"
LABEL upstream-vcs-type="git"
LABEL description="Network Observability operator for OpenShift."
LABEL version="$BUILDVERSION"
